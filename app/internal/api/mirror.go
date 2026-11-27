package api

import (
	"context"
	"log/slog"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"morgenblau/internal/session"
)

type RepairDispatcher interface {
	StartManualRefresh(ctx context.Context, did syntax.DID, sessionID string) (string, error)
}

func commitThenMirror[T any](ctx context.Context, disp RepairDispatcher, sess *session.Session, op string, commitOrWriteError func() (T, bool), mirror func(T) error) (T, bool) {
	out, ok := commitOrWriteError()
	if !ok {
		return out, false
	}
	err := mirror(out)
	if err == nil {
		return out, true
	}
	did := sess.Data.AccountDID
	slog.Error("mirror write failed; dispatching sync_user to reconcile from PDS", "op", op, "did", did, "err", err)
	if _, derr := disp.StartManualRefresh(ctx, did, sess.Data.SessionID); derr != nil {
		slog.Warn("mirror repair dispatch failed; local index stays stale until the next scheduled sync", "op", op, "did", did, "err", derr)
	}
	return out, true
}
