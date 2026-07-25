package api

// DiscoverMemo is the per-user payload memo a discover handler reads through. A nil memo turns memoization off, which is what handler tests that don't care about caching pass.
type DiscoverMemo[T any] interface {
	Get(did string) (T, bool)
	Put(did string, value T)
}

// DiscoverInvalidator stales a user's memoized discover payloads. Every local write that changes what they should be suggested calls it, so the next request reassembles instead of serving a payload that predates the write.
type DiscoverInvalidator interface {
	Invalidate(did string)
}

// memoizedPayload serves did's memoized payload, building and storing it on a miss. Ranking stays downstream of this on purpose: it is a pure function of the payload plus the request cursor, so a memo hit still honours the cursor's seed and page position.
func memoizedPayload[T any](memo DiscoverMemo[T], did string, build func() (T, error)) (T, error) {
	if memo != nil {
		if payload, ok := memo.Get(did); ok {
			return payload, nil
		}
	}
	payload, err := build()
	if err != nil {
		return payload, err
	}
	if memo != nil {
		memo.Put(did, payload)
	}
	return payload, nil
}

func invalidateDiscover(memo DiscoverInvalidator, did string) {
	if memo == nil {
		return
	}
	memo.Invalidate(did)
}
