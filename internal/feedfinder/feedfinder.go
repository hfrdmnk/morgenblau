// Package feedfinder turns a pasted URL into feed candidates via HTML link-rel and YouTube channel mapping.
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

	"github.com/PuerkitoBio/goquery"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/mmcdole/gofeed"

	"morgenblau/internal/safehttp"
	"morgenblau/internal/standardfeed"
)

// maxFeedSniffBytes caps body reads on the passthrough path against a hostile or huge feed.
const maxFeedSniffBytes = 4 << 20

// Candidate is one feed to subscribe to; rss kind guarantees FeedURL, standardfeed kind guarantees Publication and carries no FeedURL.
type Candidate struct {
	FeedURL     string `json:"feedUrl,omitempty"`
	Kind        string `json:"kind,omitempty"` // "" (rss) | "standardfeed"
	Publication string `json:"publication,omitempty"`
	Title       string `json:"title,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	SiteURL     string `json:"siteUrl,omitempty"`
}

// HTTPDoer is the minimal *http.Client method set the finder needs.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// StandardResolver lives on Finder because the article-link probe needs it mid-flow: document → site → publication.
type StandardResolver interface {
	GetPublication(ctx context.Context, uri string) (*standardfeed.Publication, error)
	GetDocument(ctx context.Context, uri string) (*standardfeed.Document, error)
	FetchWellKnown(ctx context.Context, siteURL string) (string, error)
}

// Finder resolves URLs to feed candidates.
type Finder struct {
	client HTTPDoer
	// standard enables the Standardfeed probes; nil turns them off.
	standard StandardResolver
}

// New builds a Finder using the given HTTP client.
func New(client HTTPDoer) *Finder {
	return &Finder{client: client}
}

// WithStandardResolver enables Standardfeed publication discovery (at-uri passthrough, well-known probe, article link-tag chain).
func (f *Finder) WithStandardResolver(r StandardResolver) *Finder {
	f.standard = r
	return f
}

// newGET builds a GET request identified as Morgenblau's outbound bot traffic.
func newGET(ctx context.Context, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", safehttp.UserAgent)
	return req, nil
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

// Resolve walks the strategy stack; an unresolvable URL returns nil, nil while network failures bubble up.
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

	// YouTube is checked before fetching since its URL shape is stable, avoiding a round-trip.
	if host := strings.ToLower(u.Host); strings.HasSuffix(host, "youtube.com") || host == "youtu.be" {
		c, err := f.resolveYouTube(ctx, u)
		if err != nil {
			return nil, err
		}
		if c != nil {
			return []Candidate{*c}, nil
		}
	}

	// The well-known probe runs concurrently with the HTML fetch below and merges in after link-rel extraction.
	// It's buffered so an early return (passthrough, error) never blocks the goroutine.
	var wellKnown chan string
	if f.standard != nil {
		origin := (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
		wellKnown = make(chan string, 1)
		go func() {
			uri, err := f.standard.FetchWellKnown(ctx, origin)
			if err != nil {
				uri = "" // best-effort probe: transport error reads as a miss
			}
			wellKnown <- uri
		}()
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

	ct := strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0])
	if _, isFeed := feedContentTypes[strings.ToLower(ct)]; isFeed {
		// Passthrough: parse the body for the canonical title so the dialog can prefill.
		return []Candidate{{FeedURL: raw, ContentType: ct, SiteURL: raw, Title: sniffFeedTitle(resp.Body)}}, nil
	}

	base := u
	if resp.Request != nil && resp.Request.URL != nil {
		base = resp.Request.URL
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	out := extractLinkRels(doc, base)
	if f.standard != nil {
		var pubURIs []string
		if wkURI := <-wellKnown; wkURI != "" {
			pubURIs = append(pubURIs, wkURI)
		}
		// Article pages carry <link rel="site.standard.document">; chase document → site → publication.
		if docURI := extractStandardDocLink(doc); docURI != "" {
			if sdoc, err := f.standard.GetDocument(ctx, docURI); err == nil && strings.HasPrefix(sdoc.Site, "at://") {
				pubURIs = append(pubURIs, sdoc.Site)
			}
		}
		out = append(out, f.publicationCandidates(ctx, pubURIs)...)
	}
	return out, nil
}

// resolveATURI resolves a pasted at-uri: publication directly, document via its site, other collections yield nothing.
// Errors bubble up here, unlike the best-effort probes elsewhere, since an explicit paste deserves feedback.
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
			// A loose document's site is an https URL, so there's no publication to resolve.
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

// publicationCandidates dedupes by DID-normalized URI, since well-known may return the handle form while a document's site uses the DID form; failures are skipped as probes stay best-effort.
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

// extractStandardDocLink pulls the first <link rel="site.standard.document"> at-uri from an article page.
func extractStandardDocLink(doc *goquery.Document) string {
	href, _ := doc.Find("link[rel='site.standard.document']").First().Attr("href")
	href = strings.TrimSpace(href)
	if !strings.HasPrefix(href, "at://") {
		return ""
	}
	return href
}

// sniffFeedTitle returns the feed's title, or empty if the body can't be read or parsed.
func sniffFeedTitle(body io.Reader) string {
	buf, err := io.ReadAll(io.LimitReader(body, maxFeedSniffBytes))
	if err != nil {
		return ""
	}
	parsed, err := gofeed.NewParser().Parse(bytes.NewReader(buf))
	if err != nil || parsed == nil {
		return ""
	}
	return strings.TrimSpace(parsed.Title)
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

// --- YouTube ---

// ytChannelPath matches a /channel/<id> URL path.
var ytChannelPath = regexp.MustCompile(`^/channel/([A-Za-z0-9_-]+)$`)

// ytChannelIDInHTML matches the channel ID via JS blob, canonical /channel/ link, or channel_id= feed link.
// The cookie-less consent-bypass page only has the latter two, so all three forms are needed.
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
			// Error pages can incidentally contain UC… strings; don't sniff them.
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil, nil
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return nil, err
		}
		// First match wins: canonical link, og:url, and feed link precede user content on every YouTube shape seen.
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

// fetchYTFeedTitle is best-effort: any error returns an empty string and the dialog falls back to the feed URL.
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
	return sniffFeedTitle(resp.Body)
}

func ytFeedCandidate(channelID string, u *url.URL) *Candidate {
	return &Candidate{
		FeedURL:     "https://www.youtube.com/feeds/videos.xml?channel_id=" + channelID,
		ContentType: "application/atom+xml",
		SiteURL:     u.String(),
	}
}
