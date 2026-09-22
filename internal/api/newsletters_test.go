package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"morgenblau/internal/database/db"
	"morgenblau/internal/newsletter"
)

type fakeNewsletterService struct {
	address        string
	createdAddress string
	groups         newsletter.SourceGroups
	source         newsletter.Source
	messages       []newsletter.Message
	message        newsletter.Message
	asset          newsletter.InlineAsset
	save           newsletter.Save
	saves          []newsletter.SaveItem
	err            error
	moveTarget     newsletter.MoveTarget
	deletedSaveID  string
	digestStart    time.Time
	digestEnd      time.Time
}

func (f *fakeNewsletterService) Address(context.Context, string) (string, error) {
	return f.address, f.err
}

func (f *fakeNewsletterService) CreateAddress(context.Context, string) (string, error) {
	return f.createdAddress, f.err
}

func (f *fakeNewsletterService) ListSources(context.Context, string) (newsletter.SourceGroups, error) {
	return f.groups, f.err
}

func (f *fakeNewsletterService) GetSource(context.Context, string, string) (newsletter.Source, error) {
	return f.source, f.err
}

func (f *fakeNewsletterService) PatchSource(context.Context, string, string, newsletter.SourcePatch) (newsletter.Source, error) {
	return f.source, f.err
}

func (f *fakeNewsletterService) StopSource(context.Context, string, string) (newsletter.Source, error) {
	return f.source, f.err
}

func (f *fakeNewsletterService) EnableSource(context.Context, string, string) (newsletter.Source, error) {
	return f.source, f.err
}

func (f *fakeNewsletterService) ListSourceMessages(context.Context, string, string) ([]newsletter.Message, error) {
	return f.messages, f.err
}

func (f *fakeNewsletterService) GetMessageBySlug(context.Context, string, string) (newsletter.Message, error) {
	return f.message, f.err
}

func (f *fakeNewsletterService) AllowRemoteImages(context.Context, string, string) (newsletter.Message, error) {
	return f.message, f.err
}

func (f *fakeNewsletterService) MoveMessage(_ context.Context, _ string, _ string, target newsletter.MoveTarget) (newsletter.Source, error) {
	f.moveTarget = target
	return f.source, f.err
}

func (f *fakeNewsletterService) SaveMessage(context.Context, string, string) (newsletter.Save, error) {
	return f.save, f.err
}

func (f *fakeNewsletterService) DeleteSave(_ context.Context, _ string, id string) error {
	f.deletedSaveID = id
	return f.err
}

func (f *fakeNewsletterService) ListSaves(context.Context, string) ([]newsletter.SaveItem, error) {
	return f.saves, f.err
}

func (f *fakeNewsletterService) GetInlineAsset(context.Context, string, string) (newsletter.InlineAsset, error) {
	return f.asset, f.err
}

func (f *fakeNewsletterService) ListDigestMessages(_ context.Context, _ string, start, end time.Time) ([]newsletter.Message, error) {
	f.digestStart = start
	f.digestEnd = end
	return f.messages, f.err
}

func newsletterSourceFixture() newsletter.Source {
	first := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	last := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	name := "The Sender"
	return newsletter.Source{
		ID: "source-1", Title: "Morning Letter", SenderName: &name,
		SenderAddress: "sender@example.test", Status: newsletter.SourceActive,
		Primary: true, Tags: []string{"reading"}, FirstReceivedAt: &first,
		LastReceivedAt: &last, IssueCount: 9, SavedCount: 2, Count7d: 5,
	}
}

func newsletterMessageFixture() newsletter.Message {
	sent := time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC)
	name := "The Sender"
	saveID := "save-1"
	return newsletter.Message{
		ID: "message-1", EntrySlug: "letter-1", SourceID: "source-1", SourceTitle: "Morning Letter",
		Title: "Issue one", SenderName: &name, SenderAddress: "sender@example.test", SentAt: &sent,
		ReceivedAt: time.Date(2026, 5, 20, 8, 2, 0, 0, time.UTC), BodyHTML: "<p>Hello</p>",
		HasBlockedRemoteImages: true, RemoteImagesAllowed: false, SaveID: &saveID,
	}
}

func TestNewsletterAddress_ReturnsPrivateAddress(t *testing.T) {
	h := NewsletterAddressHandler(&fakeNewsletterService{address: "random@inbound.example.test"})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/newsletters/address", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || rr.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("status = %d, cache = %q, body = %s", rr.Code, rr.Header().Get("Cache-Control"), rr.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["address"] != "random@inbound.example.test" {
		t.Errorf("address = %q", got["address"])
	}
}

