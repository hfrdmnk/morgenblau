package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"morgenblau/internal/safehttp"
)

type track struct {
	lang, kind, path string
}

// fakePlayer builds an InnerTube player answer whose caption tracks point back at origin.
func fakePlayer(origin string, defaultCaption int, length int, tracks ...track) map[string]any {
	captionTracks := []map[string]any{}
	for _, tr := range tracks {
		t := map[string]any{"baseUrl": origin + tr.path, "languageCode": tr.lang}
		if tr.kind != "" {
			t["kind"] = tr.kind
		}
		captionTracks = append(captionTracks, t)
	}
	renderer := map[string]any{"captionTracks": captionTracks}
	if defaultCaption >= 0 {
		renderer["audioTracks"] = []map[string]any{
			{"defaultCaptionTrackIndex": 0},
			{"defaultCaptionTrackIndex": defaultCaption},
		}
		renderer["defaultAudioTrackIndex"] = 1
	}
	response := map[string]any{
		"playabilityStatus": map[string]any{"status": "OK"},
		"videoDetails":      map[string]any{"lengthSeconds": strconv.Itoa(length)},
	}
	if len(tracks) > 0 {
		response["captions"] = map[string]any{"playerCaptionsTracklistRenderer": renderer}
	}
	return response
}

const manualJSON3 = `{"events":[
	{"tStartMs":4220,"dDurationMs":1180,"segs":[{"utf8":"This is a 3."}]},
	{"tStartMs":6060,"dDurationMs":4653,"segs":[{"utf8":"It is written & rendered\nat 28x28."}]}
]}`

// fakeYouTube serves the player endpoint and any caption paths; it counts every request per path.
type fakeYouTube struct {
	t       *testing.T
	server  *httptest.Server
	player  func(origin string) (int, any)
	caption map[string]string
	hits    map[string]*atomic.Int32
	agents  []string
}

