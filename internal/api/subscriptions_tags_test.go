package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"morgenblau/internal/database/db"
)

// --- List handler ---

func TestSubscriptionsList_FromIndex(t *testing.T) {
	idx := newFakeIndex()
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"https://example.test/feed.xml": {
			Did:     "did:plc:alice",
			Rkey:    "3la",
			AtUri:   "at://did:plc:alice/blue.morgen.feed.subscription/3la",
			FeedUrl: "https://example.test/feed.xml",
		},
	}
	h := SubscriptionsListHandler(idx)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got []SubscriptionWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://example.test/feed.xml" {
		t.Errorf("got = %+v", got)
	}
}

func TestSubscriptionsList_Standardfeed_CatalogTitleFallback(t *testing.T) {
	idx := newFakeIndex()
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		testPublication: {
			Did:     "did:plc:alice",
			Rkey:    "3std",
			AtUri:   "at://did:plc:alice/" + standardSubCollection + "/3std",
			FeedUrl: testPublication,
			Kind:    "standardfeed",
			// No user title: the wire falls back to the cached catalog title.
		},
	}
	idx.catalogTitles[testPublication] = ptrString("Publication Name")

	h := SubscriptionsListHandler(idx)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got []SubscriptionWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %d", len(got))
	}
	if got[0].Kind != "standardfeed" || got[0].Publication != testPublication {
		t.Errorf("wire kind/publication = %q/%q", got[0].Kind, got[0].Publication)
	}
	if got[0].Title != "Publication Name" {
		t.Errorf("wire title = %q, want catalog fallback", got[0].Title)
	}
	// The value map mirrors the PDS record; no user title there.
	if _, ok := got[0].Value["title"]; ok {
		t.Errorf("value.title should stay absent without a user title: %v", got[0].Value)
	}
}

func TestSubscriptionsList_RoundTripsPrimaryAndTags(t *testing.T) {
	idx := newFakeIndex()
	tags := `["News","Tech"]`
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"https://example.test/feed.xml": {
			Did:       "did:plc:alice",
			Rkey:      "3la",
			AtUri:     "at://did:plc:alice/blue.morgen.feed.subscription/3la",
			FeedUrl:   "https://example.test/feed.xml",
			IsPrimary: 1,
			Tags:      &tags,
		},
	}
	h := SubscriptionsListHandler(idx)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got []SubscriptionWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %d", len(got))
	}
	if !got[0].Primary {
		t.Errorf("primary = false, want true")
	}
	if len(got[0].Tags) != 2 || got[0].Tags[0] != "News" {
		t.Errorf("tags = %v", got[0].Tags)
	}
}

func TestSubscriptionsList_MutedAtTwentyConsecutiveFailures(t *testing.T) {
	idx := newFakeIndex()
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"https://a.example/feed.xml": {Did: "did:plc:alice", Rkey: "1", FeedUrl: "https://a.example/feed.xml"},
		"https://b.example/feed.xml": {Did: "did:plc:alice", Rkey: "2", FeedUrl: "https://b.example/feed.xml"},
	}
	idx.consecutiveFailures["https://a.example/feed.xml"] = 20
	idx.consecutiveFailures["https://b.example/feed.xml"] = 19

	h := SubscriptionsListHandler(idx)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got []SubscriptionWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	byURL := map[string]SubscriptionWire{}
	for _, w := range got {
		byURL[w.FeedURL] = w
	}
	if !byURL["https://a.example/feed.xml"].Muted {
		t.Errorf("20 consecutive failures: muted = false, want true")
	}
	if byURL["https://b.example/feed.xml"].Muted {
		t.Errorf("19 consecutive failures: muted = true, want false")
	}
}

func TestSubscriptionsList_BelowThresholdOmitsMutedFromJSON(t *testing.T) {
	idx := newFakeIndex()
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"https://a.example/feed.xml": {Did: "did:plc:alice", Rkey: "1", FeedUrl: "https://a.example/feed.xml"},
	}
	idx.consecutiveFailures["https://a.example/feed.xml"] = 19

	h := SubscriptionsListHandler(idx)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if body := rr.Body.String(); strings.Contains(body, "muted") {
		t.Errorf("body contains \"muted\" below the mute threshold: %s", body)
	}
}

