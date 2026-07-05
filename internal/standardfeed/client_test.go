package standardfeed

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

// identityFor builds an identity whose PDS endpoint points at the test server.
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

func newClientAgainst(t *testing.T, handler http.Handler, extraIDs ...string) (*Client, string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	_, ident := identityFor(t, pubDID, srv.URL)
	byID := map[string]*identity.Identity{pubDID: ident}
	for _, id := range extraIDs {
		byID[id] = ident
	}
	return NewClient(&fakeResolver{byID: byID}, srv.Client()), srv.URL
}

func TestGetPublication_MapsFields(t *testing.T) {
	pubURI := "at://" + pubDID + "/site.standard.publication/3abc"
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
			"uri": pubURI,
			"cid": "bafycid1",
			"value": map[string]any{
				"name":        "Example Journal",
				"url":         "https://example.com/",
				"description": "a calm publication",
				"icon": map[string]any{
					"ref": map[string]any{"$link": "bafyicon"},
				},
			},
		})
	})
	client, srvURL := newClientAgainst(t, handler)

	pub, err := client.GetPublication(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("GetPublication: %v", err)
	}
	if gotQuery["repo"] != pubDID || gotQuery["collection"] != CollectionPublication || gotQuery["rkey"] != "3abc" {
		t.Fatalf("getRecord params: %+v", gotQuery)
	}
	if pub.URI != pubURI || pub.CID != "bafycid1" || pub.DID != pubDID {
		t.Fatalf("identity fields: %+v", pub)
	}
	if pub.Name != "Example Journal" || pub.Description != "a calm publication" {
		t.Fatalf("metadata fields: %+v", pub)
	}
	if pub.URL != "https://example.com" {
		t.Fatalf("URL trailing slash not stripped: %q", pub.URL)
	}
	wantIcon := srvURL + "/xrpc/com.atproto.sync.getBlob?cid=bafyicon&did=did%3Aplc%3Apub123"
	if pub.IconURL != wantIcon {
		t.Fatalf("IconURL = %q, want %q", pub.IconURL, wantIcon)
	}
}

func TestGetPublication_HandleAuthorityNormalizesURI(t *testing.T) {
	didURI := "at://" + pubDID + "/site.standard.publication/3abc"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"uri": didURI,
			"cid": "bafycid1",
			"value": map[string]any{
				"name": "Example Journal",
				"url":  "https://example.com",
			},
		})
	})
	client, _ := newClientAgainst(t, handler, "journal.example.com")

	pub, err := client.GetPublication(context.Background(), "at://journal.example.com/site.standard.publication/3abc")
	if err != nil {
		t.Fatalf("GetPublication: %v", err)
	}
	if pub.URI != didURI {
		t.Fatalf("URI not DID-normalized: %q", pub.URI)
	}
}

func TestGetPublication_Errors(t *testing.T) {
	cases := []struct {
		name    string
		uri     string
		handler http.HandlerFunc
	}{
		{
			name: "record not found",
			uri:  "at://" + pubDID + "/site.standard.publication/3abc",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]any{"error": "RecordNotFound", "message": "nope"})
			},
		},
		{
			name: "missing required fields",
			uri:  "at://" + pubDID + "/site.standard.publication/3abc",
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{
					"uri": "at://" + pubDID + "/site.standard.publication/3abc", "cid": "c",
					"value": map[string]any{"url": "https://example.com"},
				})
			},
		},
		{
			name:    "wrong collection",
			uri:     "at://" + pubDID + "/site.standard.document/3abc",
			handler: func(w http.ResponseWriter, r *http.Request) {},
		},
		{
			name:    "not an at-uri",
			uri:     "https://example.com",
			handler: func(w http.ResponseWriter, r *http.Request) {},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := newClientAgainst(t, tc.handler)
			if _, err := client.GetPublication(context.Background(), tc.uri); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestGetDocument_MapsFields(t *testing.T) {
	docURI := "at://" + pubDID + "/site.standard.document/3doc"
	pubURI := "at://" + pubDID + "/site.standard.publication/3abc"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"uri": docURI,
			"cid": "bafydoc",
			"value": map[string]any{
				"site":        pubURI,
				"title":       "Hello World",
				"path":        "/hello-world",
				"description": "an excerpt",
				"textContent": "plain text body",
				"publishedAt": "2026-07-01T08:00:00Z",
				"updatedAt":   "2026-07-02T08:00:00Z",
				"tags":        []any{"a", 42, "b"},
				"coverImage": map[string]any{
					"ref": map[string]any{"$link": "bafycover"},
				},
			},
		})
	})
	client, srvURL := newClientAgainst(t, handler)

	doc, err := client.GetDocument(context.Background(), docURI)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if doc.URI != docURI || doc.CID != "bafydoc" || doc.Site != pubURI {
		t.Fatalf("identity fields: %+v", doc)
	}
	if doc.Title != "Hello World" || doc.Path != "/hello-world" || doc.Description != "an excerpt" ||
		doc.TextContent != "plain text body" || doc.PublishedAt != "2026-07-01T08:00:00Z" || doc.UpdatedAt != "2026-07-02T08:00:00Z" {
		t.Fatalf("content fields: %+v", doc)
	}
	if len(doc.Tags) != 2 || doc.Tags[0] != "a" || doc.Tags[1] != "b" {
		t.Fatalf("tags: %+v", doc.Tags)
	}
	wantCover := srvURL + "/xrpc/com.atproto.sync.getBlob?cid=bafycover&did=did%3Aplc%3Apub123"
	if doc.CoverImageURL != wantCover {
		t.Fatalf("CoverImageURL = %q, want %q", doc.CoverImageURL, wantCover)
	}
}