func TestNewsletterAddress_UnconfiguredIsUnavailable(t *testing.T) {
	h := NewsletterAddressHandler(&fakeNewsletterService{err: newsletter.ErrUnavailable})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/newsletters/address", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}

func TestNewsletterAddress_UnsetReturnsEmptyAddress(t *testing.T) {
	h := NewsletterAddressHandler(&fakeNewsletterService{err: newsletter.ErrNotFound})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/newsletters/address", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || rr.Body.String() != "{}\n" {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestNewsletterAddressCreate_CreatesPrivateAddress(t *testing.T) {
	h := NewsletterAddressCreateHandler(&fakeNewsletterService{createdAddress: "random@inbound.example.test"})
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/newsletters/address", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || rr.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("status = %d, cache = %q, body = %s", rr.Code, rr.Header().Get("Cache-Control"), rr.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["address"] != "random@inbound.example.test" {
		t.Errorf("address = %q", got["address"])
	}
}

func TestNewslettersList_MapsStatsAndGroups(t *testing.T) {
	source := newsletterSourceFixture()
	stopped := source
	stopped.ID = "source-2"
	stopped.Status = newsletter.SourceStopped
	h := NewslettersListHandler(&fakeNewsletterService{groups: newsletter.SourceGroups{Active: []newsletter.Source{source}, Stopped: []newsletter.Source{stopped}}})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/newsletters", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got newsletterSourcesWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Active) != 1 || len(got.Stopped) != 1 {
		t.Fatalf("groups = %+v", got)
	}
	if got.Active[0].Kind != "newsletter" || got.Active[0].Frequency != "daily" || got.Active[0].SavedByYou != 2 {
		t.Errorf("active source = %+v", got.Active[0])
	}
}

func TestNewsletterGet_CollapsesOwnershipFailureTo404(t *testing.T) {
	h := NewsletterGetHandler(&fakeNewsletterService{err: newsletter.ErrNotFound})
	mux := http.NewServeMux()
	mux.Handle("GET /api/newsletters/{id}", h)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/newsletters/not-mine", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestNewsletterEntries_ReturnEntryWire(t *testing.T) {
	message := newsletterMessageFixture()
	h := NewsletterEntriesHandler(&fakeNewsletterService{messages: []newsletter.Message{message}})
	mux := http.NewServeMux()
	mux.Handle("GET /api/newsletters/{id}/entries", h)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/newsletters/source-1/entries", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	var got []EntryWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ContentType != "newsletter" || got[0].Newsletter == nil {
		t.Fatalf("entries = %+v", got)
	}
	if got[0].Source.Kind != "newsletter" || got[0].Source.ID != "source-1" {
		t.Errorf("source = %+v", got[0].Source)
	}
	if got[0].SavedState == nil || got[0].SavedState.Kind != "newsletter" || got[0].SavedState.ID != "save-1" {
		t.Errorf("savedState = %+v", got[0].SavedState)
	}
}

func TestNewsletterRemoteImages_ReturnAllowedBody(t *testing.T) {
	message := newsletterMessageFixture()
	message.BodyHTML = `<p><img src="https://remote.example/image.png"></p>`
	message.HasBlockedRemoteImages = false
	message.RemoteImagesAllowed = true
	h := NewsletterRemoteImagesHandler(&fakeNewsletterService{message: message})
	mux := http.NewServeMux()
	mux.Handle("POST /api/newsletters/messages/{id}/images", h)
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/newsletters/messages/message-1/images", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["remoteImagesAllowed"] != true || got["hasBlockedRemoteImages"] != false || !strings.Contains(got["body"].(string), "remote.example") {
		t.Errorf("response = %+v", got)
	}
}

func TestNewsletterMove_RequiresExactlyOneTarget(t *testing.T) {
	h := NewsletterMoveHandler(&fakeNewsletterService{})
	mux := http.NewServeMux()
	mux.Handle("POST /api/newsletters/messages/{id}/move", h)
	for _, body := range []string{`{}`, `{"sourceId":"a","newSourceTitle":"b"}`} {
		req := withSession(httptest.NewRequest(http.MethodPost, "/api/newsletters/messages/message-1/move", strings.NewReader(body)), "did:plc:alice", "sid-1")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, rr.Code)
		}
	}
}

