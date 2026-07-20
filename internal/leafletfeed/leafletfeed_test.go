package leafletfeed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type fakeResolver struct {
	byID map[string]*identity.Identity
}

func (f *fakeResolver) Lookup(_ context.Context, atid syntax.AtIdentifier) (*identity.Identity, error) {
	if ident, ok := f.byID[atid.String()]; ok {
		return ident, nil
	}
	return nil, fmt.Errorf("unknown identity %s", atid)
}

func identityFor(t *testing.T, didStr, pds string) (syntax.DID, *identity.Identity) {
	t.Helper()
	did, err := syntax.ParseDID(didStr)
	if err != nil {
		t.Fatalf("ParseDID: %v", err)
	}
	return did, &identity.Identity{
		DID: did,
		Services: map[string]identity.ServiceEndpoint{
			"atproto_pds": {Type: "AtprotoPersonalDataServer", URL: pds},
		},
	}
}

const pubDID = "did:plc:pub123"

// newClientAgainst wraps handler with a request counter so tests can assert zero network calls.
func newClientAgainst(t *testing.T, handler http.Handler, extraIDs ...string) (*Client, *int) {
	t.Helper()
	hits := 0
	counting := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		handler.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(counting)
	t.Cleanup(srv.Close)
	_, ident := identityFor(t, pubDID, srv.URL)
	byID := map[string]*identity.Identity{pubDID: ident}
	for _, id := range extraIDs {
		byID[id] = ident
	}
	return NewClient(&fakeResolver{byID: byID}, srv.Client()), &hits
}

func TestGetPublication_MapsFields(t *testing.T) {
	didURI := "at://" + pubDID + "/" + CollectionPublication + "/3abc"
	var gotQuery map[string]string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/xrpc/com.atproto.repo.getRecord" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		gotQuery = map[string]string{
			"repo":       r.URL.Query().Get("repo"),
			"collection": r.URL.Query().Get("collection"),
			"rkey":       r.URL.Query().Get("rkey"),
		}
		json.NewEncoder(w).Encode(map[string]any{
			"uri": didURI,
			"value": map[string]any{
				"name":        "Example Publication",
				"base_path":   "https://pages.example.com",
				"description": "notes on the protocol",
				"preferences": map[string]any{"showComments": true, "showInDiscover": true},
			},
		})
	})
	client, _ := newClientAgainst(t, handler, "publisher.example")

	pub, err := client.GetPublication(context.Background(), "at://publisher.example/"+CollectionPublication+"/3abc")
	if err != nil {
		t.Fatalf("GetPublication: %v", err)
	}
	if gotQuery["repo"] != pubDID || gotQuery["collection"] != CollectionPublication || gotQuery["rkey"] != "3abc" {
		t.Fatalf("getRecord params: %+v", gotQuery)
	}
	if pub.URI != didURI {
		t.Fatalf("URI not DID-normalized: %q", pub.URI)
	}
	if pub.DID != pubDID || pub.Name != "Example Publication" || pub.Description != "notes on the protocol" {
		t.Fatalf("identity/metadata fields: %+v", pub)
	}
	if pub.BasePath != "pages.example.com" {
		t.Fatalf("BasePath = %q", pub.BasePath)
	}
	if pub.URL != "https://pages.example.com" || pub.FeedURL != "https://pages.example.com/rss" {
		t.Fatalf("URL/FeedURL: url=%q feed=%q", pub.URL, pub.FeedURL)
	}
	if !pub.ShowInDiscover {
		t.Fatal("ShowInDiscover should be true from explicit preferences")
	}
}

func TestGetPublication_WrongCollectionMakesNoRequest(t *testing.T) {
	uri := "at://" + pubDID + "/site.standard.publication/3abc"
	client, hits := newClientAgainst(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	if _, err := client.GetPublication(context.Background(), uri); err == nil {
		t.Fatal("expected error, got nil")
	}
	if *hits != 0 {
		t.Fatalf("expected zero HTTP requests, got %d", *hits)
	}
}

func TestGetPublication_MissingNameErrors(t *testing.T) {
	uri := "at://" + pubDID + "/" + CollectionPublication + "/3abc"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"uri":   uri,
			"value": map[string]any{"base_path": "example.com"},
		})
	})
	client, _ := newClientAgainst(t, handler)

	if _, err := client.GetPublication(context.Background(), uri); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetPublication_MissingBasePath(t *testing.T) {
	uri := "at://" + pubDID + "/" + CollectionPublication + "/3abc"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"uri":   uri,
			"value": map[string]any{"name": "Example Author"},
		})
	})
	client, _ := newClientAgainst(t, handler)

	pub, err := client.GetPublication(context.Background(), uri)
	if err != nil {
		t.Fatalf("GetPublication: %v", err)
	}
	if pub.Name != "Example Author" {
		t.Fatalf("Name = %q", pub.Name)
	}
	if pub.URL != "" || pub.FeedURL != "" || pub.BasePath != "" {
		t.Fatalf("expected empty URL fields, got url=%q feed=%q base=%q", pub.URL, pub.FeedURL, pub.BasePath)
	}
}

func TestGetPublication_ShowInDiscoverDefaultsTrue(t *testing.T) {
	cases := []struct {
		name  string
		value map[string]any
		want  bool
	}{
		{
			name:  "preferences absent",
			value: map[string]any{"name": "Example Publication"},
			want:  true,
		},
		{
			name:  "preferences null",
			value: map[string]any{"name": "Example Publication", "preferences": nil},
			want:  true,
		},
		{
			name:  "explicit false",
			value: map[string]any{"name": "Example Publication", "preferences": map[string]any{"showInDiscover": false}},
			want:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uri := "at://" + pubDID + "/" + CollectionPublication + "/3abc"
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{"uri": uri, "value": tc.value})
			})
			client, _ := newClientAgainst(t, handler)

			pub, err := client.GetPublication(context.Background(), uri)
			if err != nil {
				t.Fatalf("GetPublication: %v", err)
			}
			if pub.ShowInDiscover != tc.want {
				t.Fatalf("ShowInDiscover = %v, want %v", pub.ShowInDiscover, tc.want)
			}
		})
	}
}

func TestNormalizeBasePath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"pages.example.com", "pages.example.com"},
		{"https://pages.example.com", "pages.example.com"},
		{"http://pages.example.com", "pages.example.com"},
		{"PAGES.Example.Com/", "pages.example.com"},
		{"example.com/blog", "example.com/blog"},
		{"Example.com/Blog", "example.com/Blog"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := normalizeBasePath(tc.in); got != tc.want {
				t.Fatalf("normalizeBasePath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
