package profiles

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/safehttp"
)

func TestFetchProfile_SendsMorgenblauUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"uri":"at://did:plc:alice/app.bsky.actor.profile/self","cid":"c","value":{}}`))
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
		w.Write([]byte(`{"uri":"at://did:plc:alice/app.bsky.actor.profile/self","cid":"c","value":{"description":"Reads calmly."}}`))
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
		w.Write([]byte(`{"uri":"at://did:plc:alice/app.bsky.actor.profile/self","cid":"c","value":{}}`))
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