func TestGetDocument_MissingOptionals(t *testing.T) {
	docURI := "at://" + pubDID + "/site.standard.document/3doc"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"uri": docURI,
			"cid": "bafydoc",
			"value": map[string]any{
				"site":        "https://loose.example.com",
				"title":       "Loose",
				"publishedAt": "2026-07-01T08:00:00Z",
			},
		})
	})
	client, _ := newClientAgainst(t, handler)

	doc, err := client.GetDocument(context.Background(), docURI)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if doc.Path != "" || doc.Description != "" || doc.TextContent != "" || doc.Tags != nil || doc.CoverImageURL != "" {
		t.Fatalf("optionals should be zero: %+v", doc)
	}
	if doc.Site != "https://loose.example.com" {
		t.Fatalf("Site: %q", doc.Site)
	}
}

func TestListDocuments_PagesAndFilters(t *testing.T) {
	pubURI := "at://" + pubDID + "/site.standard.publication/3abc"
	otherPub := "at://" + pubDID + "/site.standard.publication/3zzz"
	docRecord := func(rkey, site string) map[string]any {
		return map[string]any{
			"uri": "at://" + pubDID + "/site.standard.document/" + rkey,
			"cid": "cid-" + rkey,
			"value": map[string]any{
				"site": site, "title": "Doc " + rkey, "publishedAt": "2026-07-01T08:00:00Z",
			},
		}
	}
	var cursors []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/xrpc/com.atproto.repo.listRecords" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("collection"); got != CollectionDocument {
			t.Fatalf("collection = %q", got)
		}
		cursor := r.URL.Query().Get("cursor")
		cursors = append(cursors, cursor)
		switch cursor {
		case "":
			json.NewEncoder(w).Encode(map[string]any{
				"records": []any{docRecord("3one", pubURI), docRecord("3two", otherPub)},
				"cursor":  "page2",
			})
		case "page2":
			json.NewEncoder(w).Encode(map[string]any{
				"records": []any{docRecord("3three", "https://loose.example.com"), docRecord("3four", pubURI)},
			})
		default:
			t.Fatalf("unexpected cursor %q", cursor)
		}
	})
	client, _ := newClientAgainst(t, handler)

	docs, err := client.ListDocuments(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(docs) != 2 || docs[0].URI != "at://"+pubDID+"/site.standard.document/3one" || docs[1].URI != "at://"+pubDID+"/site.standard.document/3four" {
		t.Fatalf("filtered docs: %+v", docs)
	}
	if len(cursors) != 2 || cursors[1] != "page2" {
		t.Fatalf("cursor sequence: %v", cursors)
	}
}

func TestListDocuments_HandleAuthorityMatchesDIDSite(t *testing.T) {
	// Caller passes a handle-authority publication uri, but documents' site
	// fields use the DID form. The filter must still match them.
	handlePub := "at://alice.example/site.standard.publication/3abc"
	didPub := "at://" + pubDID + "/site.standard.publication/3abc"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"records": []any{map[string]any{
				"uri": "at://" + pubDID + "/site.standard.document/3one",
				"cid": "cid-1",
				"value": map[string]any{
					"site": didPub, "title": "Doc", "publishedAt": "2026-07-01T08:00:00Z",
				},
			}},
		})
	})
	client, _ := newClientAgainst(t, handler, "alice.example")

	docs, err := client.ListDocuments(context.Background(), handlePub)
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(docs) != 1 || docs[0].URI != "at://"+pubDID+"/site.standard.document/3one" {
		t.Fatalf("handle-authority pub should match DID-form site: %+v", docs)
	}
}

func TestListDocuments_EmptyCollection(t *testing.T) {
	pubURI := "at://" + pubDID + "/site.standard.publication/3abc"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
	})
	client, _ := newClientAgainst(t, handler)

	docs, err := client.ListDocuments(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("expected no docs, got %+v", docs)
	}
}
