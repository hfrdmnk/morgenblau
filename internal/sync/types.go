package sync

import (
	"context"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
)

// PDSLister snapshots a user's blue.morgen and site.standard.graph records from the PDS.
type PDSLister interface {
	ListSubscriptions(ctx context.Context, sess *oauth.ClientSession) ([]PDSSubscription, error)
	ListSaves(ctx context.Context, sess *oauth.ClientSession) ([]PDSSave, error)
	ListStandardSubscriptions(ctx context.Context, sess *oauth.ClientSession) ([]PDSStandardSubscription, error)
}

// PDSStandardSubscription is the trimmed shape of a site.standard.graph subscription record, the sole existence authority for publication sources; CreatedAt is optional.
type PDSStandardSubscription struct {
	URI         string
	Rkey        string
	Publication string
	CreatedAt   string
}

// PDSSubscription is the trimmed shape of a blue.morgen.feed.subscription record,
// dispatched on the `source` union: rssFeed carries FeedURL/SiteURL as its own source;
// standardPublication carries Publication as a site.standard.graph subscription sidecar.
type PDSSubscription struct {
	URI         string
	Rkey        string
	Kind        string // "rss" | "standardfeed"
	FeedURL     string // rssFeed variant
	SiteURL     string // rssFeed variant
	Publication string // standardPublication variant (at-uri)
	Title       string
	Primary     bool
	Tags        []string
}

// PDSSave is the trimmed shape of a blue.morgen.feed.save record; feedUrl is optional on the record.
type PDSSave struct {
	URI       string
	Rkey      string
	ItemURL   string
	FeedURL   string
	CreatedAt string
}

// SyncStore is the slice of *db.Queries SyncUser depends on, kept narrow so the orchestrator's full surface stays hideable behind one interface.
type SyncStore interface {
	ListUserSubscriptionsForSync(ctx context.Context, did string) ([]db.UserSubscription, error)
	UpsertUserSubscription(ctx context.Context, arg db.UpsertUserSubscriptionParams) error
	DeleteUserSubscription(ctx context.Context, arg db.DeleteUserSubscriptionParams) error
	UpsertFeed(ctx context.Context, arg db.UpsertFeedParams) error
	ListUserSavesForSync(ctx context.Context, did string) ([]db.UserSave, error)
	UpsertUserSave(ctx context.Context, arg db.UpsertUserSaveParams) error
	DeleteUserSave(ctx context.Context, arg db.DeleteUserSaveParams) error
}

// SessionResumer hands SyncUser a session by (did, sessionID) rather than a request-bound one, since the login path may finish before the SyncUser goroutine starts.
type SessionResumer interface {
	ResumeSession(ctx context.Context, did syntax.DID, sessionID string) (*oauth.ClientSession, error)
}

// SessionLocker serialises a session's (did, sid) refresh cycle, the same contract the auth middleware uses; the engine holds it around resume+eager-refresh to avoid racing the request path.
type SessionLocker interface {
	LockSession(did syntax.DID, sid string) func()
}
