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
		{in: "at://did:plc:example/blue.morgen.feed.subscription/3la", want: "3la"},
		{in: "not-an-at-uri", want: ""},
		{in: "at://did:plc:example/blue.morgen.feed.subscription/", want: ""},
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
		_ = json.NewEncoder(w).Encode(RecordRef{URI: "at://did:plc:example/blue.morgen.feed.subscription/3la", CID: "bafy"})
	})
	defer srv.Close()

	out, err := (SessionWriter{}).CreateRecord(context.Background(), newTestSession(t, srv), syntax.NSID("blue.morgen.feed.subscription"), map[string]any{
		"feedUrl": "https://example.com/feed.xml",
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if out.URI == "" || out.CID != "bafy" {
		t.Fatalf("ref = %+v", out)
	}
	assertRepoWriteBody(t, got, "did:plc:example", "blue.morgen.feed.subscription", "", "https://example.com/feed.xml")
}

func TestPutRecord_PostsCorrectBody(t *testing.T) {
	var got map[string]any
	srv := repoServer(t, "com.atproto.repo.putRecord", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(RecordRef{URI: "at://did:plc:example/blue.morgen.feed.subscription/3la", CID: "bafy"})
	})
	defer srv.Close()

	_, err := (SessionWriter{}).PutRecord(context.Background(), newTestSession(t, srv), syntax.NSID("blue.morgen.feed.subscription"), "3la", map[string]any{
		"feedUrl": "https://example.com/feed.xml",
	})
	if err != nil {
		t.Fatalf("PutRecord: %v", err)
	}
	assertRepoWriteBody(t, got, "did:plc:example", "blue.morgen.feed.subscription", "3la", "https://example.com/feed.xml")
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

	if err := (SessionWriter{}).DeleteRecord(context.Background(), newTestSession(t, srv), syntax.NSID("blue.morgen.feed.subscription"), "3la"); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}
	if got["repo"] != "did:plc:example" {
		t.Errorf("repo = %v", got["repo"])
	}
	if got["collection"] != "blue.morgen.feed.subscription" {
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

	_, err := (SessionWriter{}).CreateRecord(context.Background(), newTestSession(t, srv), syntax.NSID("blue.morgen.feed.subscription"), map[string]any{
		"feedUrl": "https://example.com/feed.xml",
	})
	if err == nil {
		t.Fatal("expected upstream error")
	}
	if !strings.Contains(err.Error(), "400") && !strings.Contains(err.Error(), "bad record") {
		t.Fatalf("error = %v, want status detail", err)
	}
}

// TestListRecords_PagesUntilCursorEmpty checks the pagination contract: an empty page with a non-empty cursor must continue, since stopping early would make the DELETE sweep miss duplicate records.
func TestListRecords_PagesUntilCursorEmpty(t *testing.T) {
	page := 0
	pages := []struct {
		records []map[string]any
		cursor  string
	}{
		{records: []map[string]any{
			{"uri": "at://did:plc:example/site.standard.graph.subscription/3a", "cid": "b", "value": map[string]any{"publication": "at://pub/one"}},
		}, cursor: "c1"},
		{records: nil, cursor: "c2"},
		{records: []map[string]any{
			{"uri": "at://did:plc:example/site.standard.graph.subscription/3b", "cid": "b", "value": map[string]any{"publication": "at://pub/two"}},
		}, cursor: ""},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/xrpc/com.atproto.repo.listRecords" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("repo"); got != "did:plc:example" {
			t.Errorf("repo = %q", got)
		}
		if got := r.URL.Query().Get("collection"); got != "site.standard.graph.subscription" {
			t.Errorf("collection = %q", got)
		}
		if page >= len(pages) {
			t.Errorf("unexpected extra page request: %d", page)
			http.Error(w, "extra", http.StatusInternalServerError)
			return
		}
		p := pages[page]
		page++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"records": p.records, "cursor": p.cursor})
	}))
	defer srv.Close()

	out, err := (SessionWriter{}).ListRecords(context.Background(), newTestSession(t, srv), syntax.NSID("site.standard.graph.subscription"))
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if page != len(pages) {
		t.Errorf("pages fetched = %d, want %d", page, len(pages))
	}
	if len(out) != 2 {
		t.Fatalf("records = %d, want 2", len(out))
	}
	if out[0].URI != "at://did:plc:example/site.standard.graph.subscription/3a" {
		t.Errorf("first record = %+v", out[0])
	}
	if pub, _ := out[1].Value["publication"].(string); pub != "at://pub/two" {
		t.Errorf("second record value = %+v", out[1].Value)
	}
}

func TestListRecords_PropagatesUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"InvalidRequest","message":"bad collection"}`))
	}))
	defer srv.Close()

	_, err := (SessionWriter{}).ListRecords(context.Background(), newTestSession(t, srv), syntax.NSID("site.standard.graph.subscription"))
	if err == nil {
		t.Fatal("expected upstream error")
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
