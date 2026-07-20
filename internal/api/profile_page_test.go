package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/cache/profiles"
	"morgenblau/internal/database/db"
	"morgenblau/internal/discoverperson"
	"morgenblau/internal/sharemeta"
)

const (
	profileSelfDID   = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"
	profileTargetDID = "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"
	profileTargetHdl = "bob.example"
)

// seededProfileSource is a fakeProfileSource pre-loaded so Get/Refresh succeed for did.
func seededProfileSource(did syntax.DID, handle string) *fakeProfileSource {
	p := profiles.Profile{DID: did.String(), Handle: handle}
	return &fakeProfileSource{
		profiles:        map[syntax.DID]profiles.Profile{did: p},
		refreshProfiles: map[syntax.DID]profiles.Profile{did: p},
	}
}

// fakeAtIdentifierResolver stubs bidirectionally-verified handle-or-did resolution.
type fakeAtIdentifierResolver struct {
	byID map[syntax.AtIdentifier]*identity.Identity
	err  error
}

func newFakeAtIdentifierResolver() *fakeAtIdentifierResolver {
	return &fakeAtIdentifierResolver{byID: map[syntax.AtIdentifier]*identity.Identity{}}
}

func (f *fakeAtIdentifierResolver) seed(raw string, did syntax.DID, handle syntax.Handle) {
	atid, err := syntax.ParseAtIdentifier(raw)
	if err != nil {
		panic(err)
	}
	f.byID[atid] = &identity.Identity{DID: did, Handle: handle}
}

func (f *fakeAtIdentifierResolver) Lookup(_ context.Context, atid syntax.AtIdentifier) (*identity.Identity, error) {
	if f.err != nil {
		return nil, f.err
	}
	if ident, ok := f.byID[atid]; ok {
		return ident, nil
	}
	return nil, errors.New("not found")
}

// requestWithPathValues attaches PathValue lookups (id, segment) the way net/http's ServeMux would after route matching, since httptest.NewRequest bypasses the mux.
func requestWithPathValues(method, target string, values map[string]string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	for k, v := range values {
		req.SetPathValue(k, v)
	}
	return req
}

