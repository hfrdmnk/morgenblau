package sync

import "crypto/sha256"

// EntrySlug returns a deterministic 10-char base62 slug from (feed_url, guid),
// matching the Laravel EntrySlugger so backfills and re-runs are idempotent.
func EntrySlug(feedURL, guid string) string {
	digest := sha256.Sum256([]byte(feedURL + "|" + guid))
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	out := make([]byte, 10)
	for i := range out {
		out[i] = alphabet[digest[i]%62]
	}
	return string(out)
}
