package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
)

func TestSubscriptionsCreate_HappyPath_FullChoiceA(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp, nil)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","title":"Example","siteUrl":"https://example.test"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got addResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 1 {
		t.Fatalf("records = %d", len(got.Records))
	}
	if len(got.JobIDs) != 1 {
		t.Errorf("jobs = %d", len(got.JobIDs))
	}
	if pds.creates != 1 {
		t.Errorf("PDS creates = %d", pds.creates)
	}
	if len(disp.dispatched) != 1 {
		t.Errorf("dispatcher = %v", disp.dispatched)
	}

	// Tier-1 index should now show the new row.
	rows, _ := idx.ListUserSubscriptions(context.Background(), "did:plc:alice")
	if len(rows) != 1 {
		t.Errorf("Tier-1 rows = %d", len(rows))
	}
}

func TestSubscriptionsCreate_DedupeGuard_Idempotent(t *testing.T) {
	idx := newFakeIndex()
	feed := "https://example.test/feed.xml"
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		feed: {
			Did:     "did:plc:alice",
			Rkey:    "3laOLD",
			AtUri:   "at://did:plc:alice/blue.morgen.feed.subscription/3laOLD",
			FeedUrl: feed,
			Title:   ptrString("Existing"),
		},
	}
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp, nil)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","title":"NewName"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if pds.creates != 0 {
		t.Errorf("PDS create called on dedupe path: %d", pds.creates)
	}
	if len(disp.dispatched) != 0 {
		t.Errorf("dispatcher should not fire on dedupe: %v", disp.dispatched)
	}
}

func TestSubscriptionsCreate_PDSFailure_502(t *testing.T) {
	idx := newFakeIndex()
	pds := &failingPDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp, nil)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestSubscriptionsCreate_Validation(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "empty list", body: `{"subscriptions":[]}`, want: "no subscriptions submitted"},
		{name: "missing feed URL", body: `{"subscriptions":[{"title":"Example"}]}`, want: "subscriptions.0.feedUrl"},
		{name: "malformed JSON", body: `{"subscriptions":[`, want: "invalid json"},
		{name: "both feedUrl and publication", body: `{"subscriptions":[{"feedUrl":"https://x/feed.xml","publication":"at://did:plc:p/site.standard.publication/3p"}]}`, want: "subscriptions.0.publication"},
		{name: "publication not an at-uri", body: `{"subscriptions":[{"publication":"https://not-an-at-uri.example"}]}`, want: "subscriptions.0.publication"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			h := SubscriptionsCreateHandler(newFakeIndex(), newFakeIndex(), &fakePDS{}, &fakeDispatcher{}, nil)
			req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(tt.body)), "did:plc:alice", "sid-1")
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tt.want) {
				t.Errorf("body = %q, want substring %q", rr.Body.String(), tt.want)
			}
		})
	}
}

func TestSubscriptionsCreate_DedupeProbeError_500(t *testing.T) {
	idx := newFakeIndex()
	idx.getFeedErr = errors.New("database unavailable")
	h := SubscriptionsCreateHandler(idx, idx, &fakePDS{}, &fakeDispatcher{}, nil)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

// upsertErrIndex is a fakeIndex variant whose UpsertUserSubscription always errors, to exercise the Tier-1-failure to sync_user dispatch recovery path.
type upsertErrIndex struct{ *fakeIndex }

func (e upsertErrIndex) UpsertUserSubscription(_ context.Context, _ db.UpsertUserSubscriptionParams) error {
	return errors.New("tier-1 down")
}

func TestSubscriptionsCreate_Tier1Failure_DispatchesSyncUser(t *testing.T) {
	idx := newFakeIndex()
	writer := upsertErrIndex{idx}
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, writer, pds, disp, nil)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","title":"Example"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (PDS succeeded, tier-1 fallback should still return success): body = %s", rr.Code, rr.Body.String())
	}
	if disp.manualSync != 1 {
		t.Errorf("manual sync dispatches = %d, want 1", disp.manualSync)
	}
}

// --- standardfeed create ---

const standardSubCollection = "site.standard.graph.subscription"