func TestProfileHandler_HandleFormResolution(t *testing.T) {
	resolver := newFakeAtIdentifierResolver()
	targetDID, _ := syntax.ParseDID(profileTargetDID)
	handle, _ := syntax.ParseHandle(profileTargetHdl)
	resolver.seed(profileTargetHdl, targetDID, handle)

	h := ProfileHandler(resolver, seededProfileSource(targetDID, profileTargetHdl), newFakeFollowsIndex(), &fakeDiscoverSubsReader{}, &fakePersonInspector{})
	req := withSession(requestWithPathValues(http.MethodGet, "/api/profile/"+profileTargetHdl, map[string]string{"id": profileTargetHdl}), profileSelfDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got ProfileWire
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DID != profileTargetDID {
		t.Errorf("DID = %q, want %q", got.DID, profileTargetDID)
	}
}

func TestProfileHandler_DIDFormResolution(t *testing.T) {
	resolver := newFakeAtIdentifierResolver()
	targetDID, _ := syntax.ParseDID(profileTargetDID)
	handle, _ := syntax.ParseHandle(profileTargetHdl)
	resolver.seed(profileTargetDID, targetDID, handle)

	h := ProfileHandler(resolver, seededProfileSource(targetDID, profileTargetHdl), newFakeFollowsIndex(), &fakeDiscoverSubsReader{}, &fakePersonInspector{})
	req := withSession(requestWithPathValues(http.MethodGet, "/api/profile/"+profileTargetDID, map[string]string{"id": profileTargetDID}), profileSelfDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got ProfileWire
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DID != profileTargetDID {
		t.Errorf("DID = %q, want %q", got.DID, profileTargetDID)
	}
}

func TestProfileHandler_UnresolvableIdentity_404(t *testing.T) {
	resolver := newFakeAtIdentifierResolver() // nothing seeded
	h := ProfileHandler(resolver, &fakeProfileSource{}, newFakeFollowsIndex(), &fakeDiscoverSubsReader{}, &fakePersonInspector{})
	req := withSession(requestWithPathValues(http.MethodGet, "/api/profile/nobody.example", map[string]string{"id": "nobody.example"}), profileSelfDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body errorEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != codeNotFound {
		t.Errorf("code = %q, want %q", body.Code, codeNotFound)
	}
}

func TestProfileHandler_InvalidIdentitySyntax_404(t *testing.T) {
	resolver := newFakeAtIdentifierResolver()
	h := ProfileHandler(resolver, &fakeProfileSource{}, newFakeFollowsIndex(), &fakeDiscoverSubsReader{}, &fakePersonInspector{})
	req := withSession(requestWithPathValues(http.MethodGet, "/api/profile/not-a-valid-identifier", map[string]string{"id": "not-a-valid-identifier"}), profileSelfDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestProfileHandler_FollowRkey_NullWhenAbsent(t *testing.T) {
	resolver := newFakeAtIdentifierResolver()
	targetDID, _ := syntax.ParseDID(profileTargetDID)
	handle, _ := syntax.ParseHandle(profileTargetHdl)
	resolver.seed(profileTargetDID, targetDID, handle)

	h := ProfileHandler(resolver, seededProfileSource(targetDID, profileTargetHdl), newFakeFollowsIndex(), &fakeDiscoverSubsReader{}, &fakePersonInspector{})
	req := withSession(requestWithPathValues(http.MethodGet, "/api/profile/"+profileTargetDID, map[string]string{"id": profileTargetDID}), profileSelfDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	var got ProfileWire
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.FollowRkey != nil {
		t.Errorf("FollowRkey = %v, want nil", *got.FollowRkey)
	}
	if !strings.Contains(body, `"followRkey":null`) {
		t.Errorf("body = %s, want explicit followRkey:null", body)
	}
}

func TestProfileHandler_FollowRkey_SetWhenPresent(t *testing.T) {
	resolver := newFakeAtIdentifierResolver()
	targetDID, _ := syntax.ParseDID(profileTargetDID)
	handle, _ := syntax.ParseHandle(profileTargetHdl)
	resolver.seed(profileTargetDID, targetDID, handle)

	follows := newFakeFollowsIndex()
	follows.seed(db.UserFollow{Did: profileSelfDID, Rkey: "abc123", SubjectDid: profileTargetDID, AtUri: "at://" + profileSelfDID + "/blue.morgen.graph.follow/abc123"})

	h := ProfileHandler(resolver, seededProfileSource(targetDID, profileTargetHdl), follows, &fakeDiscoverSubsReader{}, &fakePersonInspector{})
	req := withSession(requestWithPathValues(http.MethodGet, "/api/profile/"+profileTargetDID, map[string]string{"id": profileTargetDID}), profileSelfDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	var got ProfileWire
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.FollowRkey == nil || *got.FollowRkey != "abc123" {
		t.Errorf("FollowRkey = %v, want \"abc123\"", got.FollowRkey)
	}
}

func TestProfileHandler_IsSelf(t *testing.T) {
	resolver := newFakeAtIdentifierResolver()
	selfDID, _ := syntax.ParseDID(profileSelfDID)
	resolver.seed(profileSelfDID, selfDID, "")

	follows := newFakeFollowsIndex()
	follows.seed(db.UserFollow{
		Did:        profileSelfDID,
		Rkey:       "self-follow",
		SubjectDid: profileSelfDID,
	})
	h := ProfileHandler(resolver, seededProfileSource(selfDID, profileSelfDID), follows, &fakeDiscoverSubsReader{}, &fakePersonInspector{})
	req := withSession(requestWithPathValues(http.MethodGet, "/api/profile/"+profileSelfDID, map[string]string{"id": profileSelfDID}), profileSelfDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	var got ProfileWire
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.IsSelf {
		t.Errorf("IsSelf = false, want true")
	}
	if got.FollowRkey != nil {
		t.Errorf("FollowRkey = %q, want nil for self", *got.FollowRkey)
	}
}

func TestProfileHandler_Counts(t *testing.T) {
	resolver := newFakeAtIdentifierResolver()
	targetDID, _ := syntax.ParseDID(profileTargetDID)
	handle, _ := syntax.ParseHandle(profileTargetHdl)
	resolver.seed(profileTargetDID, targetDID, handle)

	inspector := &fakePersonInspector{records: discoverperson.Records{
		Writes: []discoverperson.SourceItem{{Key: "https://a.example/feed"}, {Key: "https://b.example/feed"}},
		Reads:  []discoverperson.SourceItem{{Key: "https://c.example/feed"}},
		Shares: []discoverperson.ShareItem{{ItemURL: "https://shared.example/1"}, {ItemURL: "https://shared.example/2"}, {ItemURL: "https://shared.example/3"}},
	}}

	h := ProfileHandler(resolver, seededProfileSource(targetDID, profileTargetHdl), newFakeFollowsIndex(), &fakeDiscoverSubsReader{}, inspector)
	req := withSession(requestWithPathValues(http.MethodGet, "/api/profile/"+profileTargetDID, map[string]string{"id": profileTargetDID}), profileSelfDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	var got ProfileWire
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Counts.Writes != 2 || got.Counts.Reads != 1 || got.Counts.Shares != 3 {
		t.Errorf("Counts = %+v, want {2 1 3}", got.Counts)
	}
}

func TestProfileHandler_ZeroRecords_ZeroCounts200(t *testing.T) {
	resolver := newFakeAtIdentifierResolver()
	targetDID, _ := syntax.ParseDID(profileTargetDID)
	handle, _ := syntax.ParseHandle(profileTargetHdl)
	resolver.seed(profileTargetDID, targetDID, handle)

	h := ProfileHandler(resolver, seededProfileSource(targetDID, profileTargetHdl), newFakeFollowsIndex(), &fakeDiscoverSubsReader{}, &fakePersonInspector{})
	req := withSession(requestWithPathValues(http.MethodGet, "/api/profile/"+profileTargetDID, map[string]string{"id": profileTargetDID}), profileSelfDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got ProfileWire
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Counts.Writes != 0 || got.Counts.Reads != 0 || got.Counts.Shares != 0 {
		t.Errorf("Counts = %+v, want all zero", got.Counts)
	}
}

func TestProfileHandler_NeverLeaksSaveKey(t *testing.T) {
	resolver := newFakeAtIdentifierResolver()
	targetDID, _ := syntax.ParseDID(profileTargetDID)
	handle, _ := syntax.ParseHandle(profileTargetHdl)
	resolver.seed(profileTargetDID, targetDID, handle)

	inspector := &fakePersonInspector{records: discoverperson.Records{
		Writes: []discoverperson.SourceItem{{Key: "https://a.example/feed", Subscribed: true}},
		Shares: []discoverperson.ShareItem{{ItemURL: "https://shared.example/1"}},
	}}

	h := ProfileHandler(resolver, seededProfileSource(targetDID, profileTargetHdl), newFakeFollowsIndex(), &fakeDiscoverSubsReader{}, inspector)
	req := withSession(requestWithPathValues(http.MethodGet, "/api/profile/"+profileTargetDID, map[string]string{"id": profileTargetDID}), profileSelfDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if strings.Contains(strings.ToLower(rec.Body.String()), "save") {
		t.Fatalf("body leaks a save-related key: %s", rec.Body.String())
	}
}

// --- ProfileSegmentHandler ---

func TestProfileSegmentHandler_BadSegment_404(t *testing.T) {
	resolver := newFakeAtIdentifierResolver()
	targetDID, _ := syntax.ParseDID(profileTargetDID)
	handle, _ := syntax.ParseHandle(profileTargetHdl)
	resolver.seed(profileTargetDID, targetDID, handle)

	h := ProfileSegmentHandler(resolver, &fakeDiscoverSubsReader{}, &fakePersonInspector{}, noShareMetadata())
	req := withSession(requestWithPathValues(http.MethodGet, "/api/profile/"+profileTargetDID+"/bogus", map[string]string{"id": profileTargetDID, "segment": "bogus"}), profileSelfDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestProfileSegmentHandler_UnresolvableIdentity_404(t *testing.T) {
	resolver := newFakeAtIdentifierResolver()
	h := ProfileSegmentHandler(resolver, &fakeDiscoverSubsReader{}, &fakePersonInspector{}, noShareMetadata())
	req := withSession(requestWithPathValues(http.MethodGet, "/api/profile/nobody.example/writes", map[string]string{"id": "nobody.example", "segment": "writes"}), profileSelfDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestProfileSegmentHandler_PaginationBoundary(t *testing.T) {
	resolver := newFakeAtIdentifierResolver()
	targetDID, _ := syntax.ParseDID(profileTargetDID)
	handle, _ := syntax.ParseHandle(profileTargetHdl)
	resolver.seed(profileTargetDID, targetDID, handle)

	items := make([]discoverperson.SourceItem, 11)
	for i := range items {
		items[i] = discoverperson.SourceItem{Key: fmt.Sprintf("https://%d.example/feed", i), Kind: "rss"}
	}
	items[10].Subscribed = true // last item, lands on page 2; verifies subscribed marking survives pagination.
	inspector := &fakePersonInspector{records: discoverperson.Records{Reads: items}}

	h := ProfileSegmentHandler(resolver, &fakeDiscoverSubsReader{}, inspector, noShareMetadata())

	req1 := withSession(requestWithPathValues(http.MethodGet, "/api/profile/"+profileTargetDID+"/reads", map[string]string{"id": profileTargetDID, "segment": "reads"}), profileSelfDID, "sid")
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("page1 status = %d, want 200, body=%s", rec1.Code, rec1.Body.String())
	}
	var page1 struct {
		Items      []DiscoverPersonSourceWire `json:"items"`
		NextCursor string                     `json:"nextCursor"`
	}
	if err := json.NewDecoder(rec1.Body).Decode(&page1); err != nil {
		t.Fatalf("decode page1: %v", err)
	}
	if len(page1.Items) != 10 {
		t.Fatalf("page1 items = %d, want 10", len(page1.Items))
	}
	if page1.NextCursor == "" {
		t.Fatalf("page1 nextCursor is empty, want a cursor for page 2")
	}

	req2 := withSession(requestWithPathValues(http.MethodGet, "/api/profile/"+profileTargetDID+"/reads?cursor="+page1.NextCursor, map[string]string{"id": profileTargetDID, "segment": "reads"}), profileSelfDID, "sid")
	req2.URL.RawQuery = "cursor=" + page1.NextCursor
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("page2 status = %d, want 200, body=%s", rec2.Code, rec2.Body.String())
	}
	var page2 struct {
		Items      []DiscoverPersonSourceWire `json:"items"`
		NextCursor string                     `json:"nextCursor"`
	}
	if err := json.NewDecoder(rec2.Body).Decode(&page2); err != nil {
		t.Fatalf("decode page2: %v", err)
	}
	if len(page2.Items) != 1 {
		t.Fatalf("page2 items = %d, want 1", len(page2.Items))
	}
	if page2.NextCursor != "" {
		t.Errorf("page2 nextCursor = %q, want empty (exhausted)", page2.NextCursor)
	}
	if !page2.Items[0].Subscribed {
		t.Errorf("page2 item Subscribed = false, want true (subscribed marking must survive pagination)")
	}
}

func TestProfileSegmentHandler_Shares(t *testing.T) {
	resolver := newFakeAtIdentifierResolver()
	targetDID, _ := syntax.ParseDID(profileTargetDID)
	handle, _ := syntax.ParseHandle(profileTargetHdl)
	resolver.seed(profileTargetDID, targetDID, handle)

	inspector := &fakePersonInspector{records: discoverperson.Records{
		Shares: []discoverperson.ShareItem{{ItemURL: "https://shared.example/1", Comment: "nice"}},
	}}

	metadata := noShareMetadata()
	metadata.byKey["https://shared.example/1"] = sharemeta.Metadata{
		Title: "Resolved web article", TargetURL: "https://shared.example/final",
	}
	h := ProfileSegmentHandler(resolver, &fakeDiscoverSubsReader{}, inspector, metadata)
	req := withSession(requestWithPathValues(http.MethodGet, "/api/profile/"+profileTargetDID+"/shares", map[string]string{"id": profileTargetDID, "segment": "shares"}), profileSelfDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Items []DiscoverPersonShareWire `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ItemURL != "https://shared.example/1" || got.Items[0].Comment != "nice" {
		t.Errorf("Items = %+v", got.Items)
	}
	if got.Items[0].Title != "Resolved web article" || got.Items[0].TargetURL != "https://shared.example/final" {
		t.Errorf("share metadata = %+v", got.Items[0])
	}
}

func TestProfileSegmentHandler_NeverLeaksSaveKey(t *testing.T) {
	resolver := newFakeAtIdentifierResolver()
	targetDID, _ := syntax.ParseDID(profileTargetDID)
	handle, _ := syntax.ParseHandle(profileTargetHdl)
	resolver.seed(profileTargetDID, targetDID, handle)

	inspector := &fakePersonInspector{records: discoverperson.Records{
		Writes: []discoverperson.SourceItem{{Key: "https://a.example/feed", Subscribed: true}},
		Shares: []discoverperson.ShareItem{{ItemURL: "https://shared.example/1"}},
	}}

	h := ProfileSegmentHandler(resolver, &fakeDiscoverSubsReader{}, inspector, noShareMetadata())
	req := withSession(requestWithPathValues(http.MethodGet, "/api/profile/"+profileTargetDID+"/writes", map[string]string{"id": profileTargetDID, "segment": "writes"}), profileSelfDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if strings.Contains(strings.ToLower(rec.Body.String()), "save") {
		t.Fatalf("body leaks a save-related key: %s", rec.Body.String())
	}
}
