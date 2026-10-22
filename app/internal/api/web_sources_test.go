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
	"time"

	"morgenblau/internal/websource"
)

type fakeWebSources struct {
	preview  websource.Preview
	err      error
	calls    []string
	onCreate func(ctx context.Context)
}

func (f *fakeWebSources) Create(ctx context.Context, did, listingURL, anchorURL string) (websource.Preview, error) {
	f.calls = append(f.calls, did+"|"+listingURL+"|"+anchorURL)
	if f.onCreate != nil {
		f.onCreate(ctx)
	}
	return f.preview, f.err
}

// deadlineRecorder is a ResponseWriter that lets http.ResponseController move its write deadline, as the server's does.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error {
	d.deadline = t
	return nil
}

func TestWebSourcesCreate_SetupOutlivesTheRequestAndTheServerWriteTimeout(t *testing.T) {
	reqCtx, closeDialog := context.WithCancel(context.Background())
	defer closeDialog()
	var setupCtxErr error
	svc := &fakeWebSources{preview: websource.Preview{Source: websource.Source{ID: "abc", Health: websource.HealthOK}}, onCreate: func(ctx context.Context) {
		closeDialog()
		setupCtxErr = ctx.Err()
	}}
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/web-sources", strings.NewReader(`{"url":"https://news.example.com/"}`)).WithContext(reqCtx), "did:plc:reader", "sid-1")
	rr := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	start := time.Now()
	WebSourcesCreateHandler(svc).ServeHTTP(rr, req)
	if setupCtxErr != nil {
		t.Errorf("closing the dialog cancelled a paid setup: %v", setupCtxErr)
	}
	if rr.deadline.Before(start.Add(websource.SetupTimeout)) {
		t.Errorf("write deadline %v after the request, want past the %v setup deadline", rr.deadline.Sub(start), websource.SetupTimeout)
	}
}

func postWebSource(t *testing.T, svc WebSourceCreator, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/web-sources", strings.NewReader(body)), "did:plc:reader", "sid-1")
	rr := httptest.NewRecorder()
	WebSourcesCreateHandler(svc).ServeHTTP(rr, req)
	return rr
}

func TestWebSourcesCreate_ReturnsThePreview(t *testing.T) {
	svc := &fakeWebSources{preview: websource.Preview{
		Source: websource.Source{
			ID: "abc", FeedURL: "https://app.example.com/feed/abc", Title: "Example Publication", SiteURL: "https://news.example.com/",
			Health: websource.HealthNeedsAttention, HealthReason: "only two posts found", NeedsAnchor: true,
		},
		Items: []websource.Item{
			{URL: "https://news.example.com/posts/b", Title: "B", Date: time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC), Excerpt: "not sent"},
			{URL: "https://news.example.com/posts/a", Title: "A"},
		},
		ItemCount: 14,
	}}
	rr := postWebSource(t, svc, `{"url":" https://news.example.com/ ","anchorUrl":"https://news.example.com/posts/a"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body)
	}
	if want := "did:plc:reader|https://news.example.com/|https://news.example.com/posts/a"; len(svc.calls) != 1 || svc.calls[0] != want {
		t.Errorf("calls = %v, want %q", svc.calls, want)
	}
	want := `{"id":"abc","feedUrl":"https://app.example.com/feed/abc","title":"Example Publication","siteUrl":"https://news.example.com/",` +
		`"health":"needs_attention","healthReason":"only two posts found","needsAnchor":true,` +
		`"items":[{"url":"https://news.example.com/posts/b","title":"B","date":"2026-10-08T08:00:00Z"},{"url":"https://news.example.com/posts/a","title":"A","date":null}],` +
		`"itemCount":14}`
	if got := strings.TrimSpace(rr.Body.String()); got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestWebSourcesCreate_ARejectedAnchorAnswersTheUnchangedPreview(t *testing.T) {
	for _, why := range []websource.AnchorRejection{websource.AnchorNotOnPage, websource.AnchorNotConfirmed, websource.AnchorNoBetter} {
		svc := &fakeWebSources{preview: websource.Preview{
			Source:         websource.Source{ID: "abc", Health: websource.HealthNeedsAttention, NeedsAnchor: true},
			Items:          []websource.Item{{URL: "https://news.example.com/posts/a", Title: "A"}},
			ItemCount:      1,
			AnchorRejected: why,
		}}
		rr := postWebSource(t, svc, `{"url":"https://news.example.com/","anchorUrl":"https://news.example.com/related/a"}`)
		var body struct {
			Health         string `json:"health"`
			NeedsAnchor    bool   `json:"needsAnchor"`
			ItemCount      int    `json:"itemCount"`
			AnchorRejected string `json:"anchorRejected"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if rr.Code != http.StatusOK || body.AnchorRejected != string(why) || body.Health != "needs_attention" || !body.NeedsAnchor || body.ItemCount != 1 {
			t.Errorf("%s: status = %d, body = %s", why, rr.Code, rr.Body)
		}
	}
}

