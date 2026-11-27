package profiles

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/safehttp"
)

func TestFetchProfile_SendsMorgenblauUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"uri":"at://did:plc:alice/app.bsky.actor.profile/self","cid":"c","value":{}}`)); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	did, err := syntax.ParseDID("did:plc:alice")
	if err != nil {
		t.Fatalf("ParseDID: %v", err)
	}
	f := PDSFetcher{Client: srv.Client()}
	if _, err := f.FetchProfile(context.Background(), did, srv.URL); err != nil {
		t.Fatalf("FetchProfile: %v", err)
	}
	if gotUA != safehttp.UserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, safehttp.UserAgent)
	}
}

func TestFetchProfile_ParsesDescription(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"uri":"at://did:plc:alice/app.bsky.actor.profile/self","cid":"c","value":{"description":"Reads calmly."}}`)); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	did, err := syntax.ParseDID("did:plc:alice")
	if err != nil {
		t.Fatalf("ParseDID: %v", err)
	}
	f := PDSFetcher{Client: srv.Client()}
	record, err := f.FetchProfile(context.Background(), did, srv.URL)
	if err != nil {
		t.Fatalf("FetchProfile: %v", err)
	}
	if record.Description == nil || *record.Description != "Reads calmly." {
		t.Errorf("Description = %v, want \"Reads calmly.\"", record.Description)
	}
}

func TestFetchProfile_MissingDescription_StaysNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"uri":"at://did:plc:alice/app.bsky.actor.profile/self","cid":"c","value":{}}`)); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	did, err := syntax.ParseDID("did:plc:alice")
	if err != nil {
		t.Fatalf("ParseDID: %v", err)
	}
	f := PDSFetcher{Client: srv.Client()}
	record, err := f.FetchProfile(context.Background(), did, srv.URL)
	if err != nil {
		t.Fatalf("FetchProfile: %v", err)
	}
	if record.Description != nil {
		t.Errorf("Description = %v, want nil", *record.Description)
	}
}

func TestCache_AnAccountWithoutAProfileRecordIsCachedLikeAFoundOne(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(`{"error":"RecordNotFound","message":"Could not locate record"}`)); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	did, ident := identityFor(t, "did:plc:alice", "user.example.com", srv.URL)
	c := New(&fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}, PDSFetcher{Client: srv.Client()})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	p, err := c.Get(context.Background(), did)
	if err != nil || p.Handle != "user.example.com" || p.DisplayName != nil {
		t.Fatalf("Get = %+v, %v; want the handle and no display name", p, err)
	}
	now = now.Add(degradedTTL + time.Minute)
	if _, err := c.Get(context.Background(), did); err != nil {
		t.Fatal(err)
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("PDS requests = %d, want 1: an account with no profile record has nothing to heal, so it keeps the full TTL instead of the degraded retry", n)
	}
}

func TestFetchProfile_AServerErrorIsStillAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte(`{"error":"InternalServerError"}`)); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	did, err := syntax.ParseDID("did:plc:alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (PDSFetcher{Client: srv.Client()}).FetchProfile(context.Background(), did, srv.URL); err == nil {
		t.Fatal("FetchProfile on a 500 = nil error, want an error so the cache retries soon")
	}
}
