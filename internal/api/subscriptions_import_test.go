package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/tags"
)

func importRequest(t *testing.T, h http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions/import", bytes.NewReader(data)), "did:plc:alice", "sid-1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestImportPrepareForeignAndOPMLNeverWrites(t *testing.T) {
	for _, provider := range []string{"skyreader", "glean"} {
		t.Run(provider, func(t *testing.T) {
			collection := "at.glean.subscription"
			if provider == "skyreader" {
				collection = "app.skyreader.feed.subscription"
			}
			pds := &fakePDS{listed: map[string][]atprepo.ListedRecord{collection: {
				{Value: map[string]any{"feedUrl": "https://example.com/feed", "title": "Example", "category": "Reading", "tags": []any{"Tech"}, "customTitle": "My title"}},
				{Value: map[string]any{"feedUrl": "https://example.com/feed", "category": "Favorites"}},
				{Value: map[string]any{"sourceType": "atproto.documents", "subjectDid": "did:plc:publisher"}},
				{Value: map[string]any{"feedUrl": "file:///private"}},
			}}}
			w := importRequest(t, SubscriptionsImportPrepareHandler(pds), map[string]string{"provider": provider})
			if w.Code != 200 {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			var got importPlan
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got.Sources) != 1 || len(got.Warnings) != 2 {
				t.Fatalf("%#v", got)
			}
			wantTags := []string{"Reading", "Favorites"}
			wantTitle := "Example"
			if provider == "skyreader" {
				wantTags = []string{"Reading", "Tech", "Favorites"}
				wantTitle = "My title"
			}
			if !reflect.DeepEqual(got.Sources[0].Tags, wantTags) || got.Sources[0].Title != wantTitle {
				t.Fatalf("%#v", got.Sources[0])
			}
			if pds.creates != 0 || pds.puts != 0 {
				t.Fatal("prepare wrote records")
			}
		})
	}
	pds := &fakePDS{}
	w := importRequest(t, SubscriptionsImportPrepareHandler(pds), map[string]string{"provider": "opml", "opml": `<opml><body><outline xmlUrl="https://example.com/feed"/></body></opml>`})
	if w.Code != 200 || pds.listCalls != 0 {
		t.Fatalf("%d %s, calls=%d", w.Code, w.Body.String(), pds.listCalls)
	}
}

func TestImportMergesPDSMetadataAndRetryIsIdempotent(t *testing.T) {
	pds := &fakePDS{listed: map[string][]atprepo.ListedRecord{subscriptionCollection: {{
		URI:   "at://did:plc:alice/" + subscriptionCollection + "/old",
		CID:   "old-cid",
		Value: map[string]any{"source": sourceUnion("rss", "https://example.com/feed", "https://example.com"), "title": "My title", "primary": true, "tags": []any{"Existing"}, "createdAt": "2026-01-01T00:00:00Z", "extra": "preserve"},
	}}}}
	idx := newFakeIndex()
	disp := &fakeDispatcher{}
	h := SubscriptionsImportHandler(idx, pds, disp)
	body := map[string]any{"sources": []importSource{
		{FeedURL: "https://example.com/feed", Title: "Imported title", Tags: []string{"existing", "New"}},
		{FeedURL: "https://other.example.com/feed", Title: "Other", Tags: []string{"Fresh"}},
	}}
	w := importRequest(t, h, body)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var result importResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Added != 1 || result.Updated != 1 || len(result.Failures) != 0 {
		t.Fatalf("%#v", result)
	}
	if pds.lastPut["title"] != "My title" || pds.lastPut["primary"] != true || pds.lastPut["extra"] != "preserve" {
		t.Fatalf("metadata lost: %#v", pds.lastPut)
	}
	row := idx.rows["did:plc:alice"]["https://example.com/feed"]
	if !reflect.DeepEqual(tags.Unmarshal(row.Tags), []string{"Existing", "New"}) || row.IsPrimary != 1 {
		t.Fatalf("mirror: %#v", row)
	}
	w = importRequest(t, h, body)
	if w.Code != 200 || pds.creates != 1 || pds.puts != 1 {
		t.Fatalf("retry status %d; writes %d %d", w.Code, pds.creates, pds.puts)
	}
}

func TestImportPDSFailureDoesNotMirrorAndMirrorFailureDoesNotFailImport(t *testing.T) {
	body := map[string]any{"sources": []importSource{{FeedURL: "https://example.com/feed"}}}
	idx := newFakeIndex()
	pds := &fakePDS{createErr: map[string]error{subscriptionCollection: errors.New("offline")}}
	w := importRequest(t, SubscriptionsImportHandler(idx, pds, &fakeDispatcher{}), body)
	var result importResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Failures) != 1 || len(idx.upsertedFeeds) != 0 {
		t.Fatalf("%#v, mirrored %v", result, idx.upsertedFeeds)
	}
	pds = &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsImportHandler(upsertErrIndex{idx}, pds, disp)
	w = importRequest(t, h, body)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	result = importResult{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Added != 1 || len(result.Failures) != 0 || disp.manualSync != 1 {
		t.Fatalf("%#v, repairs %d", result, disp.manualSync)
	}
	w = importRequest(t, h, body)
	if w.Code != 200 || pds.creates != 1 {
		t.Fatalf("retry duplicated record: %d %s", pds.creates, w.Body.String())
	}
}

func TestImportExportExcludesNonRSSAndPreservesTags(t *testing.T) {
	pds := &fakePDS{listed: map[string][]atprepo.ListedRecord{subscriptionCollection: {
		{Value: map[string]any{"source": sourceUnion("rss", "https://example.com/feed", "https://example.com"), "title": "Example", "tags": []any{"One", "Two"}}},
		{Value: map[string]any{"source": sourceUnion("standardfeed", testPublication, "")}},
	}}}
	w := importRequest(t, SubscriptionsExportHandler(pds), nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var body struct {
		OPML string `json:"opml"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	plan, err := parseOPML(body.OPML)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Sources) != 1 || !sameTagSet(plan.Sources[0].Tags, []string{"One", "Two"}) {
		t.Fatalf("%#v", plan)
	}
}

func TestImportNewRecordHasCollectionType(t *testing.T) {
	pds := &fakePDS{}
	w := importRequest(t, SubscriptionsImportHandler(newFakeIndex(), pds, &fakeDispatcher{}), map[string]any{"sources": []importSource{{FeedURL: "https://example.com/feed"}}})
	if w.Code != 200 || pds.lastRec["$type"] != subscriptionCollection {
		t.Fatalf("status %d, record type %v", w.Code, pds.lastRec["$type"])
	}
}

func TestImportRejectsInvalidExistingRecord(t *testing.T) {
	for _, invalid := range []string{"createdAt", "tags"} {
		t.Run(invalid, func(t *testing.T) {
			record := importRecord(importSource{FeedURL: "https://example.com/feed"}, "2026-01-01T00:00:00Z")
			if invalid == "createdAt" {
				delete(record, "createdAt")
			} else {
				record["tags"] = []any{42}
			}
			pds := &fakePDS{listed: map[string][]atprepo.ListedRecord{subscriptionCollection: {{URI: "at://did:plc:alice/" + subscriptionCollection + "/old", Value: record}}}}
			idx := newFakeIndex()
			w := importRequest(t, SubscriptionsImportHandler(idx, pds, &fakeDispatcher{}), map[string]any{"sources": []importSource{{FeedURL: "https://example.com/feed"}}})
			var result importResult
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Failures) != 1 || result.Unchanged != 0 || pds.creates != 0 || len(idx.upsertedFeeds) != 0 {
				t.Fatalf("invalid record accepted: result=%+v creates=%d feeds=%v", result, pds.creates, idx.upsertedFeeds)
			}
		})
	}
}

func TestImportChoosesValidDuplicateBeforeInvalidEarlierRecord(t *testing.T) {
	valid := importRecord(importSource{FeedURL: "https://example.com/feed", Title: "Valid title"}, "2026-01-01T00:00:00Z")
	invalid := map[string]any{"source": sourceUnion("rss", "https://example.com/feed", ""), "title": "Invalid title"}
	pds := &fakePDS{listed: map[string][]atprepo.ListedRecord{subscriptionCollection: {
		{URI: "at://did:plc:alice/" + subscriptionCollection + "/a", Value: invalid},
		{URI: "at://did:plc:alice/" + subscriptionCollection + "/b", Value: valid},
	}}}
	idx := newFakeIndex()
	w := importRequest(t, SubscriptionsImportHandler(idx, pds, &fakeDispatcher{}), map[string]any{"sources": []importSource{{FeedURL: "https://example.com/feed"}}})
	row := idx.rows["did:plc:alice"]["https://example.com/feed"]
	if w.Code != 200 || row.Title == nil || *row.Title != "Valid title" || row.Rkey != "b" {
		t.Fatalf("wrong duplicate: %s row=%+v", w.Body.String(), row)
	}
}

func TestImportRecoveredRecordDispatchesFetch(t *testing.T) {
	// The create committed, but its response and local mirror were lost.
	record := importRecord(importSource{FeedURL: "https://example.com/feed"}, "2026-01-01T00:00:00Z")
	pds := &fakePDS{listed: map[string][]atprepo.ListedRecord{subscriptionCollection: {{URI: "at://did:plc:alice/" + subscriptionCollection + "/old", Value: record}}}}
	disp := &fakeDispatcher{}
	w := importRequest(t, SubscriptionsImportHandler(newFakeIndex(), pds, disp), map[string]any{"sources": []importSource{{FeedURL: "https://example.com/feed"}}})
	if w.Code != 200 || len(disp.dispatched) != 1 || pds.creates != 0 {
		t.Fatalf("recovery did not fetch: %s, dispatched=%v creates=%d", w.Body.String(), disp.dispatched, pds.creates)
	}
}