func TestWebSourcesCreate_ASiteThatBlocksUsIsNotAnError(t *testing.T) {
	svc := &fakeWebSources{preview: websource.Preview{Source: websource.Source{ID: "abc", Health: websource.HealthBlockedBySite}, Items: []websource.Item{}}}
	rr := postWebSource(t, svc, `{"url":"https://news.example.com/"}`)
	var body struct {
		Health string            `json:"health"`
		Items  []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rr.Code != http.StatusOK || body.Health != "blocked_by_site" || body.Items == nil {
		t.Errorf("status = %d, body = %s", rr.Code, rr.Body)
	}
}

func TestWebSourcesCreate_TooManyGenerationsIs429(t *testing.T) {
	rr := postWebSource(t, &fakeWebSources{err: &websource.RateLimitError{RetryAfter: 90*time.Second + time.Millisecond}}, `{"url":"https://news.example.com/"}`)
	var env errorEnvelope
	_ = json.Unmarshal(rr.Body.Bytes(), &env)
	if rr.Code != http.StatusTooManyRequests || env.Code != codeRateLimited || env.Message == "" {
		t.Errorf("status = %d, body = %s", rr.Code, rr.Body)
	}
	if got := rr.Header().Get("Retry-After"); got != "91" {
		t.Errorf("Retry-After = %q, want whole seconds rounded up", got)
	}
}

func TestWebSourcesCreate_Errors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		err        error
		status     int
		code       string
	}{
		{"malformed json", `{`, nil, http.StatusBadRequest, codeInvalidRequest},
		{"missing url", `{"url":"  "}`, nil, http.StatusBadRequest, codeInvalidRequest},
		{"invalid url", `{"url":"ftp://news.example.com/"}`, fmt.Errorf("%w: ftp", websource.ErrInvalidURL), http.StatusBadRequest, codeInvalidRequest},
		{"generation failed", `{"url":"https://news.example.com/"}`, fmt.Errorf("%w: refused", websource.ErrGeneration), http.StatusBadGateway, codeUpstreamError},
		{"not configured", `{"url":"https://news.example.com/"}`, websource.ErrUnavailable, http.StatusServiceUnavailable, codeUnavailable},
		{"every setup slot busy", `{"url":"https://news.example.com/"}`, websource.ErrBusy, http.StatusServiceUnavailable, codeUnavailable},
		{"this reader's setup still running", `{"url":"https://news.example.com/"}`, websource.ErrSetupInFlight, http.StatusTooManyRequests, codeSetupInFlight},
		{"storage failure", `{"url":"https://news.example.com/"}`, errors.New("disk full"), http.StatusInternalServerError, codeInternalError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := postWebSource(t, &fakeWebSources{err: tc.err}, tc.body)
			var env errorEnvelope
			_ = json.Unmarshal(rr.Body.Bytes(), &env)
			if rr.Code != tc.status || env.Code != tc.code || env.Message == "" {
				t.Errorf("status = %d, body = %s; want %d %s", rr.Code, rr.Body, tc.status, tc.code)
			}
		})
	}
}

func TestWebSourcesCreate_UnreachableSiteSaysSoRatherThanBlocked(t *testing.T) {
	err := fmt.Errorf("%w: %w: dial tcp: connection refused", websource.ErrGeneration, websource.ErrUnreachable)
	rr := postWebSource(t, &fakeWebSources{err: err}, `{"url":"https://news.example.com/"}`)
	var env errorEnvelope
	_ = json.Unmarshal(rr.Body.Bytes(), &env)
	if rr.Code != http.StatusBadGateway || env.Code != codeUpstreamError || env.Message != "Couldn't reach that site" {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
	}
}

func TestWebSourcesCreate_BrowserBuiltSiteSaysWhyWithoutARetry(t *testing.T) {
	err := fmt.Errorf("%w: %w", websource.ErrGeneration, websource.ErrBrowserBuilt)
	rr := postWebSource(t, &fakeWebSources{err: err}, `{"url":"https://app.example.com/"}`)
	var env errorEnvelope
	_ = json.Unmarshal(rr.Body.Bytes(), &env)
	if rr.Code != http.StatusUnprocessableEntity || env.Code != codeBrowserBuilt ||
		env.Message != "This site builds its post list in the browser, so Morgenblau can't follow it yet." {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
	}
}
