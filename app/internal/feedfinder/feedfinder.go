package feedfinder

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/mmcdole/gofeed"

	"morgenblau/internal/safehttp"
	"morgenblau/internal/standardfeed"
)

const maxFeedSniffBytes = 4 << 20

type Candidate struct {
	FeedURL     string `json:"feedUrl,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Publication string `json:"publication,omitempty"`
	Title       string `json:"title,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	SiteURL     string `json:"siteUrl,omitempty"`
}

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type StandardResolver interface {
	GetPublication(ctx context.Context, uri string) (*standardfeed.Publication, error)
	GetDocument(ctx context.Context, uri string) (*standardfeed.Document, error)
	FetchWellKnown(ctx context.Context, siteURL string) (string, error)
}

type Finder struct {
	client   HTTPDoer
	standard StandardResolver
}

func New(client HTTPDoer) *Finder {
	return &Finder{client: client}
}

func (f *Finder) WithStandardResolver(r StandardResolver) *Finder {
	f.standard = r
	return f
}

func newGET(ctx context.Context, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", safehttp.UserAgent)
	return req, nil
}

func mediaType(resp *http.Response) string {
	return strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0])
}

func finalURL(resp *http.Response, requested *url.URL) *url.URL {
	if resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL
	}
	return requested
}

var feedContentTypes = map[string]struct{}{
	"application/rss+xml":   {},
	"application/atom+xml":  {},
	"application/feed+json": {},
	"application/json":      {},
	"text/xml":              {},
	"application/xml":       {},
}