func TestSubscriptionsCreate_Standardfeed_DefaultsCreateOnlyStandardRecord(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp, nil)

	body := `{"subscriptions":[{"publication":"` + testPublication + `","siteUrl":"https://blog.example"}]}`
	req := withStandardWriteSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	// Exactly one PDS write (the portable standard record); no sidecar since the picker didn't customize anything.
	if pds.creates != 1 {
		t.Fatalf("PDS creates = %d, want 1 (got %+v)", pds.creates, pds.created)
	}
	if pds.created[0].collection != standardSubCollection {
		t.Errorf("collection = %q, want %s", pds.created[0].collection, standardSubCollection)
	}
	rec := pds.created[0].record
	if rec["publication"] != testPublication {
		t.Errorf("record.publication = %v", rec["publication"])
	}
	if _, ok := rec["createdAt"].(string); !ok {
		t.Errorf("record.createdAt missing: %v", rec)
	}
	if _, ok := rec["source"]; ok {
		t.Errorf("standard record must not carry a blue.morgen source union: %v", rec)
	}

	// Tier-2 catalog row keyed by the publication at-uri, kind standardfeed.
	if len(idx.feedParams) != 1 || idx.feedParams[0].FeedUrl != testPublication {
		t.Fatalf("UpsertFeed params = %+v", idx.feedParams)
	}
	if kind, _ := idx.feedParams[0].Kind.(string); kind != "standardfeed" {
		t.Errorf("UpsertFeed kind = %v", idx.feedParams[0].Kind)
	}
	if idx.feedParams[0].SiteUrl == nil || *idx.feedParams[0].SiteUrl != "https://blog.example" {
		t.Errorf("UpsertFeed siteUrl = %v", idx.feedParams[0].SiteUrl)
	}

	// Tier-1 row: existence rkey is the STANDARD record's rkey, no sidecar.
	row, err := idx.GetUserSubscriptionByFeedURL(context.Background(), db.GetUserSubscriptionByFeedURLParams{
		Did: "did:plc:alice", FeedUrl: testPublication,
	})
	if err != nil {
		t.Fatalf("row lookup: %v", err)
	}
	if row.Kind != "standardfeed" {
		t.Errorf("row kind = %q", row.Kind)
	}
	if row.Rkey != "3la1" {
		t.Errorf("row rkey = %q, want the standard record rkey 3la1", row.Rkey)
	}
	if row.SidecarRkey != nil {
		t.Errorf("row sidecar_rkey = %v, want nil", *row.SidecarRkey)
	}

	// Fetch dispatched for the publication key.
	if len(disp.dispatched) != 1 || disp.dispatched[0] != testPublication {
		t.Errorf("dispatched = %v", disp.dispatched)
	}

	var got addResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 1 {
		t.Fatalf("records = %d", len(got.Records))
	}
	wire := got.Records[0]
	if wire.Kind != "standardfeed" || wire.Publication != testPublication || wire.FeedURL != testPublication {
		t.Errorf("wire = %+v", wire)
	}
	if wire.URI != "at://did:plc:alice/"+standardSubCollection+"/3la1" {
		t.Errorf("wire uri = %q", wire.URI)
	}
}

func TestSubscriptionsCreate_Standardfeed_CustomMetadata_SidecarSecond(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	h := SubscriptionsCreateHandler(idx, idx, pds, &fakeDispatcher{}, nil)

	body := `{"subscriptions":[{"publication":"` + testPublication + `","title":"My Name","primary":true,"tags":["News"]}]}`
	req := withStandardWriteSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if pds.creates != 2 {
		t.Fatalf("PDS creates = %d, want 2 (standard + sidecar)", pds.creates)
	}
	// Order pins the failure contract: the portable record lands first, so a sidecar failure still leaves an adoptable subscription.
	if pds.created[0].collection != standardSubCollection {
		t.Errorf("first write collection = %q, want standard record first", pds.created[0].collection)
	}
	if pds.created[1].collection != "blue.morgen.feed.subscription" {
		t.Errorf("second write collection = %q, want blue.morgen sidecar", pds.created[1].collection)
	}
	sidecar := pds.created[1].record
	source, ok := sidecar["source"].(map[string]any)
	if !ok || source["$type"] != "blue.morgen.feed.subscription#standardPublication" || source["publication"] != testPublication {
		t.Errorf("sidecar source = %v", sidecar["source"])
	}
	if sidecar["title"] != "My Name" || sidecar["primary"] != true {
		t.Errorf("sidecar metadata = %v", sidecar)
	}

	row, err := idx.GetUserSubscriptionByFeedURL(context.Background(), db.GetUserSubscriptionByFeedURLParams{
		Did: "did:plc:alice", FeedUrl: testPublication,
	})
	if err != nil {
		t.Fatalf("row lookup: %v", err)
	}
	if row.Rkey != "3la1" {
		t.Errorf("row rkey = %q, want standard rkey 3la1", row.Rkey)
	}
	if row.SidecarRkey == nil || *row.SidecarRkey != "3la2" {
		t.Errorf("row sidecar_rkey = %v, want 3la2", row.SidecarRkey)
	}
	if row.Title == nil || *row.Title != "My Name" {
		t.Errorf("row title = %v", row.Title)
	}
}

