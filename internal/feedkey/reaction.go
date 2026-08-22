package feedkey

import "context"

// ReactionEntryResolver looks up a reaction's Tier-2 provenance; callers' own resolver interfaces satisfy it structurally.
type ReactionEntryResolver interface {
	GetFeedURLByGuid(ctx context.Context, guid string) (string, error)
	GetFeedURLByItemURL(ctx context.Context, url string) (string, error)
}

// ResolveReactionKey maps a share/save to its canonical source key: feedUrl provenance, then document, then itemUrl; Normalize runs on every branch since lookups return feed_url unnormalized. SPEC <discovery>.
func ResolveReactionKey(ctx context.Context, resolver ReactionEntryResolver, feedURL, document, itemURL string) (string, bool) {
	if feedURL != "" {
		return Normalize(feedURL), true
	}
	if document != "" {
		if fu, err := resolver.GetFeedURLByGuid(ctx, document); err == nil && fu != "" {
			return Normalize(fu), true
		}
	}
	if itemURL != "" {
		if fu, err := resolver.GetFeedURLByItemURL(ctx, itemURL); err == nil && fu != "" {
			return Normalize(fu), true
		}
	}
	return "", false
}