func newFakeYouTube(t *testing.T) *fakeYouTube {
	f := &fakeYouTube{t: t, caption: map[string]string{}, hits: map[string]*atomic.Int32{"/youtubei/v1/player": {}}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.agents = append(f.agents, r.Header.Get("User-Agent"))
		if counter, ok := f.hits[r.URL.Path]; ok {
			counter.Add(1)
		}
		if r.URL.Path == "/youtubei/v1/player" {
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Context struct {
					Client struct{ ClientName string } `json:"client"`
				} `json:"context"`
				VideoID string `json:"videoId"`
			}
			if err := json.Unmarshal(body, &req); err != nil || req.VideoID != "abcdefghijk" || req.Context.Client.ClientName != "IOS" {
				t.Errorf("player request body = %s", body)
			}
			status, payload := f.player(f.server.URL)
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(payload)
			return
		}
		if r.URL.Query().Get("fmt") != "json3" {
			t.Errorf("caption request %s does not ask for json3", r.URL)
		}
		body, ok := f.caption[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeYouTube) serveCaption(path, body string) {
	f.caption[path] = body
	f.hits[path] = &atomic.Int32{}
}

func (f *fakeYouTube) client() *Client {
	return New(safehttp.NewClient(5*time.Second, 3, safehttp.WithAllowLoopback()), f.server.URL)
}

func TestFetch_PicksTheManualTrackInTheDefaultCaptionLanguage(t *testing.T) {
	yt := newFakeYouTube(t)
	yt.serveCaption("/ar", manualJSON3)
	yt.serveCaption("/en-asr", `{"events":[]}`)
	yt.serveCaption("/en", manualJSON3)
	// YouTube marks the English ASR track as default for the audio; the English manual track must still win.
	yt.player = func(origin string) (int, any) {
		return http.StatusOK, fakePlayer(origin, 1, 600, track{"ar", "", "/ar?v=1"}, track{"en", "asr", "/en-asr?v=1"}, track{"en", "", "/en?v=1"})
	}

	got, err := yt.client().Fetch(context.Background(), "abcdefghijk")
	if err != nil {
		t.Fatal(err)
	}
	if got.Language != "en" || got.Kind != KindManual {
		t.Fatalf("track = %s/%s, want en/manual", got.Language, got.Kind)
	}
	want := []Cue{{Start: 4220, Duration: 1180, Text: "This is a 3."}, {Start: 6060, Duration: 4653, Text: "It is written & rendered at 28x28."}}
	if len(got.Cues) != len(want) {
		t.Fatalf("cues = %+v", got.Cues)
	}
	for i := range want {
		if got.Cues[i] != want[i] {
			t.Errorf("cue %d = %+v, want %+v", i, got.Cues[i], want[i])
		}
	}
	if yt.hits["/ar"].Load() != 0 || yt.hits["/en-asr"].Load() != 0 {
		t.Error("fetched a caption track that was not chosen")
	}
	for _, agent := range yt.agents {
		if agent != safehttp.UserAgent {
			t.Errorf("User-Agent = %q, want Morgenblau's own", agent)
		}
	}
}

func TestFetch_WithoutADefaultFallsBackToTheSpokenLanguage(t *testing.T) {
	yt := newFakeYouTube(t)
	yt.serveCaption("/de", manualJSON3)
	yt.serveCaption("/fr-asr", manualJSON3)
	// Only an ASR track exists in the spoken language, so it beats the first-listed manual track.
	yt.player = func(origin string) (int, any) {
		return http.StatusOK, fakePlayer(origin, -1, 600, track{"de", "", "/de?v=1"}, track{"fr", "asr", "/fr-asr?v=1"})
	}

	got, err := yt.client().Fetch(context.Background(), "abcdefghijk")
	if err != nil {
		t.Fatal(err)
	}
	if got.Language != "fr" || got.Kind != KindASR {
		t.Fatalf("track = %s/%s, want fr/asr", got.Language, got.Kind)
	}
}

func TestFetch_ParsesWordLevelASRIntoLines(t *testing.T) {
	yt := newFakeYouTube(t)
	yt.serveCaption("/asr", `{"events":[
		{"tStartMs":0,"dDurationMs":1107920,"id":1},
		{"tStartMs":4280,"dDurationMs":4080,"segs":[{"utf8":"This"},{"utf8":" is","tOffsetMs":480},{"utf8":" a three.","tOffsetMs":680}]},
		{"tStartMs":6830,"dDurationMs":1530,"aAppend":1,"segs":[{"utf8":"\n"}]},
		{"tStartMs":6840,"dDurationMs":4400,"segs":[{"utf8":"and"},{"utf8":" rendered","tOffsetMs":200}]}
	]}`)
	yt.player = func(origin string) (int, any) {
		return http.StatusOK, fakePlayer(origin, 0, 600, track{"en", "asr", "/asr?v=1"})
	}

	got, err := yt.client().Fetch(context.Background(), "abcdefghijk")
	if err != nil {
		t.Fatal(err)
	}
	want := []Cue{{Start: 4280, Duration: 4080, Text: "This is a three."}, {Start: 6840, Duration: 4400, Text: "and rendered"}}
	if len(got.Cues) != 2 || got.Cues[0] != want[0] || got.Cues[1] != want[1] {
		t.Fatalf("cues = %+v, want %+v", got.Cues, want)
	}
}

func TestFetch_ReportsNoTranscript(t *testing.T) {
	cases := map[string]func(origin string) (int, any){
		"no caption tracks": func(origin string) (int, any) { return http.StatusOK, fakePlayer(origin, -1, 600) },
		"unplayable": func(string) (int, any) {
			return http.StatusOK, map[string]any{"playabilityStatus": map[string]any{"status": "UNPLAYABLE"}}
		},
		"longer than the cap": func(origin string) (int, any) {
			return http.StatusOK, fakePlayer(origin, 0, 4*3600+1, track{"en", "", "/en?v=1"})
		},
	}
	for name, player := range cases {
		t.Run(name, func(t *testing.T) {
			yt := newFakeYouTube(t)
			yt.serveCaption("/en", manualJSON3)
			yt.player = player
			_, err := yt.client().Fetch(context.Background(), "abcdefghijk")
			if !errors.Is(err, ErrNoTranscript) {
				t.Fatalf("err = %v, want ErrNoTranscript", err)
			}
			if yt.hits["/en"].Load() != 0 {
				t.Error("fetched captions for a video that cannot have a transcript")
			}
		})
	}
}

func TestFetch_BlockedStartsACooldownThatSkipsYouTube(t *testing.T) {
	cases := map[string]func(origin string) (int, any){
		"bot check": func(string) (int, any) {
			return http.StatusOK, map[string]any{"playabilityStatus": map[string]any{"status": "LOGIN_REQUIRED", "reason": "Sign in to confirm you're not a bot"}}
		},
		"rate limited": func(string) (int, any) { return http.StatusTooManyRequests, map[string]any{} },
	}
	for name, player := range cases {
		t.Run(name, func(t *testing.T) {
			yt := newFakeYouTube(t)
			yt.player = player
			client := yt.client()
			now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
			client.now = func() time.Time { return now }

			if _, err := client.Fetch(context.Background(), "abcdefghijk"); !errors.Is(err, ErrBlocked) {
				t.Fatalf("err = %v, want ErrBlocked", err)
			}
			now = now.Add(14 * time.Minute)
			if _, err := client.Fetch(context.Background(), "abcdefghijk"); !errors.Is(err, ErrBlocked) {
				t.Fatalf("err during cooldown = %v, want ErrBlocked", err)
			}
			if got := yt.hits["/youtubei/v1/player"].Load(); got != 1 {
				t.Fatalf("player calls = %d, want 1: the cooldown must keep YouTube out of reach", got)
			}
			now = now.Add(2 * time.Minute)
			_, _ = client.Fetch(context.Background(), "abcdefghijk")
			if got := yt.hits["/youtubei/v1/player"].Load(); got != 2 {
				t.Fatalf("player calls after cooldown = %d, want 2", got)
			}
			// A second block doubles the wait.
			now = now.Add(29 * time.Minute)
			_, _ = client.Fetch(context.Background(), "abcdefghijk")
			if got := yt.hits["/youtubei/v1/player"].Load(); got != 2 {
				t.Fatalf("player calls inside the doubled cooldown = %d, want 2", got)
			}
		})
	}
}

func TestFetch_EmptyCaptionBodyIsABlock(t *testing.T) {
	yt := newFakeYouTube(t)
	yt.serveCaption("/en", "")
	yt.player = func(origin string) (int, any) {
		return http.StatusOK, fakePlayer(origin, 0, 600, track{"en", "", "/en?v=1"})
	}
	if _, err := yt.client().Fetch(context.Background(), "abcdefghijk"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
}

func TestFetch_RefusesCaptionURLsOffTheYouTubeOrigin(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
		_, _ = io.WriteString(w, manualJSON3)
	}))
	defer other.Close()
	yt := newFakeYouTube(t)
	yt.player = func(string) (int, any) {
		return http.StatusOK, fakePlayer(other.URL, 0, 600, track{"en", "", "/en?v=1"})
	}

	if _, err := yt.client().Fetch(context.Background(), "abcdefghijk"); err == nil {
		t.Fatal("fetched a caption track from another origin")
	}
	if elsewhere.Load() != 0 {
		t.Fatal("requested a caption URL off the YouTube origin")
	}
}

