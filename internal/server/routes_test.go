package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
