package handler

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/oauth/cookie"
)

// fakeApp implements ClientApp for handler tests — no network, no AS.
type fakeApp struct {
	startURL    string
	startErr    error
	startedWith string

	callbackSession *oauth.ClientSessionData
	callbackErr     error
	callbackParams  url.Values

	logoutDID syntax.DID
	logoutSID string
	logoutErr error
}

func (f *fakeApp) StartAuthFlow(_ context.Context, identifier string) (string, error) {
	f.startedWith = identifier
	return f.startURL, f.startErr
}

func (f *fakeApp) ProcessCallback(_ context.Context, params url.Values) (*oauth.ClientSessionData, error) {
	f.callbackParams = params
	return f.callbackSession, f.callbackErr
}

func (f *fakeApp) Logout(_ context.Context, did syntax.DID, sid string) error {
	f.logoutDID = did
	f.logoutSID = sid
	return f.logoutErr
}

func newSealer(t *testing.T) *cookie.Sealer {
	t.Helper()
	key := make([]byte, 32)
	rand.Read(key)
	s, err := cookie.New(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLogin_HappyPath_RedirectsToAS(t *testing.T) {
	app := &fakeApp{startURL: "https://pds.example.com/oauth/authorize?request_uri=urn:foo"}
	h := LoginHandler(app)

	form := url.Values{"handle": {"alice.bsky.social"}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d", rr.Code)
	}
	if rr.Header().Get("Location") != app.startURL {
		t.Errorf("Location = %q", rr.Header().Get("Location"))
	}
	if app.startedWith != "alice.bsky.social" {
		t.Errorf("StartAuthFlow called with %q", app.startedWith)
	}
}

func TestLogin_MissingHandle_400(t *testing.T) {
	app := &fakeApp{}
	h := LoginHandler(app)
	form := url.Values{"handle": {""}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestLogin_StartAuthFlowError_400(t *testing.T) {
	app := &fakeApp{startErr: fmt.Errorf("bad handle")}
	h := LoginHandler(app)
	form := url.Values{"handle": {"unknown"}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestLogin_NonPOST_405(t *testing.T) {
	app := &fakeApp{}
	h := LoginHandler(app)
	req := httptest.NewRequest(http.MethodGet, "/oauth/login", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

func TestCallback_HappyPath_SetsCookie_RedirectsHome(t *testing.T) {
	did, _ := syntax.ParseDID("did:plc:abc123xyz")
	app := &fakeApp{callbackSession: &oauth.ClientSessionData{
		AccountDID: did,
		SessionID:  "state-1",
	}}
	sealer := newSealer(t)
	h := CallbackHandler(app, sealer, nil)

	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state=s&code=c&iss=https://as.example.com", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q", loc)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	// Round-trip the cookie through the sealer to confirm contents.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(cookies[0])
	gotDID, gotSID, ok := sealer.Get(req2)
	if !ok {
		t.Fatal("cookie unsealing failed")
	}
	if gotDID != did.String() {
		t.Errorf("did = %q, want %q", gotDID, did.String())
	}
	if gotSID != "state-1" {
		t.Errorf("sid = %q", gotSID)
	}
}

func TestCallback_ProcessError_400_NoCookie(t *testing.T) {
	app := &fakeApp{callbackErr: fmt.Errorf("invalid state")}
	sealer := newSealer(t)
	h := CallbackHandler(app, sealer, nil)

	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state=bad", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	if len(rr.Result().Cookies()) != 0 {
		t.Errorf("error path should not set cookie, got %d cookies", len(rr.Result().Cookies()))
	}
}

func TestLogout_ClearsCookie_RedirectsToRoot(t *testing.T) {
	app := &fakeApp{}
	sealer := newSealer(t)
	h := LogoutHandler(app, sealer)

	// pre-existing session cookie
	setRR := httptest.NewRecorder()
	sealer.Set(setRR, "did:plc:abc", "sid-1")
	cookies := setRR.Result().Cookies()

	req := httptest.NewRequest(http.MethodPost, "/oauth/logout", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q", loc)
	}
	if app.logoutDID.String() != "did:plc:abc" {
		t.Errorf("Logout called with did = %q", app.logoutDID)
	}
	if app.logoutSID != "sid-1" {
		t.Errorf("Logout called with sid = %q", app.logoutSID)
	}
	// Should write a clearing cookie (MaxAge < 0).
	outCookies := rr.Result().Cookies()
	if len(outCookies) != 1 {
		t.Fatalf("expected 1 cookie on logout, got %d", len(outCookies))
	}
	if outCookies[0].MaxAge >= 0 {
		t.Errorf("logout cookie MaxAge = %d, want < 0", outCookies[0].MaxAge)
	}
}

func TestLogout_NoCookie_StillRedirects(t *testing.T) {
	app := &fakeApp{}
	sealer := newSealer(t)
	h := LogoutHandler(app, sealer)

	req := httptest.NewRequest(http.MethodPost, "/oauth/logout", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d", rr.Code)
	}
	if app.logoutDID.String() != "" {
		t.Errorf("Logout shouldn't be called without a session, got did = %q", app.logoutDID)
	}
}

func TestLogout_NonPOST_405(t *testing.T) {
	app := &fakeApp{}
	sealer := newSealer(t)
	h := LogoutHandler(app, sealer)
	req := httptest.NewRequest(http.MethodGet, "/oauth/logout", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}
