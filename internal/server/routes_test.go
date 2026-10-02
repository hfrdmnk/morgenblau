package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"morgenblau/internal/newsletter"
	"morgenblau/internal/session"
)

func TestDevLoginRouteRequiresLocalOptIn(t *testing.T) {
	for _, env := range []string{"local", "production"} {
		for _, enabled := range []bool{false, true} {
			t.Setenv("APP_ENV", env)
			var cfg *session.DevConfig
			if enabled {
				cfg = &session.DevConfig{}
			}
			srv := &Server{sessions: session.NewManager(nil, cfg, nil)}
			for _, method := range []string{"GET", "POST"} {
				if env == "local" && enabled && method == "POST" {
					continue
				}
				rr := httptest.NewRecorder()
				srv.RegisterRoutes().ServeHTTP(rr, httptest.NewRequest(method, "/dev/login", nil))
				want := 404
				if env == "local" && enabled {
					want = 200
				}
				if rr.Code != want {
					t.Fatalf("env=%s enabled=%v method=%s: got %d want %d", env, enabled, method, rr.Code, want)
				}
			}
		}
	}
}

func TestSignInRoutesAreRateLimitedPerClient(t *testing.T) {
	h := (&Server{}).RegisterRoutes()
	for i := range 10 {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/oauth/login", nil))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("login %d: status = %d, want the handler's 400 for a missing handle", i+1, rr.Code)
		}
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/oauth/callback", nil))
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("callback after 10 logins: status = %d, want 429; login and callback share one budget because both make anyone's request fan out to remote identity lookups", rr.Code)
	}
	other := httptest.NewRequest(http.MethodPost, "/oauth/login", nil)
	other.RemoteAddr = "198.51.100.7:1234"
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, other)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("another client: status = %d, want 400; the budget is per client", rr.Code)
	}
}

func TestDevRoutesArePublicInLocalEnvironment(t *testing.T) {
	vite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("styleguide"))
	}))
	t.Cleanup(vite.Close)
	t.Setenv("APP_ENV", "local")
	t.Setenv("VITE_URL", vite.URL)

	rr := httptest.NewRecorder()
	(&Server{}).RegisterRoutes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/dev/styleguide", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if rr.Body.String() != "styleguide" {
		t.Fatalf("body = %q, want styleguide", rr.Body.String())
	}
}

func TestDevRoutesAreNotServedOutsideLocalEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "production")

	rr := httptest.NewRecorder()
	(&Server{}).RegisterRoutes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/dev/styleguide", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestRemovedAPIRoutesReturnNotFound(t *testing.T) {
	routes := (&Server{}).routes()
	paths := []string{
		"/api/discover/sources",
		"/api/discover/people",
		"/api/search/people",
		"/api/profile/did:plc:example",
		"/api/profiles/did:plc:example",
		"/api/shares",
		"/api/follows",
		"/api/library/network-shares",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			routes.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
			if rr.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rr.Code)
			}
		})
	}
}

func TestJobsLatestRouteIsRegistered(t *testing.T) {
	rr := httptest.NewRecorder()
	(&Server{}).routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/jobs/latest", nil))
	if rr.Code == http.StatusNotFound || rr.Code == http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/jobs/latest is not registered: status = %d", rr.Code)
	}
}

func TestNewsletterRoutesAreRegistered(t *testing.T) {
	routes := (&Server{newsletters: &newsletter.Service{}}).routes()
	requests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/newsletters/address"},
		{http.MethodPost, "/api/newsletters/address"},
		{http.MethodGet, "/api/newsletters"},
		{http.MethodGet, "/api/newsletters/source-1"},
		{http.MethodPatch, "/api/newsletters/source-1"},
		{http.MethodPost, "/api/newsletters/source-1/stop"},
		{http.MethodPost, "/api/newsletters/source-1/enable"},
		{http.MethodGet, "/api/newsletters/source-1/entries"},
		{http.MethodPost, "/api/newsletters/messages/message-1/images"},
		{http.MethodPost, "/api/newsletters/messages/message-1/move"},
		{http.MethodGet, "/api/newsletter-assets/token-1"},
		{http.MethodPost, "/api/newsletter-saves"},
		{http.MethodDelete, "/api/newsletter-saves/save-1"},
	}
	for _, request := range requests {
		t.Run(request.method+" "+request.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			routes.ServeHTTP(rr, httptest.NewRequest(request.method, request.path, nil))
			if rr.Code == http.StatusNotFound || rr.Code == http.StatusMethodNotAllowed {
				t.Fatalf("route not registered: status = %d", rr.Code)
			}
		})
	}
}

// The frontend reads every /api/ failure as {code, message}; ServeMux's own 404 and 405 bodies are plain text.
func TestUnroutedAPIRequestsAnswerTheJSONErrorEnvelope(t *testing.T) {
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil),
		httptest.NewRequest(http.MethodPut, "/api/digest", nil),
	} {
		rr := httptest.NewRecorder()
		(&Server{}).routes().ServeHTTP(rr, req)

		var body struct{ Code, Message string }
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || body.Code == "" || body.Message == "" {
			t.Errorf("%s %s = %d %q, want a JSON {code, message} body", req.Method, req.URL.Path, rr.Code, rr.Body.String())
		}
		if got := rr.Header().Get("Content-Type"); got != "application/json" {
			t.Errorf("%s %s Content-Type = %q, want application/json", req.Method, req.URL.Path, got)
		}
	}
}
