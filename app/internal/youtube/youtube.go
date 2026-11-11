// Package youtube fetches a video's own caption track for the reader's transcript.
//
// YouTube offers no public API for the captions of videos we don't own, so this calls the InnerTube player API as YouTube's iOS app does. SPEC names this as the one exception to the honest-fetcher rule: requests still carry safehttp.UserAgent, and a block is final until the cooldown ends.
package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"morgenblau/internal/backoff"
	"morgenblau/internal/safehttp"
)

// iosClientVersion is the iOS app version InnerTube is asked as; when YouTube retires it, transcripts go dark through ErrBlocked until it is bumped.
const iosClientVersion = "20.10.3"

const (
	maxPlayerBytes  = 2 << 20
	maxCaptionBytes = 4 << 20
	// maxLength keeps day-long streams, about 120 KB of captions per hour, out of storage.
	maxLength = 4 * time.Hour
)

// Caption track kinds as the reader shows them.
const (
	KindManual = "manual"
	KindASR    = "asr"
)

var (
	// ErrNoTranscript means YouTube answered and the video has no transcript to show: no captions, unplayable, live, or longer than the cap.
	ErrNoTranscript = errors.New("youtube: no transcript")
	// ErrBlocked means YouTube refused or throttled us, or the cooldown after such a refusal has not ended.
	ErrBlocked = errors.New("youtube: blocked")
)

// cooldown backs off every call after a block: 15 minutes, doubling to a day.
var cooldown = backoff.Policy{Steps: backoff.Exponential(15*time.Minute, 2, 24*time.Hour)}

// Cue is one timed caption line; times are milliseconds from the start of the video.
type Cue struct {
	Start    int64  `json:"start"`
	Duration int64  `json:"duration"`
	Text     string `json:"text"`
}

// Transcript is the chosen caption track of one video.
type Transcript struct {
	Language string `json:"language"`
	Kind     string `json:"kind"`
	Cues     []Cue  `json:"cues"`
}

// Client fetches transcripts and holds the process-wide cooldown, so one Client serves the whole server.
type Client struct {
	http   *http.Client
	origin string
	now    func() time.Time

	mu       sync.Mutex
	failures int
	until    time.Time
}

// New returns a Client for origin, https://www.youtube.com in production; httpClient must come from safehttp.
func New(httpClient *http.Client, origin string) *Client {
	return &Client{http: httpClient, origin: strings.TrimSuffix(origin, "/"), now: time.Now}
}

// Fetch returns the transcript of videoID, or ErrNoTranscript, ErrBlocked, or a transient error.
func (c *Client) Fetch(ctx context.Context, videoID string) (Transcript, error) {
	if c.cooling() {
		return Transcript{}, fmt.Errorf("%w: cooling down", ErrBlocked)
	}
	transcript, err := c.fetch(ctx, videoID)
	c.observe(err)
	return transcript, err
}

func (c *Client) cooling() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now().Before(c.until)
}

func (c *Client) observe(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case err == nil, errors.Is(err, ErrNoTranscript):
		c.failures = 0
	case errors.Is(err, ErrBlocked):
		c.failures++
		c.until = c.now().Add(cooldown.Delay(c.failures))
	}
}

type captionTrack struct {
	BaseURL      string `json:"baseUrl"`
	LanguageCode string `json:"languageCode"`
	Kind         string `json:"kind"`
}

type playerResponse struct {
	PlayabilityStatus struct {
		Status string `json:"status"`
	} `json:"playabilityStatus"`
	VideoDetails struct {
		LengthSeconds string `json:"lengthSeconds"`
		IsLive        bool   `json:"isLive"`
	} `json:"videoDetails"`
	Captions struct {
		Renderer struct {
			CaptionTracks []captionTrack `json:"captionTracks"`
			AudioTracks   []struct {
				DefaultCaptionTrackIndex *int `json:"defaultCaptionTrackIndex"`
			} `json:"audioTracks"`
			DefaultAudioTrackIndex int `json:"defaultAudioTrackIndex"`
		} `json:"playerCaptionsTracklistRenderer"`
	} `json:"captions"`
}

func (c *Client) fetch(ctx context.Context, videoID string) (Transcript, error) {
	player, err := c.player(ctx, videoID)
	if err != nil {
		return Transcript{}, err
	}
	switch player.PlayabilityStatus.Status {
	case "OK":
	case "LOGIN_REQUIRED":
		return Transcript{}, fmt.Errorf("%w: bot check", ErrBlocked)
	default:
		return Transcript{}, fmt.Errorf("%w: playability %q", ErrNoTranscript, player.PlayabilityStatus.Status)
	}
	if player.VideoDetails.IsLive {
		return Transcript{}, fmt.Errorf("%w: live", ErrNoTranscript)
	}
	if seconds, err := strconv.Atoi(player.VideoDetails.LengthSeconds); err == nil && time.Duration(seconds)*time.Second > maxLength {
		return Transcript{}, fmt.Errorf("%w: %ds long", ErrNoTranscript, seconds)
	}
	track, ok := pickTrack(player)
	if !ok {
		return Transcript{}, fmt.Errorf("%w: no captions", ErrNoTranscript)
	}
	cues, err := c.captions(ctx, track.BaseURL)
	if err != nil {
		return Transcript{}, err
	}
	kind := KindManual
	if track.Kind == "asr" {
		kind = KindASR
	}
	return Transcript{Language: track.LanguageCode, Kind: kind, Cues: cues}, nil
}

