package discovercrawl

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
)

// RecordEntry is one repo record, as com.atproto.repo.listRecords returns it and as the tap mirror rebuilds it from a stored row.
type RecordEntry = recordEntry

// ShareRecord is a decoded share/sidecar record, pre-merge.
type ShareRecord = shareRecord

// RecommendRecord is a decoded site.standard.graph.recommend record, pre-merge.
type RecommendRecord = recommendRecord

// readerNetworkFollowCollections is the precedence order DecodeFollows walks; CrawlReaderNetworkFollows fetches the same two separately because their failure semantics differ.
var readerNetworkFollowCollections = [...]string{morgenFollowCollection, skyreaderFollowCollection}

// NewRecordEntry bridges a tap mirror row to the crawl's record shape; the mirror keeps a record's identity in columns, so the at-uri listRecords hands back has to be synthesized.
func NewRecordEntry(did, collection, rkey, cid string, value map[string]any) RecordEntry {
	return RecordEntry{
		URI:   "at://" + did + "/" + collection + "/" + rkey,
		CID:   cid,
		Value: value,
	}
}

// DecodeShare decodes one blue.morgen.feed.share or app.skyreader.social.share record.
func DecodeShare(r RecordEntry) (ShareRecord, bool) { return decodeShareRecord(r) }

// DecodeRecommend decodes one site.standard.graph.recommend record.
func DecodeRecommend(r RecordEntry) (RecommendRecord, bool) { return decodeRecommendRecord(r) }

// DecodeFollowSubject reads a follow record's subject DID; every follow lexicon this package reads agrees on the field.
func DecodeFollowSubject(r RecordEntry) (string, bool) { return decodeAdjacentFollow(r) }

// DecodeSave decodes one save row against its collection's lexicon shape; ok is false for an unknown collection or a record missing its url field.
func DecodeSave(collection string, r RecordEntry) (Save, bool) {
	for _, shape := range saveShapes {
		if shape.collection == collection {
			return decodeSave(shape, r)
		}
	}
	return Save{}, false
}

// DecodeSaves folds one repo's save rows, keyed by collection, into the deduped set CrawlSaves returns; saveShapes order decides which lexicon wins a duplicate item url.
func DecodeSaves(byCollection map[string][]RecordEntry) []Save {
	seen := map[string]struct{}{}
	var out []Save
	for _, shape := range saveShapes {
		appendSaves(&out, seen, byCollection[shape.collection], shape)
	}
	return out
}

// MergeShares folds one repo's share/recommend rows, keyed by collection, into the merged set CrawlShares returns: a recommend joins its lazy sidecar by document, standalone shares dedupe by item url.
func MergeShares(byCollection map[string][]RecordEntry) []Share {
	// Document-bearing morgen shares are recommend sidecars; document-less ones are standalone rss shares.
	var rssShares []ShareRecord
	sidecarByDoc := map[string]ShareRecord{}
	for _, r := range byCollection[morgenShareCollection] {
		sr, ok := decodeShareRecord(r)
		if !ok {
			slog.Warn("discovercrawl: skipping malformed share", "uri", r.URI)
			continue
		}
		if sr.Document == "" {
			rssShares = append(rssShares, sr)
			continue
		}
		// Newest sidecar wins (same rule as the own-repo reconcile) so a sync/PATCH race doesn't shadow the latest comment.
		if cur, ok := sidecarByDoc[sr.Document]; !ok || sr.Rkey > cur.Rkey {
			sidecarByDoc[sr.Document] = sr
		}
	}

	// Canonical recommend per document = smallest rkey (TID ⇒ earliest created), same rule as reconcileShares.
	canonicalByDoc := map[string]RecommendRecord{}
	for _, r := range byCollection[standardRecommendCollection] {
		rec, ok := decodeRecommendRecord(r)
		if !ok {
			slog.Warn("discovercrawl: skipping malformed recommend", "uri", r.URI)
			continue
		}
		if cur, ok := canonicalByDoc[rec.Document]; !ok || rec.Rkey < cur.Rkey {
			canonicalByDoc[rec.Document] = rec
		}
	}

	skyreaderRecords := byCollection[skyreaderShareCollection]
	out := make([]Share, 0, len(canonicalByDoc)+len(rssShares)+len(skyreaderRecords))
	for doc, rec := range canonicalByDoc {
		s := Share{Kind: "standardfeed", Document: doc, CreatedAt: rec.CreatedAt}
		if sc, ok := sidecarByDoc[doc]; ok {
			s.ItemURL = sc.ItemURL
			s.Comment = sc.Comment
			s.FeedURL = sc.FeedURL
		}
		out = append(out, s)
	}

	seen := map[string]struct{}{} // itemUrl dedupe within rss/skyreader shares
	for _, sr := range rssShares {
		if _, dup := seen[sr.ItemURL]; dup {
			continue
		}
		seen[sr.ItemURL] = struct{}{}
		out = append(out, Share{Kind: "rss", ItemURL: sr.ItemURL, FeedURL: sr.FeedURL, Comment: sr.Comment, CreatedAt: sr.CreatedAt})
	}
	for _, r := range skyreaderRecords {
		sr, ok := decodeShareRecord(r)
		if !ok {
			slog.Warn("discovercrawl: skipping malformed skyreader share", "uri", r.URI)
			continue
		}
		if _, dup := seen[sr.ItemURL]; dup {
			continue
		}
		seen[sr.ItemURL] = struct{}{}
		out = append(out, Share{Kind: "skyreader", ItemURL: sr.ItemURL, FeedURL: sr.FeedURL, Comment: sr.Comment, CreatedAt: sr.CreatedAt})
	}
	return out
}

