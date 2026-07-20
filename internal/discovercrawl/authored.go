package discovercrawl

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/standardfeed"
)

// authoredPublicationCollection is the only reader-network collection with an authorship concept; blue.morgen has no "I publish this feed" record. SPEC <discovery> Signal ordering.
const authoredPublicationCollection = standardfeed.CollectionPublication

// standardDocumentCollectionForLatest aliases standardfeed.CollectionDocument for test readability.
const standardDocumentCollectionForLatest = standardfeed.CollectionDocument

// verificationTimeout bounds a single well-known probe; a var (not const) so tests can shrink it instead of sleeping for real. One black-holed site must not stall the whole crawl inside the /api/discover/sources request.
var verificationTimeout = 3 * time.Second

// WellKnownFetcher probes a site's declared publication at-uri; *standardfeed.Client satisfies it.
type WellKnownFetcher interface {
	FetchWellKnown(ctx context.Context, siteURL string) (string, error)
}

// authorshipOutcome classifies a well-known probe against a claimed publication record.
type authorshipOutcome int

const (
	outcomeVerified authorshipOutcome = iota
	outcomeMismatch
	outcomeProbeError
)

// verifiedOutcome is the only Verification value CrawlAuthoredPublications ever emits today; mismatch and probe-error are dropped before they reach AuthoredPublication, not stored as a lesser signal. Shared with the persistence layer (authored_store.go) so a future retry policy has a stable string to key off.
const verifiedOutcome = "verified"

// AuthoredPublication is one publication a followed DID owns in their own repo, the strongest discovery signal a source can carry. SPEC <social-layer> authorship signal.
// LastPublishedAt is a best-effort recency proxy: the newest site.standard.document anywhere in the repo, not scoped to this specific publication; empty when unknown.
type AuthoredPublication struct {
	Key             string
	Kind            string // always "standardfeed"
	Title           string
	SiteURL         string
	LastPublishedAt string
	Verification    string
}

// CrawlAuthoredPublications lists did's own authored (not subscribed) publications, paired with a best-effort recency proxy; malformed records are skipped and logged, never fatal.
func (c *Client) CrawlAuthoredPublications(ctx context.Context, did syntax.DID) ([]AuthoredPublication, error) {
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

	records, err := pageRecords(ctx, client, repo, authoredPublicationCollection)
	if err != nil {
		return nil, fmt.Errorf("discovercrawl: list %s for %s: %w", authoredPublicationCollection, did, err)
	}
	if len(records) == 0 {
		return nil, nil
	}

	lastPublishedAt := latestDocumentPublishedAt(ctx, client, repo)

	out := make([]AuthoredPublication, 0, len(records))
	for _, r := range records {
		name, _ := r.Value["name"].(string)
		url, _ := r.Value["url"].(string)
		if name == "" || url == "" {
			slog.Warn("discovercrawl: skipping publication without name/url", "uri", r.URI)
			continue
		}
		rkey := atprepo.RkeyFromATURI(r.URI)
		switch verifyAuthorship(ctx, c.verifier, url, rkey, did, ident.Handle) {
		case outcomeMismatch:
			slog.Warn("discovercrawl: authorship claim does not match site's well-known", "uri", r.URI, "site", url)
			continue
		case outcomeProbeError:
			slog.Warn("discovercrawl: well-known probe failed", "uri", r.URI, "site", url)
			continue
		}
		key := "at://" + repo + "/" + authoredPublicationCollection + "/" + rkey
		out = append(out, AuthoredPublication{
			Key:             key,
			Kind:            "standardfeed",
			Title:           name,
			SiteURL:         url,
			LastPublishedAt: lastPublishedAt,
			Verification:    verifiedOutcome,
		})
	}
	return out, nil
}

// verifyAuthorship probes siteURL's declared publication and checks it names this exact record (component-wise, since a site may legitimately serve either the DID-form or handle-form uri). Mismatch and probe-error both drop the record today but stay distinguishable in code for a future retry policy (a transient probe failure deserves a sooner retry than a site's declared answer disagreeing).
func verifyAuthorship(ctx context.Context, verifier WellKnownFetcher, siteURL, wantRkey string, did syntax.DID, handle syntax.Handle) authorshipOutcome {
	probeCtx, cancel := context.WithTimeout(ctx, verificationTimeout)
	defer cancel()
	declared, err := verifier.FetchWellKnown(probeCtx, siteURL)
	if err != nil {
		return outcomeProbeError
	}
	if declared == "" {
		return outcomeMismatch
	}
	uri, err := syntax.ParseATURI(declared)
	if err != nil {
		return outcomeMismatch
	}
	if uri.Collection().String() != authoredPublicationCollection || uri.RecordKey().String() != wantRkey {
		return outcomeMismatch
	}
	authority := strings.ToLower(uri.Authority().String())
	if authority != strings.ToLower(did.String()) && authority != strings.ToLower(handle.String()) {
		return outcomeMismatch
	}
	return outcomeVerified
}

// latestDocumentPublishedAt fetches the single newest site.standard.document (listRecords defaults newest-first, limit 1) as a cheap recency proxy; a full per-publication listing would be too expensive for a signal that only leans the score.
// Best-effort: failure or an empty repo yields "", read by the ranking engine as neutral unknown recency.
func latestDocumentPublishedAt(ctx context.Context, client listRecordsClient, repo string) string {
	var resp listRecordsResp
	params := map[string]any{
		"repo":       repo,
		"collection": standardDocumentCollectionForLatest,
		"limit":      1,
	}
	if err := client.Get(ctx, syntax.NSID("com.atproto.repo.listRecords"), params, &resp); err != nil {
		slog.Warn("discovercrawl: latest document lookup failed", "did", repo, "err", err)
		return ""
	}
	if len(resp.Records) == 0 {
		return ""
	}
	publishedAt, _ := resp.Records[0].Value["publishedAt"].(string)
	return publishedAt
}