func TestFetch_RefusesAnOversizedCaptionBody(t *testing.T) {
	yt := newFakeYouTube(t)
	yt.serveCaption("/en", `{"events":[{"tStartMs":0,"dDurationMs":1,"segs":[{"utf8":"`+strings.Repeat("a", maxCaptionBytes)+`"}]}]}`)
	yt.player = func(origin string) (int, any) {
		return http.StatusOK, fakePlayer(origin, 0, 600, track{"en", "", "/en?v=1"})
	}
	if _, err := yt.client().Fetch(context.Background(), "abcdefghijk"); err == nil {
		t.Fatal("accepted a caption body over the limit")
	}
}

func TestVideoID(t *testing.T) {
	cases := map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ":         "dQw4w9WgXcQ",
		"https://m.youtube.com/watch?v=dQw4w9WgXcQ&t=10":      "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ":                        "dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ":          "dQw4w9WgXcQ",
		"https://www.youtube.com/live/dQw4w9WgXcQ/":           "dQw4w9WgXcQ",
		"https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ":  "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=short":               "",
		"https://www.youtube.com/channel/UCabcdefghijklmnop":  "",
		"https://youtube.com.example.com/watch?v=dQw4w9WgXcQ": "",
		"javascript:alert(1)":                                 "",
		"":                                                    "",
	}
	for raw, want := range cases {
		if got := VideoID(raw); got != want {
			t.Errorf("VideoID(%q) = %q, want %q", raw, got, want)
		}
	}
}