func (f *Finder) Resolve(ctx context.Context, raw string) ([]Candidate, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty url")
	}
	if strings.HasPrefix(raw, "at://") {
		if f.standard == nil {
			return nil, nil
		}
		return f.resolveATURI(ctx, raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme == "" {
		u.Scheme = "https"
		raw = u.String()
	}

	if host := strings.ToLower(u.Host); strings.HasSuffix(host, "youtube.com") || host == "youtu.be" {
		c, err := f.resolveYouTube(ctx, u)
		if err != nil {
			return nil, err
		}
		if c != nil {
			return []Candidate{*c}, nil
		}
	}

	probeCtx, cancelProbe := context.WithCancel(ctx)
	var wellKnownURI string
	var probe sync.WaitGroup
	defer func() {
		cancelProbe()
		probe.Wait()
	}()
	if f.standard != nil {
		origin := (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
		probe.Go(func() {
			if uri, err := f.standard.FetchWellKnown(probeCtx, origin); err == nil {
				wellKnownURI = uri
			}
		})
	}

	req, err := newGET(ctx, raw)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	base := finalURL(resp, u)
	ct := mediaType(resp)
	if _, isFeed := feedContentTypes[strings.ToLower(ct)]; isFeed {
		title, link := sniffFeed(resp.Body)
		return []Candidate{{FeedURL: raw, ContentType: ct, SiteURL: siteLink(base, link), Title: title}}, nil
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	readable := resp.StatusCode >= 200 && resp.StatusCode < 300
	probed := map[string]*probedFeed{}
	out := f.pageFeeds(ctx, doc, base, readable, probed)
	if f.standard != nil {
		probe.Wait()
		var pubURIs []string
		if wellKnownURI != "" {
			pubURIs = append(pubURIs, wellKnownURI)
		}
		if docURI := extractStandardDocLink(doc); docURI != "" {
			if sdoc, err := f.standard.GetDocument(ctx, docURI); err == nil && strings.HasPrefix(sdoc.Site, "at://") {
				pubURIs = append(pubURIs, sdoc.Site)
			}
		}
		out = append(out, f.publicationCandidates(ctx, pubURIs)...)
	}
	if len(out) == 0 && readable {
		out = f.sectionFeeds(ctx, doc, base, probed)
	}
	return out, nil
}

func (f *Finder) pageFeeds(ctx context.Context, doc *goquery.Document, base *url.URL, readable bool, probed map[string]*probedFeed) []Candidate {
	if out := extractLinkRels(doc, base); len(out) > 0 {
		return out
	}
	if c := f.discoverFeed(ctx, doc, base, readable, probed); c != nil {
		return []Candidate{*c}
	}
	return nil
}

func (f *Finder) resolveATURI(ctx context.Context, raw string) ([]Candidate, error) {
	uri, err := syntax.ParseATURI(raw)
	if err != nil {
		return nil, nil
	}
	switch uri.Collection().String() {
	case standardfeed.CollectionPublication:
		pub, err := f.standard.GetPublication(ctx, raw)
		if err != nil {
			return nil, err
		}
		return []Candidate{publicationCandidate(pub)}, nil
	case standardfeed.CollectionDocument:
		doc, err := f.standard.GetDocument(ctx, raw)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(doc.Site, "at://") {
			return nil, nil
		}
		pub, err := f.standard.GetPublication(ctx, doc.Site)
		if err != nil {
			return nil, err
		}
		return []Candidate{publicationCandidate(pub)}, nil
	}
	return nil, nil
}

func (f *Finder) publicationCandidates(ctx context.Context, uris []string) []Candidate {
	var out []Candidate
	seen := make(map[string]struct{}, len(uris))
	for _, uri := range uris {
		pub, err := f.standard.GetPublication(ctx, uri)
		if err != nil {
			continue
		}
		if _, dup := seen[pub.URI]; dup {
			continue
		}
		seen[pub.URI] = struct{}{}
		out = append(out, publicationCandidate(pub))
	}
	return out
}

func publicationCandidate(pub *standardfeed.Publication) Candidate {
	return Candidate{
		Kind:        "standardfeed",
		Publication: pub.URI,
		Title:       pub.Name,
		SiteURL:     pub.URL,
	}
}

func extractStandardDocLink(doc *goquery.Document) string {
	href, _ := doc.Find("link[rel='site.standard.document']").First().Attr("href")
	href = strings.TrimSpace(href)
	if !strings.HasPrefix(href, "at://") {
		return ""
	}
	return href
}

func sniffFeed(body io.Reader) (title, link string) {
	buf, err := io.ReadAll(io.LimitReader(body, maxFeedSniffBytes))
	if err != nil {
		return "", ""
	}
	parsed, err := gofeed.NewParser().Parse(bytes.NewReader(buf))
	if err != nil || parsed == nil {
		return "", ""
	}
	return strings.TrimSpace(parsed.Title), strings.TrimSpace(parsed.Link)
}

func siteLink(feedURL *url.URL, link string) string {
	if link == "" {
		return ""
	}
	abs, err := feedURL.Parse(link)
	if err != nil || (abs.Scheme != "http" && abs.Scheme != "https") || abs.Host == "" {
		return ""
	}
	return abs.String()
}

func extractLinkRels(doc *goquery.Document, base *url.URL) []Candidate {
	var out []Candidate
	doc.Find("link[rel='alternate']").Each(func(_ int, s *goquery.Selection) {
		ct, _ := s.Attr("type")
		ct = strings.ToLower(strings.TrimSpace(ct))
		if _, ok := feedContentTypes[ct]; !ok {
			return
		}
		href, _ := s.Attr("href")
		href = strings.TrimSpace(href)
		if href == "" {
			return
		}
		abs, err := base.Parse(href)
		if err != nil {
			return
		}
		title, _ := s.Attr("title")
		out = append(out, Candidate{
			FeedURL:     abs.String(),
			Title:       strings.TrimSpace(title),
			ContentType: ct,
			SiteURL:     base.String(),
		})
	})
	return out
}

var ytChannelPath = regexp.MustCompile(`^/channel/([A-Za-z0-9_-]+)$`)

var ytChannelIDInHTML = regexp.MustCompile(`(?:"channelId":"|/channel/|channel_id=)(UC[A-Za-z0-9_-]{20,})`)

func (f *Finder) resolveYouTube(ctx context.Context, u *url.URL) (*Candidate, error) {
	var channelID string
	path := u.Path
	switch {
	case ytChannelPath.MatchString(path):
		channelID = ytChannelPath.FindStringSubmatch(path)[1]
	case strings.HasPrefix(path, "/@"),
		strings.HasPrefix(path, "/c/"),
		strings.HasPrefix(path, "/user/"):
		req, err := newGET(ctx, u.String())
		if err != nil {
			return nil, err
		}
		resp, err := f.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil, nil
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return nil, err
		}
		if m := ytChannelIDInHTML.FindSubmatch(body); m != nil {
			channelID = string(m[1])
		}
	}
	if channelID == "" {
		return nil, nil
	}
	cand := ytFeedCandidate(channelID, u)
	cand.Title = f.fetchYTFeedTitle(ctx, cand.FeedURL)
	return cand, nil
}

func (f *Finder) fetchYTFeedTitle(ctx context.Context, feedURL string) string {
	req, err := newGET(ctx, feedURL)
	if err != nil {
		return ""
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return ""
	}
	title, _ := sniffFeed(resp.Body)
	return title
}

func ytFeedCandidate(channelID string, u *url.URL) *Candidate {
	return &Candidate{
		FeedURL:     ytChannelFeed + channelID,
		ContentType: "application/atom+xml",
		SiteURL:     u.String(),
	}
}

const (
	ytChannelFeed      = "https://www.youtube.com/feeds/videos.xml?channel_id="
	ytPlaylistFeed     = "https://www.youtube.com/feeds/videos.xml?playlist_id="
	ytLongFormPlaylist = "UULF"
)

var (
	ytChannelID          = regexp.MustCompile(`^UC([A-Za-z0-9_-]{22})$`)
	ytLongFormPlaylistID = regexp.MustCompile(`^` + ytLongFormPlaylist + `([A-Za-z0-9_-]{22})$`)
)

func SameYouTubeFeedURLs(raw string) []string {
	id := ytFeedChannel(raw)
	if id == "" {
		return nil
	}
	return []string{ytChannelFeed + "UC" + id, ytPlaylistFeed + ytLongFormPlaylist + id}
}

func YouTubeChannelURL(raw string) string {
	id := ytFeedChannel(raw)
	if u, err := url.Parse(raw); id == "" && err == nil && isYouTubeHost(u.Host) {
		if m := ytChannelPath.FindStringSubmatch(u.Path); m != nil {
			if m := ytChannelID.FindStringSubmatch(m[1]); m != nil {
				id = m[1]
			}
		}
	}
	if id == "" {
		return ""
	}
	return "https://www.youtube.com/channel/UC" + id
}

var ytVanityURL = regexp.MustCompile(`"vanityChannelUrl":"https?://(?:www\.)?youtube\.com/(@[^"/?]+)"`)

func YouTubeHandleURL(ctx context.Context, client HTTPDoer, channelURL string) (string, error) {
	body, err := youTubeChannelPage(ctx, client, channelURL)
	if err != nil {
		return "", err
	}
	if m := ytVanityURL.FindSubmatch(body); m != nil {
		return "https://www.youtube.com/" + string(m[1]), nil
	}
	return "", nil
}

var (
	ytErrorAlert      = []byte(`"alertRenderer":{"type":"ERROR"`)
	ytChannelMetadata = []byte(`"channelMetadataRenderer"`)
)

func YouTubeChannelGone(ctx context.Context, client HTTPDoer, channelURL string) (bool, error) {
	body, err := youTubeChannelPage(ctx, client, channelURL)
	if err != nil {
		return false, err
	}
	return bytes.Contains(body, ytErrorAlert) && !bytes.Contains(body, ytChannelMetadata), nil
}

func youTubeChannelPage(ctx context.Context, client HTTPDoer, channelURL string) ([]byte, error) {
	req, err := newGET(ctx, channelURL)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, nil
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxFeedSniffBytes))
}

func IsYouTubeFeedURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && isYouTubeHost(u.Host) && u.Path == "/feeds/videos.xml" && u.RawQuery != ""
}

const (
	youtubeOutageMinFailing       = 3
	youtubeOutageLongest          = 48 * time.Hour
	YouTubeMutedAfterLoneFailures = 48
)

func YouTubeOutageCutoff(now time.Time) string {
	return now.Add(-youtubeOutageLongest).UTC().Format(time.RFC3339)
}

func YouTubeFeedsDown(total, failing int64) bool {
	return failing >= youtubeOutageMinFailing && failing*2 >= total
}

func IsYouTubeHandleURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && isYouTubeHost(u.Host) && strings.HasPrefix(u.Path, "/@")
}

func isYouTubeHost(host string) bool {
	host = strings.ToLower(host)
	return host == "youtube.com" || host == "www.youtube.com"
}

func ytFeedChannel(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Path != "/feeds/videos.xml" {
		return ""
	}
	if !isYouTubeHost(u.Host) {
		return ""
	}
	q := u.Query()
	m := ytChannelID.FindStringSubmatch(q.Get("channel_id"))
	if m == nil {
		m = ytLongFormPlaylistID.FindStringSubmatch(q.Get("playlist_id"))
	}
	if m == nil {
		return ""
	}
	return m[1]
}