func TestNewsletterMove_ReturnsDestinationSource(t *testing.T) {
	svc := &fakeNewsletterService{source: newsletterSourceFixture()}
	h := NewsletterMoveHandler(svc)
	mux := http.NewServeMux()
	mux.Handle("POST /api/newsletters/messages/{id}/move", h)
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/newsletters/messages/message-1/move", strings.NewReader(`{"sourceId":"source-1"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || svc.moveTarget.SourceID != "source-1" {
		t.Fatalf("status = %d, target = %+v, body = %s", rr.Code, svc.moveTarget, rr.Body.String())
	}
	var got struct {
		Source newsletterSourceWire `json:"source"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Source.ID != "source-1" {
		t.Errorf("source = %+v", got.Source)
	}
}

func TestNewsletterSave_CreateAndDeleteStayPrivate(t *testing.T) {
	svc := &fakeNewsletterService{save: newsletter.Save{ID: "save-1", MessageID: "message-1"}}
	create := NewsletterSaveCreateHandler(svc)
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/newsletter-saves", strings.NewReader(`{"messageId":"message-1"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	create.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated || rr.Body.String() != "{\"id\":\"save-1\"}\n" {
		t.Fatalf("create status = %d, body = %s", rr.Code, rr.Body.String())
	}

	remove := NewsletterSaveDeleteHandler(svc)
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/newsletter-saves/{id}", remove)
	req = withSession(httptest.NewRequest(http.MethodDelete, "/api/newsletter-saves/save-1", nil), "did:plc:alice", "sid-1")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent || svc.deletedSaveID != "save-1" {
		t.Fatalf("delete status = %d, id = %q", rr.Code, svc.deletedSaveID)
	}
}

func TestSavesList_AggregatesPrivateNewsletterSaves(t *testing.T) {
	public := newFakeSavesIndex()
	public.list = []db.ListUserSavesRow{{
		Rkey: "feed-save", AtUri: "at://did:plc:alice/blue.morgen.feed.save/feed-save",
		ItemUrl: "https://example.test/post", CreatedAt: "2026-05-20T08:00:00Z",
	}}
	newsletters := &fakeNewsletterService{saves: []newsletter.SaveItem{{
		ID: "newsletter-save", MessageID: "message-1", CreatedAt: time.Date(2026, 5, 21, 8, 0, 0, 0, time.UTC),
		Title: "Issue one", EntrySlug: "letter-1",
	}}}
	h := SavesListHandler(public, newsletters)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/saves", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Cache-Control") != "private, no-store" {
		t.Errorf("Cache-Control = %q", rr.Header().Get("Cache-Control"))
	}
	var got []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0]["kind"] != "newsletter" || got[0]["id"] != "newsletter-save" || got[0]["entrySlug"] != "letter-1" {
		t.Errorf("saves = %+v", got)
	}
	if _, leaked := got[0]["itemUrl"]; leaked {
		t.Errorf("newsletter save leaked public URL fields: %+v", got[0])
	}
}

func TestNewsletterAsset_IsOwnerScopedAndNeverCached(t *testing.T) {
	svc := &fakeNewsletterService{asset: newsletter.InlineAsset{MediaType: "image/png", Data: []byte("png"), ETag: "hash"}}
	h := NewsletterAssetHandler(svc)
	mux := http.NewServeMux()
	mux.Handle("GET /api/newsletter-assets/{token}", h)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/newsletter-assets/token-1", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || rr.Body.String() != "png" {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Cache-Control") != "private, no-store" || rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers = %+v", rr.Header())
	}

	svc.err = newsletter.ErrNotFound
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("not-owned status = %d, want 404", rr.Code)
	}
}

func TestNewsletterErrors_InvalidAndInternal(t *testing.T) {
	for _, tt := range []struct {
		err  error
		want int
	}{
		{newsletter.ErrInvalid, http.StatusBadRequest},
		{errors.New("database down"), http.StatusInternalServerError},
	} {
		h := NewsletterGetHandler(&fakeNewsletterService{err: tt.err})
		mux := http.NewServeMux()
		mux.Handle("GET /api/newsletters/{id}", h)
		req := withSession(httptest.NewRequest(http.MethodGet, "/api/newsletters/source-1", nil), "did:plc:alice", "sid-1")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != tt.want {
			t.Errorf("error %v: status = %d, want %d", tt.err, rr.Code, tt.want)
		}
	}
}