// DecodeFollows collects one repo's reader-network follows from mirrored rows, deduped and self-excluding, matching CrawlReaderNetworkFollows.
func DecodeFollows(did string, byCollection map[string][]RecordEntry) []ReaderNetworkFollow {
	seen := make(map[string]struct{})
	for _, coll := range readerNetworkFollowCollections {
		addReaderNetworkFollows(seen, byCollection[coll], did)
	}
	out := make([]ReaderNetworkFollow, 0, len(seen))
	for d := range seen {
		out = append(out, ReaderNetworkFollow{DID: d})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DID < out[j].DID })
	return out
}

// DecodeSubscriptions folds one repo's subscription rows, keyed by collection, into the deduped set Crawl returns. Publication resolution reaches the network, so this must not run inside a transaction.
func (c *Client) DecodeSubscriptions(ctx context.Context, byCollection map[string][]RecordEntry) []Subscription {
	pubCache := make(map[string]Subscription)
	seen := make(map[string]Subscription)
	for _, coll := range subscriptionCollections {
		for _, r := range byCollection[coll] {
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
	return out
}

// DecodeAuthoredPublication decodes and verifies one publication record. handle and lastPublishedAt are repo-scoped: the crawl reads them off the PDS, the tap rebuild off its mirror.
func (c *Client) DecodeAuthoredPublication(ctx context.Context, r RecordEntry, did syntax.DID, handle syntax.Handle, lastPublishedAt string) (AuthoredPublication, bool) {
	pub, outcome, _ := c.decodeAuthoredPublication(ctx, r, did, handle, lastPublishedAt)
	return pub, outcome == outcomeVerified
}

func (c *Client) decodeAuthoredPublication(ctx context.Context, r RecordEntry, did syntax.DID, handle syntax.Handle, lastPublishedAt string) (AuthoredPublication, authorshipOutcome, error) {
	name, _ := r.Value["name"].(string)
	url, _ := r.Value["url"].(string)
	if name == "" || url == "" {
		slog.Warn("discovercrawl: skipping publication without name/url", "uri", r.URI)
		return AuthoredPublication{}, outcomeMismatch, nil
	}
	rkey := atprepo.RkeyFromATURI(r.URI)
	outcome, err := verifyAuthorship(ctx, c.verifier, url, rkey, did, handle)
	switch outcome {
	case outcomeMismatch:
		slog.Warn("discovercrawl: authorship claim does not match site's well-known", "uri", r.URI, "site", url)
		return AuthoredPublication{}, outcomeMismatch, nil
	case outcomeProbeError:
		slog.Warn("discovercrawl: well-known probe failed", "uri", r.URI, "site", url, "err", err)
		return AuthoredPublication{}, outcomeProbeError, err
	}
	return AuthoredPublication{
		Key:             "at://" + did.String() + "/" + authoredPublicationCollection + "/" + rkey,
		Kind:            "standardfeed",
		Title:           name,
		SiteURL:         url,
		LastPublishedAt: lastPublishedAt,
		Verification:    verifiedOutcome,
	}, outcomeVerified, nil
}

// DecodeAuthoredPublications decodes one repo's mirrored publication rows. A probe failure aborts the batch so tap retains the last verified aggregate for retry.
func (c *Client) DecodeAuthoredPublications(ctx context.Context, byCollection map[string][]RecordEntry, did syntax.DID, handle syntax.Handle) ([]AuthoredPublication, error) {
	records := byCollection[authoredPublicationCollection]
	if len(records) == 0 {
		return nil, nil
	}
	lastPublishedAt := maxDocumentPublishedAt(byCollection[standardDocumentCollectionForLatest])
	out := make([]AuthoredPublication, 0, len(records))
	for _, r := range records {
		pub, outcome, err := c.decodeAuthoredPublication(ctx, r, did, handle, lastPublishedAt)
		switch outcome {
		case outcomeVerified:
			out = append(out, pub)
		case outcomeProbeError:
			return nil, fmt.Errorf("discovercrawl: authorship probe failed for %s: %w", r.URI, err)
		}
	}
	return out, nil
}

// maxDocumentPublishedAt stands in for the crawl's newest-first listRecords probe; RFC3339 stamps sort lexically, so the largest string is the newest document.
func maxDocumentPublishedAt(rows []RecordEntry) string {
	var newest string
	for _, r := range rows {
		if s, _ := r.Value["publishedAt"].(string); s > newest {
			newest = s
		}
	}
	return newest
}
