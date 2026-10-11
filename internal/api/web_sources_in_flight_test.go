package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"morgenblau/internal/websource"
)

func TestWebSourcesCreate_SetupInFlightHasItsOwnCode(t *testing.T) {
	rr := postWebSource(t, &fakeWebSources{err: websource.ErrSetupInFlight}, `{"url":"https://news.example.com/"}`)
	var env errorEnvelope
	_ = json.Unmarshal(rr.Body.Bytes(), &env)
	if rr.Code != http.StatusTooManyRequests || env.Code != "setup_in_flight" || env.Message == "" {
		t.Errorf("status = %d, body = %s; want 429 setup_in_flight", rr.Code, rr.Body)
	}
	if got := rr.Header().Get("Retry-After"); got != "10" {
		t.Errorf("Retry-After = %q, want 10", got)
	}
}
