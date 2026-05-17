package atprepo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

func newTestSession(t *testing.T, srv *httptest.Server) *oauth.ClientSession {
	t.Helper()
	did, err := syntax.ParseDID("did:plc:example")
	if err != nil {
		t.Fatal(err)
	}
	priv, err := atcrypto.GeneratePrivateKeyP256()
	if err != nil {
		t.Fatal(err)
	}
	return &oauth.ClientSession{
		Client: srv.Client(),
		Config: &oauth.ClientConfig{},
		Data: &oauth.ClientSessionData{
			AccountDID:                   did,
			SessionID:                    "sid-1",
			HostURL:                      srv.URL,
			AuthServerTokenEndpoint:      srv.URL + "/oauth/token",
			AccessToken:                  "access-token",
			RefreshToken:                 "refresh-token",
			DPoPPrivateKeyMultibase:      priv.Multibase(),
			DPoPAuthServerNonce:          "auth-nonce",
			DPoPHostNonce:                "host-nonce",
			AuthServerURL:                srv.URL,
			AuthServerRevocationEndpoint: srv.URL + "/oauth/revoke",
		},
		DPoPPrivateKey: priv,
	}
}

func TestRkeyFromATURI(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "at://did:plc:example/app.skyreader.feed.subscription/3la", want: "3la"},
		{in: "not-an-at-uri", want: ""},
		{in: "at://did:plc:example/app.skyreader.feed.subscription/", want: ""},
	}

	for _, tt := range cases {
		t.Run(tt.in, func(t *testing.T) {
			if got := RkeyFromATURI(tt.in); got != tt.want {
				t.Errorf("RkeyFromATURI(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCreateRecord_PostsCorrectBody(t *testing.T) {
	var got map[string]any
	srv := repoServer(t, "com.atproto.repo.createRecord", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(RecordRef{URI: "at://did:plc:example/app.skyreader.feed.subscription/3la", CID: "bafy"})
	})
	defer srv.Close()

	out, err := (SessionWriter{}).CreateRecord(context.Background(), newTestSession(t, srv), syntax.NSID("app.skyreader.feed.subscription"), map[string]any{
		"feedUrl": "https://example.com/feed.xml",
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if out.URI == "" || out.CID != "bafy" {
		t.Fatalf("ref = %+v", out)
	}
	assertRepoWriteBody(t, got, "did:plc:example", "app.skyreader.feed.subscription", "", "https://example.com/feed.xml")
}

func TestPutRecord_PostsCorrectBody(t *testing.T) {
	var got map[string]any
	srv := repoServer(t, "com.atproto.repo.putRecord", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(RecordRef{URI: "at://did:plc:example/app.skyreader.feed.subscription/3la", CID: "bafy"})
	})
	defer srv.Close()

	_, err := (SessionWriter{}).PutRecord(context.Background(), newTestSession(t, srv), syntax.NSID("app.skyreader.feed.subscription"), "3la", map[string]any{
		"feedUrl": "https://example.com/feed.xml",
	})
	if err != nil {
		t.Fatalf("PutRecord: %v", err)
	}
	assertRepoWriteBody(t, got, "did:plc:example", "app.skyreader.feed.subscription", "3la", "https://example.com/feed.xml")
}

func TestDeleteRecord_PostsCorrectBody(t *testing.T) {
	var got map[string]any
	srv := repoServer(t, "com.atproto.repo.deleteRecord", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	defer srv.Close()

	if err := (SessionWriter{}).DeleteRecord(context.Background(), newTestSession(t, srv), syntax.NSID("app.skyreader.feed.subscription"), "3la"); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}
	if got["repo"] != "did:plc:example" {
		t.Errorf("repo = %v", got["repo"])
	}
	if got["collection"] != "app.skyreader.feed.subscription" {
		t.Errorf("collection = %v", got["collection"])
	}
	if got["rkey"] != "3la" {
		t.Errorf("rkey = %v", got["rkey"])
	}
}

func TestCreateRecord_PropagatesUpstreamError(t *testing.T) {
	srv := repoServer(t, "com.atproto.repo.createRecord", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"InvalidRequest","message":"bad record"}`))
	})
	defer srv.Close()

	_, err := (SessionWriter{}).CreateRecord(context.Background(), newTestSession(t, srv), syntax.NSID("app.skyreader.feed.subscription"), map[string]any{
		"feedUrl": "https://example.com/feed.xml",
	})
	if err == nil {
		t.Fatal("expected upstream error")
	}
	if !strings.Contains(err.Error(), "400") && !strings.Contains(err.Error(), "bad record") {
		t.Fatalf("error = %v, want status detail", err)
	}
}

func repoServer(t *testing.T, endpoint string, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/xrpc/"+endpoint {
			t.Errorf("path = %s", r.URL.Path)
		}
		h(w, r)
	}))
}

func assertRepoWriteBody(t *testing.T, got map[string]any, repo, collection, rkey, feedURL string) {
	t.Helper()
	if got["repo"] != repo {
		t.Errorf("repo = %v", got["repo"])
	}
	if got["collection"] != collection {
		t.Errorf("collection = %v", got["collection"])
	}
	if rkey == "" {
		if _, ok := got["rkey"]; ok {
			t.Errorf("unexpected rkey = %v", got["rkey"])
		}
	} else if got["rkey"] != rkey {
		t.Errorf("rkey = %v", got["rkey"])
	}
	record, ok := got["record"].(map[string]any)
	if !ok {
		t.Fatalf("record = %#v", got["record"])
	}
	if record["feedUrl"] != feedURL {
		t.Errorf("record.feedUrl = %v", record["feedUrl"])
	}
}