func (c *Client) player(ctx context.Context, videoID string) (playerResponse, error) {
	body, err := json.Marshal(map[string]any{
		"context": map[string]any{"client": map[string]any{"clientName": "IOS", "clientVersion": iosClientVersion}},
		"videoId": videoID,
	})
	if err != nil {
		return playerResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+"/youtubei/v1/player?prettyPrint=false", bytes.NewReader(body))
	if err != nil {
		return playerResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	raw, err := c.do(req, maxPlayerBytes)
	if err != nil {
		return playerResponse{}, err
	}
	var player playerResponse
	if err := json.Unmarshal(raw, &player); err != nil {
		return playerResponse{}, fmt.Errorf("youtube: player response: %w", err)
	}
	return player, nil
}

// pickTrack follows the video, not the reader: the default caption language of the default audio track, else the spoken language that only ASR tracks name, else the first track. Within the language a manual track beats ASR.
func pickTrack(player playerResponse) (captionTrack, bool) {
	renderer := player.Captions.Renderer
	tracks := renderer.CaptionTracks
	if len(tracks) == 0 {
		return captionTrack{}, false
	}
	language := ""
	if i := renderer.DefaultAudioTrackIndex; i >= 0 && i < len(renderer.AudioTracks) {
		if d := renderer.AudioTracks[i].DefaultCaptionTrackIndex; d != nil && *d >= 0 && *d < len(tracks) {
			language = tracks[*d].LanguageCode
		}
	}
	if language == "" {
		for _, t := range tracks {
			if t.Kind == "asr" {
				language = t.LanguageCode
				break
			}
		}
	}
	if language == "" {
		language = tracks[0].LanguageCode
	}
	var asr *captionTrack
	for i, t := range tracks {
		if t.LanguageCode != language {
			continue
		}
		if t.Kind != "asr" {
			return t, true
		}
		if asr == nil {
			asr = &tracks[i]
		}
	}
	return *asr, true
}

type json3 struct {
	Events []struct {
		Start    int64 `json:"tStartMs"`
		Duration int64 `json:"dDurationMs"`
		Segs     []struct {
			Text string `json:"utf8"`
		} `json:"segs"`
	} `json:"events"`
}

func (c *Client) captions(ctx context.Context, baseURL string) ([]Cue, error) {
	// The track URL is signed for our IP and expires, so it is used at once and only on the origin the player call went to.
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme+"://"+u.Host != c.origin {
		return nil, fmt.Errorf("youtube: caption track off the YouTube origin: %q", baseURL)
	}
	q := u.Query()
	q.Set("fmt", "json3")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	raw, err := c.do(req, maxCaptionBytes)
	if err != nil {
		return nil, err
	}
	// An empty answer to a signed track URL is how a throttled timedtext call looks.
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("%w: empty caption body", ErrBlocked)
	}
	var doc json3
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("youtube: caption body: %w", err)
	}
	var cues []Cue
	for _, event := range doc.Events {
		var text strings.Builder
		for _, seg := range event.Segs {
			text.WriteString(seg.Text)
		}
		line := strings.Join(strings.Fields(text.String()), " ")
		if line == "" {
			continue
		}
		cues = append(cues, Cue{Start: event.Start, Duration: event.Duration, Text: line})
	}
	if len(cues) == 0 {
		return nil, fmt.Errorf("%w: caption track is empty", ErrNoTranscript)
	}
	return cues, nil
}

func (c *Client) do(req *http.Request, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", safehttp.UserAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusTooManyRequests:
		return nil, fmt.Errorf("%w: HTTP %d from %s", ErrBlocked, resp.StatusCode, req.URL.Path)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("youtube: HTTP %d from %s", resp.StatusCode, req.URL.Path)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("youtube: %s answered over %d bytes", req.URL.Path, limit)
	}
	return raw, nil
}

var videoIDPattern = regexp.MustCompile(`^[\w-]{11}$`)
var pathID = regexp.MustCompile(`^/(?:shorts|embed|live)/([^/]+)/?$`)

// VideoID returns the video a YouTube URL names, or "" for anything else; it mirrors youtubeID in the frontend's lib/reader.ts.
func VideoID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	id := ""
	switch u.Hostname() {
	case "youtu.be":
		id = strings.TrimPrefix(u.Path, "/")
	case "youtube.com", "www.youtube.com", "m.youtube.com", "www.youtube-nocookie.com":
		if u.Path == "/watch" {
			id = u.Query().Get("v")
		} else if m := pathID.FindStringSubmatch(u.Path); m != nil {
			id = m[1]
		}
	}
	if !videoIDPattern.MatchString(id) {
		return ""
	}
	return id
}