func TestSubscriptionsCreate_Standardfeed_StaleScope_403(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp, nil)

	// withSession carries no scopes, simulating a pre-change grant.
	body := `{"subscriptions":[{"publication":"` + testPublication + `"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "reauth_required") {
		t.Errorf("body = %q, want reauth_required code", rr.Body.String())
	}
	if pds.creates != 0 {
		t.Errorf("PDS creates = %d, want 0 on stale scope", pds.creates)
	}
	if len(disp.dispatched) != 0 {
		t.Errorf("dispatched = %v, want none", disp.dispatched)
	}
}

func TestSubscriptionsCreate_Standardfeed_Dedupe_Idempotent(t *testing.T) {
	idx := newFakeIndex()
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		testPublication: {
			Did:     "did:plc:alice",
			Rkey:    "3laOLD",
			AtUri:   "at://did:plc:alice/" + standardSubCollection + "/3laOLD",
			FeedUrl: testPublication,
			Kind:    "standardfeed",
		},
	}
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp, nil)

	body := `{"subscriptions":[{"publication":"` + testPublication + `"}]}`
	req := withStandardWriteSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if pds.creates != 0 {
		t.Errorf("PDS creates on dedupe path: %d", pds.creates)
	}
	if len(disp.dispatched) != 0 {
		t.Errorf("dispatch on dedupe path: %v", disp.dispatched)
	}
	var got addResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 1 || got.Records[0].Kind != "standardfeed" || got.Records[0].Publication != testPublication {
		t.Errorf("records = %+v", got.Records)
	}
}

func TestSubscriptionsCreate_MixedBatch_BothKinds(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp, nil)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","title":"RSS"},{"publication":"` + testPublication + `"}]}`
	req := withStandardWriteSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if pds.creates != 2 {
		t.Fatalf("PDS creates = %d, want 2", pds.creates)
	}
	if pds.created[0].collection != "blue.morgen.feed.subscription" || pds.created[1].collection != standardSubCollection {
		t.Errorf("collections = %q, %q", pds.created[0].collection, pds.created[1].collection)
	}
	var got addResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 2 || got.Records[0].Kind != "rss" || got.Records[1].Kind != "standardfeed" {
		t.Errorf("records = %+v", got.Records)
	}
	if len(disp.dispatched) != 2 {
		t.Errorf("dispatched = %v", disp.dispatched)
	}
}

func TestSubscriptionsCreate_SiblingPairInBatch_409(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp, nil)

	// An rss feed and a publication for the SAME site in one batch.
	body := `{"subscriptions":[
		{"feedUrl":"https://blog.example.test/feed.xml","siteUrl":"https://blog.example.test"},
		{"publication":"` + testPublication + `","siteUrl":"https://blog.example.test"}
	]}`
	req := withStandardWriteSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rr.Code, rr.Body.String())
	}
	if pds.creates != 0 {
		t.Errorf("PDS creates = %d, want 0", pds.creates)
	}
	if len(disp.dispatched) != 0 {
		t.Errorf("dispatched = %v", disp.dispatched)
	}
}

// --- primary + tags ---

func TestSubscriptionsCreate_PrimaryAndTags_PersistedEverywhere(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp, nil)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","title":"Example","primary":true,"tags":["News","Tech"]}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	// PDS record map carries primary + tags.
	if pds.lastRec["primary"] != true {
		t.Errorf("PDS record primary = %v, want true", pds.lastRec["primary"])
	}
	if tags, ok := pds.lastRec["tags"].([]string); !ok || len(tags) != 2 || tags[0] != "News" || tags[1] != "Tech" {
		t.Errorf("PDS record tags = %v (%T), want [News Tech]", pds.lastRec["tags"], pds.lastRec["tags"])
	}
	// Rev-2 shape: identity lives in the required source union, not flat.
	recSource, ok := pds.lastRec["source"].(map[string]any)
	if !ok || recSource["$type"] != "blue.morgen.feed.subscription#rssFeed" || recSource["feedUrl"] != "https://example.test/feed.xml" {
		t.Errorf("PDS record source = %v, want rssFeed variant", pds.lastRec["source"])
	}
	if _, flat := pds.lastRec["feedUrl"]; flat {
		t.Errorf("PDS record must not carry flat feedUrl: %v", pds.lastRec["feedUrl"])
	}

	// Tier-1 upsert persisted them (round-trip via the stored row).
	row, err := idx.GetUserSubscriptionByFeedURL(context.Background(), db.GetUserSubscriptionByFeedURLParams{
		Did:     "did:plc:alice",
		FeedUrl: "https://example.test/feed.xml",
	})
	if err != nil {
		t.Fatalf("GetUserSubscriptionByFeedURL: %v", err)
	}
	if row.IsPrimary != 1 {
		t.Errorf("stored is_primary = %d, want 1", row.IsPrimary)
	}
	if row.Tags == nil || *row.Tags != `["News","Tech"]` {
		t.Errorf("stored tags = %v, want JSON [News Tech]", row.Tags)
	}

	// Response wire echoes them.
	var got addResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 1 {
		t.Fatalf("records = %d", len(got.Records))
	}
	rec := got.Records[0]
	if !rec.Primary {
		t.Errorf("response primary = %v, want true", rec.Primary)
	}
	if len(rec.Tags) != 2 || rec.Tags[0] != "News" || rec.Tags[1] != "Tech" {
		t.Errorf("response tags = %v", rec.Tags)
	}
	if rec.Value["primary"] != true {
		t.Errorf("response value.primary = %v", rec.Value["primary"])
	}
}

