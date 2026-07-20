// Package discovercrawl crawls followed DIDs' repos for reader-network
// subscription and share records (SPEC <discovery> "Personal" acquisition).
package discovercrawl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"golang.org/x/sync/singleflight"

	"morgenblau/internal/atxrpc"
	"morgenblau/internal/feedkey"
	"morgenblau/internal/lexicon"
	"morgenblau/internal/standardfeed"
)

// skyreader and glean are read-only foreign lexicons consumed here for discovery only; blue.morgen and site.standard are also written elsewhere (SPEC <discovery>).
const (
	morgenSubscriptionCollection    = lexicon.Subscription
	skyreaderSubscriptionCollection = "app.skyreader.feed.subscription"
	gleanSubscriptionCollection     = "at.glean.subscription"
	standardSubscriptionCollection  = standardfeed.CollectionSubscription

	defaultCrawlTimeout         = 20 * time.Second
	maxListRecordsPages         = 100
	maxListRecordsRecords       = 10_000
	maxListRecordsResponseBytes = 1 << 20
)

var errListRecordsResponseTooLarge = errors.New("discovercrawl: listRecords response too large")

var subscriptionCollections = [...]string{
	morgenSubscriptionCollection,
	skyreaderSubscriptionCollection,
	gleanSubscriptionCollection,
	standardSubscriptionCollection,
}

// Subscription is one decoded, canonically-keyed subscription from a
// followed DID's repo. Key reuses Tier-2's canonical source-key scheme
// (feedkey.Normalize'd for rss, DID-normalized publication uri for standardfeed) so cross-reader variants collapse together.
type Subscription struct {
	Key     string
	Kind    string // "rss" | "standardfeed"
	Title   string
	SiteURL string
	// CreatedAt drives ranking's recency lean (SPEC <discovery>); empty if the record didn't carry one.
	CreatedAt string
}

// Resolver turns a followed DID into a PDS endpoint; production wiring must use the SSRF-guarded directory (internal/atidentity), never identity.DefaultDirectory.
type Resolver interface {
	LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error)
}

// PublicationResolver resolves a publication at-uri to canonical form; *standardfeed.Client satisfies it, keeping canonicalization to one implementation shared with the subscribe flow.
type PublicationResolver interface {
	GetPublication(ctx context.Context, uri string) (*standardfeed.Publication, error)
}

// Client crawls followed repos for subscription records, unauthenticated; httpClient must be SSRF-safe since PDS endpoints are attacker-influenceable.
type Client struct {
	resolver Resolver
	http     *http.Client
	standard PublicationResolver
	verifier WellKnownFetcher
	leaflet  LeafletResolver

	now              func() time.Time
	crawlTimeout     time.Duration
	resolveGroup     singleflight.Group
	resolutionReader ResolutionCacheReader
	resolutionWriter ResolutionCacheWriter
}

func NewClient(resolver Resolver, httpClient *http.Client, standard PublicationResolver, verifier WellKnownFetcher, leaflet LeafletResolver) *Client {
	return &Client{
		resolver:     resolver,
		http:         httpClient,
		standard:     standard,
		verifier:     verifier,
		leaflet:      leaflet,
		crawlTimeout: defaultCrawlTimeout,
	}
}

func (c *Client) apiClient(endpoint string) *atclient.APIClient {
	return atxrpc.New(endpoint, responseLimitedClient(c.http, maxListRecordsResponseBytes))
}

