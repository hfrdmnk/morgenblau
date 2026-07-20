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
	ListShares(ctx context.Context, sess *oauth.ClientSession) ([]PDSShare, error)
	ListRecommends(ctx context.Context, sess *oauth.ClientSession) ([]PDSRecommend, error)
	ListFollows(ctx context.Context, sess *oauth.ClientSession) ([]PDSFollow, error)
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

// PDSShare is the trimmed shape of a blue.morgen.feed.share record: with a Document it's a site.standard.graph.recommend comment sidecar, otherwise a standalone rss share.
type PDSShare struct {
	URI       string
	Rkey      string
	ItemURL   string
	Document  string
	FeedURL   string
	Comment   string
	CreatedAt string
}

// PDSRecommend is the trimmed shape of a site.standard.graph.recommend record, the sole existence authority for a standardfeed share.
type PDSRecommend struct {
	URI       string
	Rkey      string
	Document  string
	CreatedAt string
}

// PDSFollow is the trimmed shape of a blue.morgen.graph.follow record.
type PDSFollow struct {
	URI        string
	Rkey       string
	SubjectDID string
	CreatedAt  string
}

// SyncStore is the slice of *db.Queries SyncUser depends on, kept narrow so the orchestrator's full surface stays hideable behind one interface.
type SyncStore interface {
	ListUserSubscriptionsForSync(ctx context.Context, did string) ([]db.ListUserSubscriptionsForSyncRow, error)
	UpsertUserSubscription(ctx context.Context, arg db.UpsertUserSubscriptionParams) error
	DeleteUserSubscription(ctx context.Context, arg db.DeleteUserSubscriptionParams) error
	UpsertFeed(ctx context.Context, arg db.UpsertFeedParams) error
	ListUserSavesForSync(ctx context.Context, did string) ([]db.ListUserSavesForSyncRow, error)
	UpsertUserSave(ctx context.Context, arg db.UpsertUserSaveParams) error
	DeleteUserSave(ctx context.Context, arg db.DeleteUserSaveParams) error
	ListUserSharesForSync(ctx context.Context, did string) ([]db.ListUserSharesForSyncRow, error)
	UpsertUserShare(ctx context.Context, arg db.UpsertUserShareParams) error
	DeleteUserShare(ctx context.Context, arg db.DeleteUserShareParams) error
	GetFeedEntryURLByGuid(ctx context.Context, guid string) (string, error)
	ListUserFollowsForSync(ctx context.Context, did string) ([]db.ListUserFollowsForSyncRow, error)
	UpsertUserFollow(ctx context.Context, arg db.UpsertUserFollowParams) error
	DeleteUserFollow(ctx context.Context, arg db.DeleteUserFollowParams) error
}

// SessionResumer hands SyncUser a session by (did, sessionID) rather than a request-bound one, since the login path may finish before the SyncUser goroutine starts.
type SessionResumer interface {
	ResumeSession(ctx context.Context, did syntax.DID, sessionID string) (*oauth.ClientSession, error)
}

// SessionLocker serialises a session's (did, sid) refresh cycle, the same contract the auth middleware uses; the engine holds it around resume+eager-refresh to avoid racing the request path.
type SessionLocker interface {
	LockSession(did syntax.DID, sid string) func()
}