func TestSubscriptionsCreate_DefaultsOmitPrimaryAndTags(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	h := SubscriptionsCreateHandler(idx, idx, pds, &fakeDispatcher{}, nil)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","title":"Example"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if _, ok := pds.lastRec["primary"]; ok {
		t.Errorf("PDS record should omit primary when false: %v", pds.lastRec["primary"])
	}
	if _, ok := pds.lastRec["tags"]; ok {
		t.Errorf("PDS record should omit tags when empty: %v", pds.lastRec["tags"])
	}

	var got addResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	rec := got.Records[0]
	if rec.Primary {
		t.Errorf("response primary = true, want false")
	}
	// tags is `omitempty` so it should be absent from JSON entirely.
	if strings.Contains(rr.Body.String(), `"tags"`) {
		t.Errorf("response should omit tags key when empty: %s", rr.Body.String())
	}
}

func TestSubscriptionsCreate_TagNormalization(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	h := SubscriptionsCreateHandler(idx, idx, pds, &fakeDispatcher{}, nil)

	long := strings.Repeat("x", 65) // 65 runes > 64, must be dropped
	tags := []string{
		" News ", // trimmed → "News"
		"news",   // case-dup of News → dropped, first-seen casing kept
		"",       // blank → dropped
		"  ",     // blank → dropped
		"Tech",
		"TECH",                                 // case-dup → dropped
		long,                                   // over 64 graphemes → dropped
		"a", "b", "c", "d", "e", "f", "g", "h", // pushes well past 10 total
	}
	jsonTags, _ := json.Marshal(tags)
	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","tags":` + string(jsonTags) + `}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got addResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	out := got.Records[0].Tags

	if len(out) > 10 {
		t.Errorf("tags not capped at 10: %v", out)
	}
	if out[0] != "News" || out[1] != "Tech" {
		t.Errorf("expected first-seen casing News, Tech; got %v", out)
	}
	for _, tag := range out {
		if tag == "" {
			t.Errorf("blank tag survived: %v", out)
		}
		if len([]rune(tag)) > 64 {
			t.Errorf("over-64-grapheme tag survived: %q", tag)
		}
	}
	// "news"/"TECH" must not appear (case-dedupe).
	seen := map[string]int{}
	for _, tag := range out {
		seen[strings.ToLower(tag)]++
	}
	for k, n := range seen {
		if n > 1 {
			t.Errorf("case-duplicate %q appears %d times: %v", k, n, out)
		}
	}
}

// TestSubscriptionsCreate_InvalidRecord_500_NoWrite proves an over-cap title (unenforced at the app-input layer) is rejected before the PDS write, not merely logged.
func TestSubscriptionsCreate_InvalidRecord_500_NoWrite(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp, nil)

	overlong := strings.Repeat("x", 200) // > maxGraphemes:128
	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","title":"` + overlong + `"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid_record") {
		t.Errorf("body = %q, want invalid_record code", rr.Body.String())
	}
	if pds.creates != 0 {
		t.Errorf("PDS creates = %d, want 0 (validation must run before the write)", pds.creates)
	}
	if len(disp.dispatched) != 0 {
		t.Errorf("dispatched = %v, want none", disp.dispatched)
	}
}

type failingPDS struct{}

func (failingPDS) CreateRecord(_ context.Context, _ *oauth.ClientSession, _ syntax.NSID, _ map[string]any) (*atprepo.RecordRef, error) {
	return nil, errors.New("pds down")
}

func (failingPDS) PutRecord(_ context.Context, _ *oauth.ClientSession, _ syntax.NSID, _ string, _ map[string]any) (*atprepo.RecordRef, error) {
	return nil, errors.New("pds down")
}

func (failingPDS) DeleteRecord(_ context.Context, _ *oauth.ClientSession, _ syntax.NSID, _ string) error {
	return errors.New("pds down")
}

func (failingPDS) ListRecords(_ context.Context, _ *oauth.ClientSession, _ syntax.NSID) ([]atprepo.ListedRecord, error) {
	return nil, errors.New("pds down")
}