func (c *Client) withCrawlTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := c.crawlTimeout
	if timeout <= 0 {
		timeout = defaultCrawlTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

// Crawl fetches did's subscription records across all four lexicons and returns the deduped set; malformed or unknown records are skipped and logged, never fatal.
func (c *Client) Crawl(ctx context.Context, did syntax.DID) ([]Subscription, error) {
	ctx, cancel := c.withCrawlTimeout(ctx)
	defer cancel()

	ident, err := c.resolver.LookupDID(ctx, did)
	if err != nil {
		return nil, fmt.Errorf("discovercrawl: resolve %s: %w", did, err)
	}
	endpoint := ident.PDSEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("discovercrawl: no PDS endpoint for %s", did)
	}
	client := c.apiClient(endpoint)
	repo := did.String()

	pubCache := make(map[string]Subscription)
	seen := make(map[string]Subscription)
	for _, coll := range subscriptionCollections {
		records, err := pageRecords(ctx, client, repo, coll)
		if err != nil {
			return nil, fmt.Errorf("discovercrawl: list %s for %s: %w", coll, did, err)
		}
		for _, r := range records {
			sub, ok := c.decode(ctx, coll, r, pubCache)
			if !ok {
				continue
			}
			if _, dup := seen[sub.Key]; dup {
				continue
			}
			seen[sub.Key] = sub
		}
	}

	out := make([]Subscription, 0, len(seen))
	for _, s := range seen {
		out = append(out, s)
	}
	return out, nil
}

func (c *Client) decode(ctx context.Context, collection string, r recordEntry, pubCache map[string]Subscription) (Subscription, bool) {
	switch collection {
	case morgenSubscriptionCollection:
		return c.decodeMorgen(ctx, r, pubCache)
	case skyreaderSubscriptionCollection, gleanSubscriptionCollection:
		return c.decodeFeedURLRecord(ctx, collection, r, pubCache)
	case standardSubscriptionCollection:
		publication, _ := r.Value["publication"].(string)
		if publication == "" {
			slog.Warn("discovercrawl: skipping standard subscription without publication", "uri", r.URI)
			return Subscription{}, false
		}
		createdAt, _ := r.Value["createdAt"].(string)
		sub, ok := c.resolvePublication(ctx, publication, "", pubCache)
		if ok {
			sub.CreatedAt = createdAt
		}
		return sub, ok
	default:
		return Subscription{}, false
	}
}

// decodeFeedURLRecord decodes skyreader/glean records; a record without feedUrl (e.g. Skyreader's atproto.* variants) isn't an RSS subscription and is skipped rather than guessed at. An at:// feedUrl is these readers' way of mirroring a standardfeed subscription, so it's resolved as a publication rather than minted as a literal RSS key.
func (c *Client) decodeFeedURLRecord(ctx context.Context, collection string, r recordEntry, pubCache map[string]Subscription) (Subscription, bool) {
	feedURL, _ := r.Value["feedUrl"].(string)
	if feedURL == "" {
		slog.Warn("discovercrawl: skipping subscription without feedUrl", "collection", collection, "uri", r.URI)
		return Subscription{}, false
	}
	createdAt, _ := r.Value["createdAt"].(string)
	if strings.HasPrefix(feedURL, "at://") {
		sub, ok := c.resolvePublication(ctx, feedURL, "", pubCache)
		if ok {
			sub.CreatedAt = createdAt
		}
		return sub, ok
	}
	title, _ := r.Value["title"].(string)
	siteURL, _ := r.Value["siteUrl"].(string)
	return Subscription{Key: feedkey.Normalize(feedURL), Kind: "rss", Title: title, SiteURL: siteURL, CreatedAt: createdAt}, true
}

