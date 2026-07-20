package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"morgenblau/internal/personsearch"
)

const searchSessionDID = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"

type fakeSearcher struct {
	results []personsearch.Result
	err     error
	gotQ    string
}

func (f *fakeSearcher) Search(_ context.Context, q string) ([]personsearch.Result, error) {
	f.gotQ = q
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func TestSearchPeopleHandler_MissingQ_400(t *testing.T) {
	h := SearchPeopleHandler(&fakeSearcher{})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/search/people", nil), searchSessionDID, "sid")
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

func TestSearchPeopleHandler_EmptyQ_400(t *testing.T) {
	h := SearchPeopleHandler(&fakeSearcher{})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/search/people?q=", nil), searchSessionDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSearchPeopleHandler_SearcherError_502(t *testing.T) {
	h := SearchPeopleHandler(&fakeSearcher{err: errors.New("appview unreachable")})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/search/people?q=alice", nil), searchSessionDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	var body errorEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != codeUpstreamError {
		t.Errorf("code = %q, want %q", body.Code, codeUpstreamError)
	}
}

func TestSearchPeopleHandler_Success_200(t *testing.T) {
	f := &fakeSearcher{results: []personsearch.Result{
		{
			DID:             "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb",
			Handle:          "bob.example",
			DisplayName:     "Bob",
			Avatar:          "https://cdn.example/bob.png",
			InReaderNetwork: true,
			TasteHint:       []string{"Example Weekly"},
		},
	}}
	h := SearchPeopleHandler(f)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/search/people?q=bob", nil), searchSessionDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if f.gotQ != "bob" {
		t.Errorf("gotQ = %q, want %q", f.gotQ, "bob")
	}

	var got []SearchPersonWire
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	want := SearchPersonWire{
		DID:             "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb",
		Handle:          "bob.example",
		DisplayName:     "Bob",
		Avatar:          "https://cdn.example/bob.png",
		InReaderNetwork: true,
		TasteHint:       []string{"Example Weekly"},
	}
	if got[0].DID != want.DID || got[0].Handle != want.Handle || got[0].DisplayName != want.DisplayName ||
		got[0].Avatar != want.Avatar || got[0].InReaderNetwork != want.InReaderNetwork ||
		len(got[0].TasteHint) != 1 || got[0].TasteHint[0] != want.TasteHint[0] {
		t.Errorf("got = %+v, want %+v", got[0], want)
	}
}

func TestSearchPeopleHandler_EmptyResults_EncodesEmptyArrayNotNull(t *testing.T) {
	h := SearchPeopleHandler(&fakeSearcher{results: nil})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/search/people?q=nobody", nil), searchSessionDID, "sid")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if got := body[:1]; got == "n" {
		t.Fatalf("body = %q, want a JSON array, not null", body)
	}
	var got []SearchPersonWire
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got == nil {
		t.Errorf("decoded slice is nil, want non-nil empty slice (raw body: %s)", body)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}
