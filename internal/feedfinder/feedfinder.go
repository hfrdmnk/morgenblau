// Package feedfinder turns any pasted URL into a list of feed candidates.
// Hides the strategy stack: HTML link-rel-alternate, YouTube channel mapping,
// Apple Podcasts iTunes Lookup. Each candidate carries the canonical feedUrl,
// a best-guess title, and the originating site URL.
package feedfinder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Candidate is one feed the user could subscribe to. All fields are best-
// effort; only FeedURL is guaranteed non-empty.
type Candidate struct {
	FeedURL     string `json:"feedUrl"`
	Title       string `json:"title,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	SiteURL     string `json:"siteUrl,omitempty"`
}

// HTTPDoer is the minimal slice of *http.Client the finder uses. Production
// wires the polite fetcher; tests inject a canned RoundTripper.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Finder resolves URLs to feed candidates.
type Finder struct {
	client HTTPDoer
	// itunesBase is overridable in tests so we don't have to hit Apple.
	itunesBase string
}

// New builds a Finder using the given HTTP client.
func New(client HTTPDoer) *Finder {
	return &Finder{client: client, itunesBase: "https://itunes.apple.com/lookup"}
}

// WithITunesBase swaps the iTunes Lookup base URL — for tests.
func (f *Finder) WithITunesBase(base string) *Finder {
	f.itunesBase = base
	return f
}

// feed content types we recognize as "this URL is already a feed."
var feedContentTypes = map[string]struct{}{
	"application/rss+xml":   {},
	"application/atom+xml":  {},
	"application/feed+json": {},
	"application/json":      {},
	"text/xml":              {},
	"application/xml":       {},
}

// Resolve walks the strategy stack and returns candidates. An unresolvable
// URL returns nil, nil — empty list, no error. Network failures bubble up.
func (f *Finder) Resolve(ctx context.Context, raw string) ([]Candidate, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme == "" {
		u.Scheme = "https"
		raw = u.String()
	}

	// Strategy 2 (YouTube) and 3 (Apple Podcasts) are recognized before
	// fetching because they have stable URL shapes — saves a round-trip
	// when we already know what to do.
	if host := strings.ToLower(u.Host); strings.HasSuffix(host, "youtube.com") || host == "youtu.be" {
		c, err := f.resolveYouTube(ctx, u)
		if err != nil {
			return nil, err
		}
		if c != nil {
			return []Candidate{*c}, nil
		}
	}
	if strings.Contains(strings.ToLower(u.Host), "podcasts.apple.com") {
		c, err := f.resolveApplePodcasts(ctx, u)
		if err != nil {
			return nil, err
		}
		if c != nil {
			return []Candidate{*c}, nil
		}
	}

	// Strategy 1 (and passthrough): fetch the URL, inspect content-type and
	// HTML link-rel-alternate.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	ct := strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0])
	if _, isFeed := feedContentTypes[strings.ToLower(ct)]; isFeed {
		// Passthrough — user pasted a direct feed URL.
		return []Candidate{{FeedURL: raw, ContentType: ct, SiteURL: raw}}, nil
	}

	base := u
	if resp.Request != nil && resp.Request.URL != nil {
		base = resp.Request.URL
	}
	return f.extractLinkRels(resp.Body, base)
}

func (f *Finder) extractLinkRels(body io.Reader, base *url.URL) ([]Candidate, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
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
	return out, nil
}

// --- YouTube ---

// channelIDRe matches the `?channel_id=...` query feed shape we emit.
var ytChannelPath = regexp.MustCompile(`^/channel/([A-Za-z0-9_-]+)$`)
var ytChannelIDInHTML = regexp.MustCompile(`"channelId":"(UC[A-Za-z0-9_-]{20,})"`)

func (f *Finder) resolveYouTube(ctx context.Context, u *url.URL) (*Candidate, error) {
	path := u.Path
	// Direct /channel/<id> — no fetch required.
	if m := ytChannelPath.FindStringSubmatch(path); m != nil {
		return ytFeedCandidate(m[1], u), nil
	}

	// /@handle, /c/<name>, /user/<name> — pull the page and grep for channelId.
	switch {
	case strings.HasPrefix(path, "/@"),
		strings.HasPrefix(path, "/c/"),
		strings.HasPrefix(path, "/user/"):
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		resp, err := f.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return nil, err
		}
		if m := ytChannelIDInHTML.FindSubmatch(body); m != nil {
			return ytFeedCandidate(string(m[1]), u), nil
		}
	}
	return nil, nil
}

func ytFeedCandidate(channelID string, u *url.URL) *Candidate {
	return &Candidate{
		FeedURL:     "https://www.youtube.com/feeds/videos.xml?channel_id=" + channelID,
		ContentType: "application/atom+xml",
		SiteURL:     u.String(),
		Title:       "",
	}
}

// --- Apple Podcasts ---

var applePodcastsIDRe = regexp.MustCompile(`/id(\d+)`)

type itunesLookupResp struct {
	Results []struct {
		FeedURL    string `json:"feedUrl"`
		Title      string `json:"collectionName"`
		ArtistName string `json:"artistName"`
	} `json:"results"`
}

func (f *Finder) resolveApplePodcasts(ctx context.Context, u *url.URL) (*Candidate, error) {
	m := applePodcastsIDRe.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, nil
	}
	lookupURL := f.itunesBase + "?id=" + m[1]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, lookupURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var parsed itunesLookupResp
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("itunes lookup: %w", err)
	}
	if len(parsed.Results) == 0 || parsed.Results[0].FeedURL == "" {
		return nil, nil
	}
	r := parsed.Results[0]
	return &Candidate{
		FeedURL:     r.FeedURL,
		Title:       r.Title,
		ContentType: "application/rss+xml",
		SiteURL:     u.String(),
	}, nil
}
