package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"morgenblau/internal/newsletter"
)

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
