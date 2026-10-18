package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/cache/profiles"
	"morgenblau/internal/database/db"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/websource"
)

type fakeUsageReader struct {
	prefixes      db.CountSubscriptionsByReaderParams
	subscriptions []db.CountSubscriptionsByReaderRow
	activity      []db.ListAccountsLastActiveRow
	calls         int
}

func (f *fakeUsageReader) CountSubscriptionsByReader(_ context.Context, prefixes db.CountSubscriptionsByReaderParams) ([]db.CountSubscriptionsByReaderRow, error) {
	f.calls++
	f.prefixes = prefixes
	return f.subscriptions, nil
}

func (f *fakeUsageReader) ListAccountsLastActive(context.Context) ([]db.ListAccountsLastActiveRow, error) {
	f.calls++
	return f.activity, nil
}

type fakeNewsletterUsage map[string]int64

func (f fakeNewsletterUsage) CountActiveSourcesByOwner(context.Context) (map[string]int64, error) {
	return f, nil
}

func (f *fakeProfileSource) Lookup(ctx context.Context, did syntax.DID) (profiles.Profile, error) {
	return f.Get(ctx, did)
}

// hangingProfiles answers fast for one DID and, like an unresponsive PDS, holds every other lookup until the caller gives up.
type hangingProfiles struct{ fast syntax.DID }

func (h hangingProfiles) Lookup(ctx context.Context, did syntax.DID) (profiles.Profile, error) {
	if did == h.fast {
		return profiles.Profile{DID: did.String(), Handle: "fast.example"}, nil
	}
	<-ctx.Done()
	return profiles.Profile{}, ctx.Err()
}

func adminRequest(admin bool) *http.Request {
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/admin/usage", nil), "did:plc:admin", "sid")
	if admin {
		req = req.WithContext(auth.WithAdmin(req.Context()))
	}
	return req
}

func TestAdminUsage_RefusesRequestsTheGateDidNotAdmit(t *testing.T) {
	reader := &fakeUsageReader{}
	rr := httptest.NewRecorder()
	AdminUsageHandler(reader, nil, &fakeProfileSource{}, websource.FeedPrefixes{}).ServeHTTP(rr, adminRequest(false))
	if rr.Code != http.StatusNotFound || reader.calls != 0 {
		t.Fatalf("status = %d, reads = %d; an admin handler mounted outside the admin gate must read nothing", rr.Code, reader.calls)
	}
}

func TestAdminUsage_CountsEveryReaderBySourceType(t *testing.T) {
	reader := &fakeUsageReader{
		subscriptions: []db.CountSubscriptionsByReaderRow{
			{Did: "did:plc:beta", TotalCount: 1, GeneratedCount: 0},
			{Did: "did:plc:alpha", TotalCount: 3, GeneratedCount: 1},
		},
		activity: []db.ListAccountsLastActiveRow{
			{Did: "did:plc:gamma", LastActiveAt: "2026-10-01T08:00:00Z"},
			{Did: "did:plc:alpha", LastActiveAt: "2026-10-13T08:00:00Z"},
		},
	}
	newsletters := fakeNewsletterUsage{"did:plc:alpha": 2, "did:plc:delta": 1}
	src := &fakeProfileSource{profiles: map[syntax.DID]profiles.Profile{
		"did:plc:alpha": {DID: "did:plc:alpha", Handle: "alpha.example", DisplayName: sptr("Example Reader")},
	}}
	rr := httptest.NewRecorder()
	AdminUsageHandler(reader, newsletters, src, websource.FeedPrefixes{Current: "https://app.example.com/feed/", Legacy: "https://app.example.com/web/"}).ServeHTTP(rr, adminRequest(true))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if reader.prefixes != (db.CountSubscriptionsByReaderParams{WebFeedPrefix: "https://app.example.com/feed/", LegacyWebFeedPrefix: "https://app.example.com/web/"}) {
		t.Errorf("generated-feed prefixes = %+v, want this server's current and legacy ones", reader.prefixes)
	}
	var got adminUsageResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Totals != (adminUsageTotals{People: 4, Regular: 3, Generated: 1, Newsletter: 3}) {
		t.Errorf("totals = %+v", got.Totals)
	}
	wantOrder := []string{"did:plc:alpha", "did:plc:gamma", "did:plc:beta", "did:plc:delta"}
	if len(got.People) != len(wantOrder) {
		t.Fatalf("people = %+v, want %v", got.People, wantOrder)
	}
	for i, did := range wantOrder {
		if got.People[i].DID != did {
			t.Fatalf("people[%d] = %s, want %s; most recently active first, never-seen accounts last", i, got.People[i].DID, did)
		}
	}
	alpha := got.People[0]
	if alpha.Handle != "alpha.example" || alpha.DisplayName == nil || *alpha.DisplayName != "Example Reader" ||
		alpha.LastActiveAt == nil || *alpha.LastActiveAt != "2026-10-13T08:00:00Z" ||
		alpha.Regular != 2 || alpha.Generated != 1 || alpha.Newsletter != 2 {
		t.Errorf("alpha = %+v", alpha)
	}
	if delta := got.People[3]; delta.Handle != "" || delta.LastActiveAt != nil || delta.Newsletter != 1 || delta.Regular != 0 {
		t.Errorf("delta = %+v, want an unresolved newsletter-only reader listed by DID", delta)
	}
}

func TestAdminUsage_WithoutNewsletterDeliveryCountsNoNewsletters(t *testing.T) {
	reader := &fakeUsageReader{subscriptions: []db.CountSubscriptionsByReaderRow{{Did: "did:plc:alpha", TotalCount: 1}}}
	rr := httptest.NewRecorder()
	AdminUsageHandler(reader, nil, &fakeProfileSource{}, websource.FeedPrefixes{}).ServeHTTP(rr, adminRequest(true))
	var got adminUsageResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || rr.Code != http.StatusOK {
		t.Fatalf("status = %d, err = %v", rr.Code, err)
	}
	if got.Totals != (adminUsageTotals{People: 1, Regular: 1}) || got.People[0].DID != "did:plc:alpha" {
		t.Fatalf("got = %+v", got)
	}
}

func TestAdminUsage_UnresponsiveProfilesFallBackToDIDsWithinTheBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := &fakeUsageReader{}
		for i := range 40 {
			reader.subscriptions = append(reader.subscriptions, db.CountSubscriptionsByReaderRow{Did: fmt.Sprintf("did:plc:slow%02d", i), TotalCount: 1})
		}
		reader.subscriptions = append(reader.subscriptions, db.CountSubscriptionsByReaderRow{Did: "did:plc:fast", TotalCount: 1})
		start := time.Now()
		rr := httptest.NewRecorder()
		AdminUsageHandler(reader, nil, hangingProfiles{fast: "did:plc:fast"}, websource.FeedPrefixes{}).ServeHTTP(rr, adminRequest(true))
		if waited := time.Since(start); waited > adminProfileBudget {
			t.Fatalf("responded after %v, want within the %v profile budget; the server write timeout would cut a slower page off", waited, adminProfileBudget)
		}
		var got adminUsageResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || rr.Code != http.StatusOK {
			t.Fatalf("status = %d, err = %v", rr.Code, err)
		}
		if got.Totals.People != 41 {
			t.Fatalf("people = %d, want every account listed even when its profile is unresolved", got.Totals.People)
		}
		for _, p := range got.People {
			if want := map[bool]string{true: "fast.example", false: ""}[p.DID == "did:plc:fast"]; p.Handle != want {
				t.Errorf("%s handle = %q, want %q", p.DID, p.Handle, want)
			}
		}
	})
}
