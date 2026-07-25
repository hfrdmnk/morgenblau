package discovermemo

import "testing"

type otherPayload struct {
	dids []string
}

func TestGroup_InvalidateStalesEveryPayloadType(t *testing.T) {
	sources := New[payload](DefaultTTL)
	people := New[otherPayload](DefaultTTL)
	sources.Put("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", payload{})
	people.Put("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", otherPayload{})
	sources.Put("did:plc:bbbbbbbbbbbbbbbbbbbbbbbb", payload{})

	NewGroup(sources, people).Invalidate("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa")

	if _, ok := sources.Get("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"); ok {
		t.Error("sources memo kept the invalidated did")
	}
	if _, ok := people.Get("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"); ok {
		t.Error("people memo kept the invalidated did")
	}
	if _, ok := sources.Get("did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"); !ok {
		t.Error("sources memo dropped an unrelated did")
	}
}

func TestGroup_InvalidateAllStalesEveryPayloadType(t *testing.T) {
	sources := New[payload](DefaultTTL)
	people := New[otherPayload](DefaultTTL)
	sources.Put("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", payload{})
	people.Put("did:plc:bbbbbbbbbbbbbbbbbbbbbbbb", otherPayload{})

	NewGroup(sources, people).InvalidateAll()

	if n := sources.size(); n != 0 {
		t.Errorf("sources entries = %d, want 0", n)
	}
	if n := people.size(); n != 0 {
		t.Errorf("people entries = %d, want 0", n)
	}
}
