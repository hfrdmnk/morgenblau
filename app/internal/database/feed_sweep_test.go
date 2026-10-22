package database

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"morgenblau/internal/database/db"
)

const sweepWebPrefix = "https://app.example.com/web/"

type sweepFixture struct {
	t     *testing.T
	ctx   context.Context
	conn  *sql.DB
	q     *db.Queries
	start time.Time
}

func newSweepFixture(t *testing.T) *sweepFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sweep.db")
	runGoose(t, path, "up")
	conn := openMigrationDB(t, path)
	return &sweepFixture{
		t: t, ctx: context.Background(), conn: conn, q: db.New(conn),
		start: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
	}
}

func (f *sweepFixture) stamp(at time.Time) string { return at.UTC().Format(time.RFC3339) }

func (f *sweepFixture) feed(url string) {
	f.t.Helper()
	now := f.stamp(f.start)
	if err := f.q.UpsertFeed(f.ctx, db.UpsertFeedParams{FeedUrl: url, CreatedAt: now, UpdatedAt: now}); err != nil {
		f.t.Fatal(err)
	}
	if err := f.q.UpsertFeedEntry(f.ctx, db.UpsertFeedEntryParams{
		FeedUrl: url, Guid: url + "#post", EntrySlug: url + "#post", Url: url + "/post",
		ContentType: "html", PublishedAt: now, FetchedAt: now,
	}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *sweepFixture) subscribe(did, rkey, url string) {
	f.t.Helper()
	now := f.stamp(f.start)
	// Every subscribe path, mirror, reconcile and import alike, upserts the catalog row before the subscription.
	if err := f.q.UpsertFeed(f.ctx, db.UpsertFeedParams{FeedUrl: url, CreatedAt: now, UpdatedAt: now}); err != nil {
		f.t.Fatal(err)
	}
	f.follow(did, rkey, url)
}

// follow writes only the subscription row, the second half of a subscribe.
func (f *sweepFixture) follow(did, rkey, url string) {
	f.t.Helper()
	now := f.stamp(f.start)
	if err := f.q.UpsertUserSubscription(f.ctx, db.UpsertUserSubscriptionParams{
		Did: did, Rkey: rkey, FeedUrl: url,
		AtUri: "at://" + did + "/blue.morgen.feed.subscription/" + rkey, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *sweepFixture) unsubscribe(did, rkey string) {
	f.t.Helper()
	if err := f.q.DeleteUserSubscription(f.ctx, db.DeleteUserSubscriptionParams{Did: did, Rkey: rkey}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *sweepFixture) sweep(at time.Time) SweepResult {
	f.t.Helper()
	res, err := SweepUnfollowedFeeds(f.ctx, f.conn, at)
	if err != nil {
		f.t.Fatal(err)
	}
	return res
}

func (f *sweepFixture) exists(url string) bool {
	f.t.Helper()
	_, err := f.q.GetFeed(f.ctx, url)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		f.t.Fatal(err)
	}
	return true
}

func (f *sweepFixture) entries(url string) int {
	f.t.Helper()
	var n int
	if err := f.conn.QueryRow(`SELECT COUNT(*) FROM feed_entries WHERE feed_url = ?`, url).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *sweepFixture) followed() []string {
	f.t.Helper()
	urls, err := f.q.ListFollowedFeedURLs(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return urls
}

func TestSweepDeletesAFeedOnlyAfterItWentUnfollowedForTheWholeGrace(t *testing.T) {
	f := newSweepFixture(t)
	const kept, dropped = "https://kept.example.com/feed", "https://dropped.example.com/feed"
	f.feed(kept)
	f.feed(dropped)
	f.subscribe("did:plc:alice", "a1", kept)
	f.subscribe("did:plc:bob", "b1", kept)
	f.subscribe("did:plc:alice", "a2", dropped)
	f.unsubscribe("did:plc:alice", "a2")
	// One of two followers leaving must not orphan the feed.
	f.unsubscribe("did:plc:bob", "b1")

	marked := f.start.Add(time.Hour)
	f.sweep(marked)
	if got := f.followed(); !slices.Equal(got, []string{kept}) {
		t.Fatalf("followed after unsubscribe = %v, want only %s: an unfollowed feed must stop being refreshed", got, kept)
	}

	f.sweep(marked.Add(FeedSweepGrace - time.Second))
	if !f.exists(dropped) || f.entries(dropped) != 1 {
		t.Fatal("feed was deleted before its grace ran out")
	}

	res := f.sweep(marked.Add(FeedSweepGrace))
	if f.exists(dropped) || f.entries(dropped) != 0 {
		t.Error("feed unfollowed for the whole grace survived the sweep with its entries")
	}
	if res.Feeds != 1 {
		t.Errorf("swept feeds = %d, want 1", res.Feeds)
	}
	if !f.exists(kept) || f.entries(kept) != 1 {
		t.Error("followed feed was swept")
	}
}

func TestSweepGraceRestartsWhenAFeedIsFollowedAgain(t *testing.T) {
	f := newSweepFixture(t)
	const url = "https://returning.example.com/feed"
	f.feed(url)
	f.sweep(f.start)
	if len(f.followed()) != 0 {
		t.Fatal("feed with no followers is still refreshed")
	}

	f.subscribe("did:plc:alice", "a1", url)
	if got := f.followed(); !slices.Equal(got, []string{url}) {
		t.Fatalf("followed after resubscribe = %v: refreshing must resume at once", got)
	}
	f.sweep(f.start.Add(FeedSweepGrace))
	f.unsubscribe("did:plc:alice", "a1")

	left := f.start.Add(FeedSweepGrace + time.Hour)
	f.sweep(left)
	f.sweep(left.Add(FeedSweepGrace - time.Second))
	if !f.exists(url) {
		t.Fatal("grace counted from the first time the feed went unfollowed, not the last")
	}
	f.sweep(left.Add(FeedSweepGrace))
	if f.exists(url) {
		t.Error("feed unfollowed again for the whole grace survived")
	}
}

func TestSweepCannotDeleteAFeedBetweenTheSubscribeMirrorsTwoWrites(t *testing.T) {
	f := newSweepFixture(t)
	const url = "https://raced.example.com/feed"
	f.feed(url)
	f.sweep(f.start)

	late := f.start.Add(3 * FeedSweepGrace)
	if err := f.q.UpsertFeed(f.ctx, db.UpsertFeedParams{FeedUrl: url, CreatedAt: f.stamp(late), UpdatedAt: f.stamp(late)}); err != nil {
		t.Fatal(err)
	}
	f.sweep(late)
	f.follow("did:plc:alice", "a1", url)
	if !f.exists(url) {
		t.Fatal("feed vanished under a subscribe in flight")
	}
	if got := f.followed(); !slices.Equal(got, []string{url}) {
		t.Errorf("followed = %v: a sweep between the subscribe's two writes must not keep the new follower's feed from refreshing", got)
	}
}

func TestSweepKeepsAFeedWhileOneOfItsEntriesIsSaved(t *testing.T) {
	f := newSweepFixture(t)
	const url = "https://saved.example.com/feed"
	f.feed(url)
	now := f.stamp(f.start)
	if err := f.q.UpsertUserSave(f.ctx, db.UpsertUserSaveParams{
		Did: "did:plc:alice", Rkey: "s1", AtUri: "at://did:plc:alice/blue.morgen.feed.save/s1",
		ItemUrl: url + "/post", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	f.sweep(f.start)
	f.sweep(f.start.Add(2 * FeedSweepGrace))
	if !f.exists(url) || f.entries(url) != 1 {
		t.Fatal("sweep deleted the entry a Library save shows its title from")
	}
	if len(f.followed()) != 0 {
		t.Error("a feed kept only for a save is still refreshed")
	}

	if err := f.q.DeleteUserSave(f.ctx, db.DeleteUserSaveParams{Did: "did:plc:alice", Rkey: "s1"}); err != nil {
		t.Fatal(err)
	}
	f.sweep(f.start.Add(2 * FeedSweepGrace))
	if f.exists(url) {
		t.Error("feed outlived the save that held it")
	}
}

func (f *sweepFixture) webSource(id string, at time.Time) {
	f.t.Helper()
	if err := f.q.CreateWebSource(f.ctx, db.CreateWebSourceParams{
		ID: id, ListingKey: id + ".example.com/", ListingUrl: "https://" + id + ".example.com/", SiteUrl: "https://" + id + ".example.com/",
		Title: "Example Publication", Recipe: "{}", Health: "ok", SetupCost: "{}", CreatedByDid: "did:plc:creator",
		CreatedAt: f.stamp(at), NextPollAt: ptrTo(f.stamp(at)),
	}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *sweepFixture) webSourceExists(id string) bool {
	f.t.Helper()
	_, err := f.q.GetWebSource(f.ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		f.t.Fatal(err)
	}
	return true
}

func ptrTo(s string) *string { return &s }

func TestSweepDeletesWebSourcesNobodyFollowsOrWasOfferedDuringTheGrace(t *testing.T) {
	f := newSweepFixture(t)
	f.webSource("followed", f.start)
	f.feed(sweepWebPrefix + "followed")
	f.subscribe("did:plc:alice", "a1", sweepWebPrefix+"followed")

	f.webSource("unfollowed", f.start)
	f.feed(sweepWebPrefix + "unfollowed")
	f.subscribe("did:plc:alice", "a2", sweepWebPrefix+"unfollowed")
	f.unsubscribe("did:plc:alice", "a2")

	f.webSource("previewed", f.start)
	f.webSource("reoffered", f.start)

	f.sweep(f.start)
	due, err := f.q.ListDueWebSources(f.ctx, db.ListDueWebSourcesParams{Now: ptrTo(f.stamp(f.start.Add(time.Hour))), FeedUrlPrefix: sweepWebPrefix, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != "followed" {
		t.Fatalf("due web sources = %v, want only the followed one: an unfollowed source must stop polling and spending", due)
	}

	offered := f.start.Add(FeedSweepGrace - time.Hour)
	if _, err := f.q.OfferWebSourceByListingKey(f.ctx, db.OfferWebSourceByListingKeyParams{ListingKey: "reoffered.example.com/", OfferedAt: f.stamp(offered)}); err != nil {
		t.Fatal(err)
	}

	res := f.sweep(f.start.Add(FeedSweepGrace))
	for id, want := range map[string]bool{"followed": true, "unfollowed": false, "previewed": false, "reoffered": true} {
		if got := f.webSourceExists(id); got != want {
			t.Errorf("web source %s exists = %v, want %v", id, got, want)
		}
	}
	if res.Feeds != 1 || res.WebSources != 2 {
		t.Errorf("swept = %+v, want 1 feed and 2 web sources", res)
	}
}

func TestSweepKeepsAWebSourceFollowedUnderEitherPathOrAnotherOrigin(t *testing.T) {
	f := newSweepFixture(t)
	const oldOrigin = "https://old-app.example.com/web/"
	f.webSource("moved", f.start)
	f.subscribe("did:plc:alice", "a1", oldOrigin+"moved")

	f.webSource("current", f.start)
	f.subscribe("did:plc:alice", "a2", "https://app.example.com/feed/current")

	f.sweep(f.start.Add(2 * FeedSweepGrace))
	if !f.webSourceExists("moved") {
		t.Error("a web source readers follow was swept because they subscribed under the server's previous origin")
	}
	if !f.webSourceExists("current") {
		t.Error("a web source readers follow at its current /feed/ path was swept")
	}
}