func TestSubscriptionsList_CarriesLastFetchedAt(t *testing.T) {
	idx := newFakeIndex()
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"https://a.example/feed.xml": {Did: "did:plc:alice", Rkey: "1", FeedUrl: "https://a.example/feed.xml"},
	}
	idx.lastFetchedAt["https://a.example/feed.xml"] = ptrString("2026-07-12T08:00:00Z")

	h := SubscriptionsListHandler(idx)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got []SubscriptionWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].LastFetchedAt != "2026-07-12T08:00:00Z" {
		t.Errorf("got = %+v", got)
	}

	idxNone := newFakeIndex()
	idxNone.rows["did:plc:bob"] = map[string]db.UserSubscription{
		"https://b.example/feed.xml": {Did: "did:plc:bob", Rkey: "1", FeedUrl: "https://b.example/feed.xml"},
	}
	reqNone := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil), "did:plc:bob", "sid-1")
	rrNone := httptest.NewRecorder()
	SubscriptionsListHandler(idxNone).ServeHTTP(rrNone, reqNone)

	if rrNone.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rrNone.Code, rrNone.Body.String())
	}
	if body := rrNone.Body.String(); strings.Contains(body, "lastFetchedAt") {
		t.Errorf("body contains \"lastFetchedAt\" with no fetch recorded: %s", body)
	}
}

// --- Tags handler ---

func TestSubscriptionsTags_DistinctSortedUnion(t *testing.T) {
	idx := newFakeIndex()
	aliceA := `["News","Tech"]`
	aliceB := `["news","Design","apple"]` // "news" dups News (case), keep first-seen "News"
	bobTags := `["BobOnly"]`
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"https://a.test/feed.xml": {Did: "did:plc:alice", Rkey: "1", FeedUrl: "https://a.test/feed.xml", Tags: &aliceA},
		"https://b.test/feed.xml": {Did: "did:plc:alice", Rkey: "2", FeedUrl: "https://b.test/feed.xml", Tags: &aliceB},
	}
	idx.rows["did:plc:bob"] = map[string]db.UserSubscription{
		"https://c.test/feed.xml": {Did: "did:plc:bob", Rkey: "3", FeedUrl: "https://c.test/feed.xml", Tags: &bobTags},
	}

	h := SubscriptionsTagsHandler(idx)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions/tags", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got struct {
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	// Case-insensitive sort ascending, first-seen casing, bob excluded.
	want := []string{"apple", "Design", "News", "Tech"}
	if len(got.Tags) != len(want) {
		t.Fatalf("tags = %v, want %v", got.Tags, want)
	}
	for i := range want {
		if got.Tags[i] != want[i] {
			t.Errorf("tags[%d] = %q, want %q (full: %v)", i, got.Tags[i], want[i], got.Tags)
		}
	}
}

// Guards route precedence: Go's ServeMux must treat the literal /api/subscriptions/tags as more specific than /api/subscriptions/{rkey}, or the wildcard swallows it. Mirrors routes.go order.
func TestSubscriptionsTags_RouteWinsOverRkeyWildcard(t *testing.T) {
	idx := newFakeIndex()
	fakeDetail := newFakeSourceDetail()
	mux := http.NewServeMux()
	mux.Handle("GET /api/subscriptions/tags", SubscriptionsTagsHandler(idx))
	mux.Handle("GET /api/subscriptions/{rkey}", SubscriptionGetHandler(fakeDetail))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions/tags", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	// The tags handler emits {"tags":...}; the rkey handler would 404 or emit a detail wire, so this asserts the tags shape won.
	if body := strings.TrimSpace(rr.Body.String()); body != `{"tags":[]}` {
		t.Errorf("wildcard route swallowed /tags: body = %q", body)
	}
}

func TestSubscriptionsTags_EmptyIsArrayNotNull(t *testing.T) {
	idx := newFakeIndex()
	h := SubscriptionsTagsHandler(idx)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions/tags", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if body := strings.TrimSpace(rr.Body.String()); body != `{"tags":[]}` {
		t.Errorf("body = %q, want {\"tags\":[]}", body)
	}
}
