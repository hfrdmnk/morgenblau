package discovercrawl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/backoff"
	"morgenblau/internal/database/db"
	"morgenblau/internal/feedkey"
	"morgenblau/internal/leafletfeed"
	"morgenblau/internal/standardfeed"
)

// resolutionBackoff paces retries on transient publication-resolve failures (SPEC <discovery>): 1h,2h,4h,...,168h cap.
var resolutionBackoff = backoff.Policy{Steps: backoff.Exponential(time.Hour, 2, 7*24*time.Hour)}

// resolutionRecheckTTL is the flat (non-exponential) TTL for both a success and a deterministic skip: a skipped pub may become discoverable later.
const resolutionRecheckTTL = 24 * time.Hour

// resolvedPublication is dispatchResolve's collection-agnostic result.
type resolvedPublication struct {
	Key     string
	Kind    string
	Title   string
	SiteURL string
	IconURL string
}

// LeafletResolver resolves a pub.leaflet.publication at-uri; *leafletfeed.Client satisfies it.
type LeafletResolver interface {
	GetPublication(ctx context.Context, uri string) (*leafletfeed.Publication, error)
}

// ResolutionCacheReader is the read side of the DB-backed publication resolution cache; wire it to the reader pool.
type ResolutionCacheReader interface {
	GetDiscoverPublicationResolution(ctx context.Context, publicationUri string) (db.DiscoverPublicationResolution, error)
}

// ResolutionCacheWriter is the write side: a single-statement upsert, never called from inside a transaction.
type ResolutionCacheWriter interface {
	UpsertDiscoverPublicationResolution(ctx context.Context, arg db.UpsertDiscoverPublicationResolutionParams) error
}

// WithResolutionCache wires the DB-backed resolution cache; without it, resolveCached still dispatches network calls but never persists an outcome.
func (c *Client) WithResolutionCache(r ResolutionCacheReader, w ResolutionCacheWriter) *Client {
	c.resolutionReader = r
	c.resolutionWriter = w
	return c
}

func (c *Client) clock() time.Time {
	if c.now == nil {
		return time.Now()
	}
	return c.now()
}

// isRecordNotFound reports whether err is the XRPC "RecordNotFound" response (HTTP 400); any other error is treated as transient.
func isRecordNotFound(err error) bool {
	var apiErr *atclient.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusBadRequest && apiErr.Name == "RecordNotFound"
}

// isResolvablePublicationCollection reports whether coll is a collection dispatchResolve resolves publications from.
func isResolvablePublicationCollection(coll string) bool {
	return coll == standardfeed.CollectionPublication || coll == leafletfeed.CollectionPublication
}

// dispatchResolve maps uri to a resolvedPublication; ok=false,err=nil is a deterministic skip, err!=nil is a transient failure to back off and retry.
func (c *Client) dispatchResolve(ctx context.Context, uri syntax.ATURI, rawURI string) (resolvedPublication, bool, error) {
	switch uri.Collection().String() {
	case standardfeed.CollectionPublication:
		return c.resolveStandard(ctx, rawURI)
	case leafletfeed.CollectionPublication:
		return c.resolveLeafletSibling(ctx, uri, rawURI)
	default:
		return resolvedPublication{}, false, nil
	}
}

func (c *Client) resolveStandard(ctx context.Context, rawURI string) (resolvedPublication, bool, error) {
	if c.standard == nil {
		return resolvedPublication{}, false, nil
	}
	pub, err := c.standard.GetPublication(ctx, rawURI)
	if err != nil {
		return resolvedPublication{}, false, err
	}
	if !pub.ShowInDiscover {
		return resolvedPublication{}, false, nil
	}
	return resolvedPublication{Key: pub.URI, Kind: "standardfeed", Title: pub.Name, SiteURL: pub.URL, IconURL: pub.IconURL}, true, nil
}

// resolveLeafletSibling prefers the dual-written site.standard.publication sibling, falling back to the leaflet record's RSS feed only when the sibling is definitively absent (SPEC <lexicons> pub.leaflet.publication).
func (c *Client) resolveLeafletSibling(ctx context.Context, uri syntax.ATURI, rawURI string) (resolvedPublication, bool, error) {
	if c.standard != nil {
		sibling := fmt.Sprintf("at://%s/%s/%s", uri.Authority(), standardfeed.CollectionPublication, uri.RecordKey())
		pub, err := c.standard.GetPublication(ctx, sibling)
		switch {
		case err == nil:
			// Publisher opt-out is a deterministic skip; the 24h recheck TTL picks up a later opt-in.
			if !pub.ShowInDiscover {
				return resolvedPublication{}, false, nil
			}
			return resolvedPublication{Key: pub.URI, Kind: "standardfeed", Title: pub.Name, SiteURL: pub.URL, IconURL: pub.IconURL}, true, nil
		case !isRecordNotFound(err):
			// Never fall back to leaflet on a transient error: a PDS outage must not misclassify a mirrored pub as RSS.
			return resolvedPublication{}, false, err
		}
	}
	if c.leaflet == nil {
		return resolvedPublication{}, false, nil
	}
	pub, err := c.leaflet.GetPublication(ctx, rawURI)
	if err != nil {
		return resolvedPublication{}, false, err
	}
	if !pub.ShowInDiscover || pub.FeedURL == "" {
		return resolvedPublication{}, false, nil
	}
	return resolvedPublication{Key: feedkey.Normalize(pub.FeedURL), Kind: "rss", Title: pub.Name, SiteURL: pub.URL}, true, nil
}

// cachedResolution is what resolveCached's singleflight group shares across collapsed callers.
type cachedResolution struct {
	pub resolvedPublication
	ok  bool
}