// decodeMorgen dispatches on the required open union (SPEC <lexicons>); unknown variants are skipped and logged, mirroring sync.toPDSSubscription.
func (c *Client) decodeMorgen(ctx context.Context, r recordEntry, pubCache map[string]Subscription) (Subscription, bool) {
	source, ok := r.Value["source"].(map[string]any)
	if !ok {
		slog.Warn("discovercrawl: skipping blue.morgen subscription without source union", "uri", r.URI)
		return Subscription{}, false
	}
	title, _ := r.Value["title"].(string)
	createdAt, _ := r.Value["createdAt"].(string)
	typ, _ := source["$type"].(string)
	switch typ {
	case lexicon.SourceRSS:
		feedURL, _ := source["feedUrl"].(string)
		if feedURL == "" {
			slog.Warn("discovercrawl: skipping rssFeed subscription without feedUrl", "uri", r.URI)
			return Subscription{}, false
		}
		if strings.HasPrefix(feedURL, "at://") {
			sub, ok := c.resolvePublication(ctx, feedURL, title, pubCache)
			if ok {
				sub.CreatedAt = createdAt
			}
			return sub, ok
		}
		siteURL, _ := source["siteUrl"].(string)
		return Subscription{Key: feedkey.Normalize(feedURL), Kind: "rss", Title: title, SiteURL: siteURL, CreatedAt: createdAt}, true
	case lexicon.SourceStandard:
		publication, _ := source["publication"].(string)
		if publication == "" {
			slog.Warn("discovercrawl: skipping standardPublication subscription without publication", "uri", r.URI)
			return Subscription{}, false
		}
		sub, ok := c.resolvePublication(ctx, publication, title, pubCache)
		if ok {
			sub.CreatedAt = createdAt
		}
		return sub, ok
	default:
		slog.Warn("discovercrawl: skipping subscription with unknown source variant", "uri", r.URI, "type", typ)
		return Subscription{}, false
	}
}

type listRecordsResp struct {
	Records []recordEntry `json:"records"`
	Cursor  string        `json:"cursor"`
}

type recordEntry struct {
	URI   string         `json:"uri"`
	CID   string         `json:"cid"`
	Value map[string]any `json:"value"`
}

// listRecordsClient is the seam pageRecords uses so tests can swap in a fake.
type listRecordsClient interface {
	Get(ctx context.Context, endpoint syntax.NSID, params map[string]any, out any) error
}

// pageRecords follows cursors until the cursor is empty; an empty page with a non-empty cursor is still a valid continuation.
func pageRecords(ctx context.Context, client listRecordsClient, repo, collection string) ([]recordEntry, error) {
	var (
		out    []recordEntry
		cursor string
		pages  int
	)
	seenCursors := make(map[string]struct{})
	for {
		if pages >= maxListRecordsPages {
			return nil, fmt.Errorf("discovercrawl: listRecords page limit reached for %s", collection)
		}
		var resp listRecordsResp
		params := map[string]any{
			"repo":       repo,
			"collection": collection,
			"limit":      100,
		}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := client.Get(ctx, syntax.NSID("com.atproto.repo.listRecords"), params, &resp); err != nil {
			return nil, err
		}
		pages++
		if len(resp.Records) > maxListRecordsRecords-len(out) {
			return nil, fmt.Errorf("discovercrawl: listRecords record limit reached for %s", collection)
		}
		out = append(out, resp.Records...)
		if resp.Cursor == "" {
			return out, nil
		}
		if _, repeated := seenCursors[resp.Cursor]; repeated {
			return nil, fmt.Errorf("discovercrawl: listRecords repeated cursor for %s", collection)
		}
		seenCursors[resp.Cursor] = struct{}{}
		cursor = resp.Cursor
	}
}

type responseLimitRoundTripper struct {
	base  http.RoundTripper
	limit int64
}

func responseLimitedClient(client *http.Client, limit int64) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	clone := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	clone.Transport = responseLimitRoundTripper{base: base, limit: limit}
	return &clone
}

func (t responseLimitRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp.ContentLength > t.limit {
		resp.Body.Close()
		return nil, fmt.Errorf("%w: limit is %d bytes", errListRecordsResponseTooLarge, t.limit)
	}
	resp.Body = &limitedResponseBody{body: resp.Body, remaining: t.limit}
	return resp, nil
}

type limitedResponseBody struct {
	body      io.ReadCloser
	remaining int64
}

func (b *limitedResponseBody) Read(p []byte) (int, error) {
	if b.remaining > 0 {
		if int64(len(p)) > b.remaining {
			p = p[:b.remaining]
		}
		n, err := b.body.Read(p)
		b.remaining -= int64(n)
		return n, err
	}

	var probe [1]byte
	n, err := b.body.Read(probe[:])
	if n > 0 {
		return 0, errListRecordsResponseTooLarge
	}
	return 0, err
}

func (b *limitedResponseBody) Close() error {
	return b.body.Close()
}
