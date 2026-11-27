package sync

import (
	"context"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"morgenblau/internal/session"

	"morgenblau/internal/database/db"
)

type PDSLister interface {
	ListSubscriptions(ctx context.Context, sess *session.Session) ([]PDSSubscription, error)
	ListSaves(ctx context.Context, sess *session.Session) ([]PDSSave, error)
	ListStandardSubscriptions(ctx context.Context, sess *session.Session) ([]PDSStandardSubscription, error)
}

type PDSStandardSubscription struct {
	URI         string
	Rkey        string
	Publication string
	CreatedAt   string
}

type PDSSubscription struct {
	URI         string
	Rkey        string
	Kind        string
	FeedURL     string
	SiteURL     string
	Publication string
	Title       string
	Primary     bool
	Tags        []string
}

type PDSSave struct {
	URI       string
	Rkey      string
	ItemURL   string
	FeedURL   string
	CreatedAt string
	Tags      []string
}

type SyncStore interface {
	ListUserSubscriptionsForSync(ctx context.Context, did string) ([]db.UserSubscription, error)
	UpsertUserSubscription(ctx context.Context, arg db.UpsertUserSubscriptionParams) error
	DeleteUserSubscription(ctx context.Context, arg db.DeleteUserSubscriptionParams) error
	UpsertFeed(ctx context.Context, arg db.UpsertFeedParams) error
	ListUserSavesForSync(ctx context.Context, did string) ([]db.UserSave, error)
	UpsertUserSave(ctx context.Context, arg db.UpsertUserSaveParams) error
	DeleteUserSave(ctx context.Context, arg db.DeleteUserSaveParams) error
}

type SessionResumer interface {
	ResumeSession(ctx context.Context, did syntax.DID, sessionID string) (*session.Session, error)
}

type SessionLocker interface {
	LockSession(did syntax.DID, sid string) func()
}
