package api

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/session"
)

// SessionRepo is the PDS surface the mutation routes share: commits plus the reads they preflight with.
type SessionRepo interface {
	atprepo.Writer
	atprepo.AtomicWriter
	atprepo.Lister
	atprepo.RecordGetter
}

var errCommitAlreadySpent = errors.New("PDS commit refused: a mutation request makes at most one")

type commitBudgetKey struct{}

// OneCommitPerRequest gives each request through next one PDS commit to spend through a CommitGate.
func OneCommitPerRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), commitBudgetKey{}, new(atomic.Bool))))
	})
}

// CommitGate refuses, before it reaches the PDS, any commit after a request's first and any commit outside OneCommitPerRequest.
type CommitGate struct {
	Repo SessionRepo
}

func spendCommit(ctx context.Context) error {
	spent, _ := ctx.Value(commitBudgetKey{}).(*atomic.Bool)
	if spent == nil || !spent.CompareAndSwap(false, true) {
		return errCommitAlreadySpent
	}
	return nil
}

func (g CommitGate) CreateRecord(ctx context.Context, sess *session.Session, collection syntax.NSID, record map[string]any) (*atprepo.RecordRef, error) {
	if err := spendCommit(ctx); err != nil {
		return nil, err
	}
	return g.Repo.CreateRecord(ctx, sess, collection, record)
}

func (g CommitGate) PutRecord(ctx context.Context, sess *session.Session, collection syntax.NSID, rkey string, record map[string]any) (*atprepo.RecordRef, error) {
	if err := spendCommit(ctx); err != nil {
		return nil, err
	}
	return g.Repo.PutRecord(ctx, sess, collection, rkey, record)
}

func (g CommitGate) DeleteRecord(ctx context.Context, sess *session.Session, collection syntax.NSID, rkey string) error {
	if err := spendCommit(ctx); err != nil {
		return err
	}
	return g.Repo.DeleteRecord(ctx, sess, collection, rkey)
}

func (g CommitGate) ApplyWrites(ctx context.Context, sess *session.Session, writes []atprepo.RecordWrite) ([]*atprepo.RecordRef, error) {
	if err := spendCommit(ctx); err != nil {
		return nil, err
	}
	return g.Repo.ApplyWrites(ctx, sess, writes)
}

func (g CommitGate) ListRecords(ctx context.Context, sess *session.Session, collection syntax.NSID) ([]atprepo.ListedRecord, error) {
	return g.Repo.ListRecords(ctx, sess, collection)
}

func (g CommitGate) GetRecord(ctx context.Context, sess *session.Session, collection syntax.NSID, rkey syntax.RecordKey) (*atprepo.ListedRecord, error) {
	return g.Repo.GetRecord(ctx, sess, collection, rkey)
}
