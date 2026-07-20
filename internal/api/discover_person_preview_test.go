package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"morgenblau/internal/database/db"
	"morgenblau/internal/discoverperson"
	"morgenblau/internal/sharemeta"
)

const previewSessionDID = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"
const previewPersonDID = "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"

type fakePersonInspector struct {
	records       discoverperson.Records
	preview       discoverperson.Preview
	gotDID        string
	gotViewerKeys map[string]struct{}
	recordsCalls  int
}

func (f *fakePersonInspector) Records(_ context.Context, did string, viewerKeys map[string]struct{}) discoverperson.Records {
	f.recordsCalls++
	f.gotDID = did
	f.gotViewerKeys = viewerKeys
	return f.records
}

func (f *fakePersonInspector) Preview(discoverperson.Records) discoverperson.Preview {
	return f.preview
}

func TestDiscoverPersonPreviewHandler_MissingDID_400(t *testing.T) {
	h := DiscoverPersonPreviewHandler(&fakePersonInspector{}, &fakeDiscoverSubsReader{}, noShareMetadata())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people/preview", nil), previewSessionDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body errorEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != codeInvalidRequest {
		t.Errorf("code = %q, want %q", body.Code, codeInvalidRequest)
	}
}

func TestDiscoverPersonPreviewHandler_MalformedDID_400(t *testing.T) {
	h := DiscoverPersonPreviewHandler(&fakePersonInspector{}, &fakeDiscoverSubsReader{}, noShareMetadata())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people/preview?did=not-a-did", nil), previewSessionDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestDiscoverPersonPreviewHandler_Success_200(t *testing.T) {
	sharedAt := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	fake := &fakePersonInspector{
		records: discoverperson.Records{
			Writes: make([]discoverperson.SourceItem, 3),
			Reads:  make([]discoverperson.SourceItem, 5),
		},
		preview: discoverperson.Preview{
			Writes: []discoverperson.SourceItem{
				{Key: "https://a.example/feed", Kind: "rss", Title: "A Feed", SiteURL: "https://a.example", Subscribed: false},
			},
			Reads: []discoverperson.SourceItem{
				{Key: "at://did:plc:cccccccccccccccccccccccc/site.standard.publication/self", Kind: "standardfeed", Title: "Example Publication", Subscribed: true},
			},
			LatestShare: &discoverperson.ShareItem{
				ItemURL:   "https://shared.example/post",
				Document:  "at://did:plc:cccccccccccccccccccccccc/site.standard.document/3example",
				Comment:   "worth a read",
				CreatedAt: sharedAt,
			},
		},
	}
	subs := &fakeDiscoverSubsReader{rows: []db.UserSubscription{
		{FeedUrl: "https://a.example/feed"},
	}}

	metadata := noShareMetadata()
	metadata.byKey["at://did:plc:cccccccccccccccccccccccc/site.standard.document/3example"] = sharemeta.Metadata{
		Title: "Resolved document", TargetURL: "https://publication.example/post", EntrySlug: "resolved-doc",
	}
	h := DiscoverPersonPreviewHandler(fake, subs, metadata)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people/preview?did="+previewPersonDID, nil), previewSessionDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if fake.gotDID != previewPersonDID {
		t.Errorf("gotDID = %q, want %q", fake.gotDID, previewPersonDID)
	}
	if _, ok := fake.gotViewerKeys["https://a.example/feed"]; !ok {
		t.Errorf("viewerKeys = %v, want normalized subscription feed url present", fake.gotViewerKeys)
	}

	var got struct {
		Writes      []DiscoverPersonSourceWire `json:"writes"`
		WritesTotal int                        `json:"writesTotal"`
		Reads       []DiscoverPersonSourceWire `json:"reads"`
		ReadsTotal  int                        `json:"readsTotal"`
		LatestShare *struct {
			ItemURL   string `json:"itemUrl"`
			Document  string `json:"document"`
			Comment   string `json:"comment"`
			CreatedAt string `json:"createdAt"`
			Title     string `json:"title"`
			TargetURL string `json:"targetUrl"`
			EntrySlug string `json:"entrySlug"`
		} `json:"latestShare"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(got.Writes) != 1 || got.Writes[0].Key != "https://a.example/feed" || got.Writes[0].FeedURL != "https://a.example/feed" || got.Writes[0].Publication != "" {
		t.Errorf("Writes = %+v", got.Writes)
	}
	if got.Writes[0].Subscribed {
		t.Errorf("Writes[0].Subscribed = true, want false")
	}
	if got.WritesTotal != 3 {
		t.Errorf("WritesTotal = %d, want 3", got.WritesTotal)
	}
	if len(got.Reads) != 1 || got.Reads[0].Publication == "" || got.Reads[0].FeedURL != "" {
		t.Errorf("Reads = %+v, want kind=standardfeed to set Publication not FeedURL", got.Reads)
	}
	if !got.Reads[0].Subscribed {
		t.Errorf("Reads[0].Subscribed = false, want true")
	}
	if got.ReadsTotal != 5 {
		t.Errorf("ReadsTotal = %d, want 5", got.ReadsTotal)
	}
	if got.LatestShare == nil || got.LatestShare.ItemURL != "https://shared.example/post" || got.LatestShare.Comment != "worth a read" {
		t.Errorf("LatestShare = %+v", got.LatestShare)
	}
	if got.LatestShare.Document != "at://did:plc:cccccccccccccccccccccccc/site.standard.document/3example" {
		t.Errorf("LatestShare.Document = %q, want document AT-URI", got.LatestShare.Document)
	}
	if got.LatestShare.Title != "Resolved document" || got.LatestShare.TargetURL != "https://publication.example/post" || got.LatestShare.EntrySlug != "resolved-doc" {
		t.Errorf("LatestShare metadata = %+v", got.LatestShare)
	}
}

func TestDiscoverPersonPreviewHandler_EmptySections_EncodeAsEmptyArraysAndNullShare(t *testing.T) {
	fake := &fakePersonInspector{preview: discoverperson.Preview{}}
	h := DiscoverPersonPreviewHandler(fake, &fakeDiscoverSubsReader{}, noShareMetadata())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people/preview?did="+previewPersonDID, nil), previewSessionDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, `"writes":null`) || strings.Contains(body, `"reads":null`) {
		t.Fatalf("body = %s, writes/reads must encode as [] not null", body)
	}
	if !strings.Contains(body, `"latestShare":null`) {
		t.Fatalf("body = %s, latestShare must encode as explicit null when absent", body)
	}
}

func TestDiscoverPersonPreviewHandler_NeverLeaksSaveKey(t *testing.T) {
	fake := &fakePersonInspector{
		preview: discoverperson.Preview{
			Writes:      []discoverperson.SourceItem{{Key: "https://a.example/feed", Kind: "rss", Subscribed: true}},
			Reads:       []discoverperson.SourceItem{{Key: "https://b.example/feed", Kind: "rss"}},
			LatestShare: &discoverperson.ShareItem{ItemURL: "https://shared.example/post"},
		},
	}
	h := DiscoverPersonPreviewHandler(fake, &fakeDiscoverSubsReader{}, noShareMetadata())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people/preview?did="+previewPersonDID, nil), previewSessionDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if strings.Contains(strings.ToLower(rec.Body.String()), "save") {
		t.Fatalf("body leaks a save-related key: %s", rec.Body.String())
	}
}
