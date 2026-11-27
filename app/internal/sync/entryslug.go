package sync

import "crypto/sha256"

func EntrySlug(feedURL, guid string) string {
	digest := sha256.Sum256([]byte(feedURL + "|" + guid))
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	out := make([]byte, 10)
	for i := range out {
		out[i] = alphabet[digest[i]%62]
	}
	return string(out)
}
