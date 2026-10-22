package cookie

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
)

func randomKey(t *testing.T) []byte {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSetGet_RoundTrips(t *testing.T) {
	s, err := New(randomKey(t))
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	s.Set(rr, "did:plc:abc", "sid-1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rr.Result().Cookies() {
		req.AddCookie(c)
	}

	did, sid, ok := s.Get(req)
	if !ok {
		t.Fatal("Get reported no session")
	}
	if did != "did:plc:abc" {
		t.Errorf("did = %q", did)
	}
	if sid != "sid-1" {
		t.Errorf("sid = %q", sid)
	}
}

func TestGet_DifferentKey_FailsToUnseal(t *testing.T) {
	keyA := randomKey(t)
	keyB := randomKey(t)
	a, _ := New(keyA)
	b, _ := New(keyB)

	rr := httptest.NewRecorder()
	a.Set(rr, "did:plc:abc", "sid-1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rr.Result().Cookies() {
		req.AddCookie(c)
	}

	if _, _, ok := b.Get(req); ok {
		t.Errorf("cookie sealed with key A unsealed under key B (tamper not detected)")
	}
}

func TestGet_GarbageCookie_NoSession(t *testing.T) {
	s, _ := New(randomKey(t))
	cases := []string{"", "not-base64!@#$", "AAAA", "short"}
	for _, value := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: cookieName, Value: value})
		if _, _, ok := s.Get(req); ok {
			t.Errorf("Get reported a session for garbage value %q", value)
		}
	}
}

func TestGet_TamperedCiphertext_NoSession(t *testing.T) {
	s, _ := New(randomKey(t))
	rr := httptest.NewRecorder()
	s.Set(rr, "did:plc:abc", "sid-1")

	cookies := rr.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	// flip a character somewhere in the middle of the base64 payload
	val := []byte(cookies[0].Value)
	if val[10] == 'A' {
		val[10] = 'B'
	} else {
		val[10] = 'A'
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: string(val)})
	if _, _, ok := s.Get(req); ok {
		t.Errorf("tampered cookie unsealed")
	}
}

func TestGet_NoCookie_NoSession(t *testing.T) {
	s, _ := New(randomKey(t))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, _, ok := s.Get(req); ok {
		t.Errorf("Get reported a session when no cookie was sent")
	}
}

func TestClear_OverridesPrior(t *testing.T) {
	s, _ := New(randomKey(t))
	rr := httptest.NewRecorder()
	s.Clear(rr)
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.MaxAge >= 0 {
		t.Errorf("Clear should set MaxAge < 0, got %d", c.MaxAge)
	}
	if c.Value != "" {
		t.Errorf("Clear cookie should have empty value, got %q", c.Value)
	}
}

func TestSet_CookieAttributes(t *testing.T) {
	s, _ := New(randomKey(t))
	rr := httptest.NewRecorder()
	s.Set(rr, "did:plc:abc", "sid-1")
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if !c.HttpOnly {
		t.Error("cookie not HttpOnly")
	}
	if !c.Secure {
		t.Error("cookie not Secure")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("Path = %q", c.Path)
	}
	if c.MaxAge <= 0 {
		t.Errorf("MaxAge should be positive, got %d", c.MaxAge)
	}
}

func TestNew_RejectsShortKey(t *testing.T) {
	if _, err := New(make([]byte, 16)); err == nil {
		t.Error("New should reject a 16-byte key (must be 32 for AES-256)")
	}
}
