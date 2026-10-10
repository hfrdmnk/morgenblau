package webfeed

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// failingDoer fails every request for one path at the transport and passes the rest to the test site.
type failingDoer struct {
	path string
	next Doer
}

func (d failingDoer) Do(req *http.Request) (*http.Response, error) {
	if req.URL.Path == d.path {
		return nil, errors.New("dial tcp: connection refused")
	}
	return d.next.Do(req)
}

func TestSetupTellsAnUnreachableSiteFromOneThatBlocksUs(t *testing.T) {
	cases := []struct {
		name        string
		prepare     func(*testSite) Options
		unreachable bool
	}{
		{name: "network error", unreachable: true, prepare: func(s *testSite) Options {
			return Options{Client: failingDoer{path: "/news", next: s.srv.Client()}}
		}},
		{name: "503", unreachable: true, prepare: func(s *testSite) Options { s.status["/news"] = 503; return s.opts() }},
		{name: "502", unreachable: true, prepare: func(s *testSite) Options { s.status["/news"] = 502; return s.opts() }},
		{name: "401", prepare: func(s *testSite) Options { s.status["/news"] = 401; return s.opts() }},
		{name: "403", prepare: func(s *testSite) Options { s.status["/news"] = 403; return s.opts() }},
		{name: "429", prepare: func(s *testSite) Options { s.status["/news"] = 429; return s.opts() }},
		{name: "challenge", prepare: func(s *testSite) Options {
			s.set("/news", `<html><head><title>Just a moment...</title></head></html>`)
			return s.opts()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestSite(t)
			opts := tc.prepare(s)
			res, err := Setup(context.Background(), s.url("/news"), opts)
			s.mu.Lock()
			sitemapHits := s.hits["/sitemap.xml"]
			s.mu.Unlock()
			if tc.unreachable {
				if !errors.Is(err, ErrUnreachable) {
					t.Fatalf("err = %v, want ErrUnreachable", err)
				}
				if sitemapHits != 0 {
					t.Fatal("an unreachable site is not a block; the sitemap fallback must not run")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !res.Blocked || sitemapHits != 1 {
				t.Fatalf("reasons %v, sitemap hits %d; a refusal falls back to the sitemap", res.Reasons, sitemapHits)
			}
		})
	}
}

func TestPollTellsAnUnreachableSiteFromOneThatBlocksUs(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		fail    bool
		reason  Reason
		blocked bool
	}{
		{name: "network error", fail: true, reason: ReasonUnreachable},
		{name: "503", status: 503, reason: ReasonHTTPStatus},
		{name: "500", status: 500, reason: ReasonHTTPStatus},
		{name: "401", status: 401, reason: ReasonHTTPStatus, blocked: true},
		{name: "403", status: 403, reason: ReasonHTTPStatus, blocked: true},
		{name: "429", status: 429, reason: ReasonHTTPStatus, blocked: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestSite(t)
			s.addBlog(8)
			res := setupRecipe(t, s, "/blog")
			opts := s.opts()
			if tc.fail {
				opts.Client = failingDoer{path: "/blog", next: s.srv.Client()}
			}
			if tc.status != 0 {
				s.mu.Lock()
				s.status["/blog"] = tc.status
				s.mu.Unlock()
			}
			got, err := Poll(context.Background(), PollInput{Recipe: res.Recipe, History: historyFrom(res)}, opts)
			if err != nil {
				t.Fatal(err)
			}
			if got.Verdict != PollBroken || got.Reason != tc.reason || got.Blocked != tc.blocked {
				t.Fatalf("verdict %s reason %q blocked %v, want broken %q %v", got.Verdict, got.Reason, got.Blocked, tc.reason, tc.blocked)
			}
		})
	}
}
