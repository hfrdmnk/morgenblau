package sync

import (
	"context"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
)

// canonicalByKey keeps the smallest rkey per key: TID ordering makes the smallest rkey the earliest-created, existence-authority record.
func canonicalByKey[K comparable, R any](records []R, keyOf func(R) K, rkeyOf func(R) string) map[K]R {
	byKey := make(map[K]R, len(records))
	for _, r := range records {
		k := keyOf(r)
		if cur, ok := byKey[k]; !ok || rkeyOf(r) < rkeyOf(cur) {
			byKey[k] = r
		}
	}
	return byKey
}

// newestSidecarByKey keeps the largest rkey per key, so the newest edit survives a sync/PATCH-race duplicate.
func newestSidecarByKey[K comparable, S any](sidecars []S, keyOf func(S) K, rkeyOf func(S) string, onDuplicate func(kept, dropped S)) map[K]S {
	byKey := make(map[K]S, len(sidecars))
	for _, s := range sidecars {
		k := keyOf(s)
		if cur, ok := byKey[k]; !ok || rkeyOf(s) > rkeyOf(cur) {
			if ok {
				onDuplicate(s, cur)
			}
			byKey[k] = s
		}
	}
	return byKey
}

// sidecarCleanup deletes orphaned or superseded sidecars from the PDS, the one place reconcile writes back. A nil pds disables it.
func sidecarCleanup[K comparable, S any, R any](
	ctx context.Context,
	pds atprepo.Writer,
	sess *oauth.ClientSession,
	collection syntax.NSID,
	sidecars []S,
	keyOf func(S) K,
	rkeyOf func(S) string,
	canonicalByKey map[K]R,
	newestSidecarByKey map[K]S,
	onErr func(rkey string, err error),
) {
	if pds == nil {
		return
	}
	for _, sc := range sidecars {
		k := keyOf(sc)
		if _, alive := canonicalByKey[k]; alive && rkeyOf(newestSidecarByKey[k]) == rkeyOf(sc) {
			continue
		}
		if err := pds.DeleteRecord(ctx, sess, collection, rkeyOf(sc)); err != nil {
			onErr(rkeyOf(sc), err)
		}
	}
}
