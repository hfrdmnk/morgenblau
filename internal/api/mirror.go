package api

import (
	"context"
	"log/slog"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

// RepairDispatcher kicks a sync_user reconcile for one user; handlers reach for it when a local mirror write diverges from the PDS.
type RepairDispatcher interface {
	StartManualRefresh(ctx context.Context, did syntax.DID, sessionID string) (string, error)
}

// mirrorOrRepair runs a local mirror write and never propagates its error: the PDS write it follows already succeeded, so
// the response is committed and the only remedy left is a reconcile from the PDS, which is the source of truth.
func mirrorOrRepair(ctx context.Context, disp RepairDispatcher, sess *oauth.ClientSession, op string, write func() error) {
	err := write()
	if err == nil {
		return
	}
	did := sess.Data.AccountDID
	slog.Error("mirror write failed; dispatching sync_user to reconcile from PDS", "op", op, "did", did, "err", err)
	if _, derr := disp.StartManualRefresh(ctx, did, sess.Data.SessionID); derr != nil {
		slog.Warn("mirror repair dispatch failed; local index stays stale until the next scheduled sync", "op", op, "did", did, "err", derr)
	}
}