// resolveCached serves rawURI from the DB-backed cache when fresh, otherwise dispatches and persists; concurrent calls for the same uri collapse to one dispatch.
func (c *Client) resolveCached(ctx context.Context, rawURI string, uri syntax.ATURI) (resolvedPublication, bool) {
	v, _, _ := c.resolveGroup.Do(rawURI, func() (any, error) {
		pub, ok := c.resolveCachedOnce(ctx, rawURI, uri)
		return cachedResolution{pub, ok}, nil
	})
	r := v.(cachedResolution)
	return r.pub, r.ok
}

func (c *Client) resolveCachedOnce(ctx context.Context, rawURI string, uri syntax.ATURI) (resolvedPublication, bool) {
	now := c.clock()

	var (
		row     db.DiscoverPublicationResolution
		haveRow bool
	)
	if c.resolutionReader != nil {
		r, err := c.resolutionReader.GetDiscoverPublicationResolution(ctx, rawURI)
		switch {
		case err == nil:
			row, haveRow = r, true
		case errors.Is(err, sql.ErrNoRows):
			// Never resolved before: falls through to a fresh attempt below.
		default:
			slog.Warn("discovercrawl: resolution cache read failed", "uri", rawURI, "err", err)
		}
	}

	if haveRow {
		if nextRetry, perr := time.Parse(time.RFC3339, row.NextRetryAt); perr == nil && now.Before(nextRetry) {
			return rowToResolution(row)
		}
	}

	pub, ok, err := c.dispatchResolve(ctx, uri, rawURI)
	switch {
	case err != nil:
		if ctx.Err() != nil {
			return resolvedPublication{}, false
		}
		slog.Warn("discovercrawl: publication resolve failed", "uri", rawURI, "err", err)
		var failures int64 = 1
		if haveRow {
			failures = row.FailureCount + 1
		}
		next := now.Add(resolutionBackoff.Delay(int(failures)))
		if haveRow {
			// Stale-while-error: preserve the prior payload, a blip never un-resolves a known-good pub.
			c.upsertResolution(ctx, rawURI, row.CanonicalKey, row.Kind, row.Title, row.SiteUrl, row.IconUrl, failures, now, next)
			return rowToResolution(row)
		}
		c.upsertResolution(ctx, rawURI, nil, nil, nil, nil, nil, failures, now, next)
		return resolvedPublication{}, false
	case ok:
		c.upsertResolution(ctx, rawURI, nilIfEmpty(pub.Key), nilIfEmpty(pub.Kind), nilIfEmpty(pub.Title), nilIfEmpty(pub.SiteURL), nilIfEmpty(pub.IconURL), 0, now, now.Add(resolutionRecheckTTL))
		return pub, true
	default:
		c.upsertResolution(ctx, rawURI, nil, nil, nil, nil, nil, 0, now, now.Add(resolutionRecheckTTL))
		return resolvedPublication{}, false
	}
}

func rowToResolution(row db.DiscoverPublicationResolution) (resolvedPublication, bool) {
	if row.CanonicalKey == nil {
		return resolvedPublication{}, false
	}
	return resolvedPublication{
		Key:     *row.CanonicalKey,
		Kind:    derefString(row.Kind),
		Title:   derefString(row.Title),
		SiteURL: derefString(row.SiteUrl),
	}, true
}

func (c *Client) upsertResolution(ctx context.Context, uri string, canonicalKey, kind, title, siteURL, iconURL *string, failureCount int64, now, nextRetryAt time.Time) {
	if c.resolutionWriter == nil {
		return
	}
	arg := db.UpsertDiscoverPublicationResolutionParams{
		PublicationUri: uri,
		CanonicalKey:   canonicalKey,
		Kind:           kind,
		Title:          title,
		SiteUrl:        siteURL,
		IconUrl:        iconURL,
		FailureCount:   failureCount,
		FetchedAt:      now.UTC().Format(time.RFC3339),
		NextRetryAt:    nextRetryAt.UTC().Format(time.RFC3339),
	}
	if err := c.resolutionWriter.UpsertDiscoverPublicationResolution(ctx, arg); err != nil {
		slog.Warn("discovercrawl: resolution cache write failed", "uri", uri, "err", err)
	}
}

// resolvePublication resolves rawURI through the per-crawl L1 cache then the DB-backed cache; fallbackTitle is applied per call, never memoized, so two callers get their own title from one resolution.
func (c *Client) resolvePublication(ctx context.Context, rawURI, fallbackTitle string, pubCache map[string]Subscription) (Subscription, bool) {
	if cached, ok := pubCache[rawURI]; ok {
		if cached.Key == "" {
			return Subscription{}, false
		}
		return applyFallbackTitle(cached, fallbackTitle), true
	}

	uri, err := syntax.ParseATURI(rawURI)
	if err != nil || !isResolvablePublicationCollection(uri.Collection().String()) {
		// Unparseable or a collection discovercrawl doesn't resolve publications from; local-only, never hits the DB cache.
		slog.Debug("discovercrawl: skipping unresolvable publication uri", "uri", rawURI)
		pubCache[rawURI] = Subscription{}
		return Subscription{}, false
	}

	pub, ok := c.resolveCached(ctx, rawURI, uri)
	if !ok {
		pubCache[rawURI] = Subscription{}
		return Subscription{}, false
	}
	sub := Subscription{Key: pub.Key, Kind: pub.Kind, Title: pub.Title, SiteURL: pub.SiteURL}
	pubCache[rawURI] = sub
	return applyFallbackTitle(sub, fallbackTitle), true
}

func applyFallbackTitle(sub Subscription, fallbackTitle string) Subscription {
	if fallbackTitle != "" {
		sub.Title = fallbackTitle
	}
	return sub
}
