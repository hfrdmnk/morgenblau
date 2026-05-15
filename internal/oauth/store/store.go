// Package store implements indigo's ClientAuthStore over SQLite.
//
// The data blobs are plaintext JSON for now; the migration carries a
// TODO(P1) marker for the AEAD-at-rest pass. The serializer is the
// single insertion point — wrapping it later is a no-schema-change op.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
)

// authRequestTTL bounds how long an in-flight authorization can sit before
// it's considered stale. AT Proto spec is 10 minutes.
const authRequestTTL = 10 * time.Minute

// Store is a SQLite-backed ClientAuthStore plus a per-(did, session_id)
// mutex registry for the refresh-race path. The mutex is *advisory*: callers
// must call LockSession around the GetSession → refresh → SaveSession cycle.
type Store struct {
	db *sql.DB
	q  *db.Queries

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

var _ oauth.ClientAuthStore = (*Store)(nil)

// New builds a Store. The underlying *sql.DB is borrowed, not owned —
// the caller is responsible for Close.
func New(database *sql.DB) *Store {
	return &Store{
		db:    database,
		q:     db.New(database),
		locks: make(map[string]*sync.Mutex),
	}
}

func (s *Store) sessionLockKey(did syntax.DID, sid string) string {
	return did.String() + "\x00" + sid
}

// LockSession returns an unlock function. Concurrent callers for the same
// (did, sid) serialize; concurrent callers for different keys run in parallel.
//
// Hold the lock across the whole GetSession → use → SaveSession cycle.
// Otherwise concurrent refreshes for the same session can clobber each other.
func (s *Store) LockSession(did syntax.DID, sid string) func() {
	key := s.sessionLockKey(did, sid)
	s.mu.Lock()
	m, ok := s.locks[key]
	if !ok {
		m = &sync.Mutex{}
		s.locks[key] = m
	}
	s.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// --- ClientAuthStore: sessions ---

func (s *Store) SaveSession(ctx context.Context, sess oauth.ClientSessionData) error {
	data, err := json.Marshal(&sess)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	return s.q.PutSession(ctx, db.PutSessionParams{
		Did:       sess.AccountDID.String(),
		SessionID: sess.SessionID,
		Data:      data,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *Store) GetSession(ctx context.Context, did syntax.DID, sid string) (*oauth.ClientSessionData, error) {
	raw, err := s.q.GetSession(ctx, db.GetSessionParams{
		Did:       did.String(),
		SessionID: sid,
	})
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	var out oauth.ClientSessionData
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &out, nil
}

func (s *Store) DeleteSession(ctx context.Context, did syntax.DID, sid string) error {
	return s.q.DeleteSession(ctx, db.DeleteSessionParams{
		Did:       did.String(),
		SessionID: sid,
	})
}

// --- ClientAuthStore: auth requests ---

func (s *Store) SaveAuthRequestInfo(ctx context.Context, info oauth.AuthRequestData) error {
	data, err := json.Marshal(&info)
	if err != nil {
		return fmt.Errorf("marshal auth request: %w", err)
	}
	now := time.Now().UTC()
	return s.q.PutAuthRequest(ctx, db.PutAuthRequestParams{
		State:     info.State,
		Data:      data,
		CreatedAt: now.Format(time.RFC3339Nano),
		ExpiresAt: now.Add(authRequestTTL).Format(time.RFC3339Nano),
	})
}

func (s *Store) GetAuthRequestInfo(ctx context.Context, state string) (*oauth.AuthRequestData, error) {
	row, err := s.q.GetAuthRequest(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("auth request not found: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if err == nil && time.Now().After(expiresAt) {
		return nil, fmt.Errorf("auth request expired")
	}
	var out oauth.AuthRequestData
	if err := json.Unmarshal(row.Data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal auth request: %w", err)
	}
	return &out, nil
}

func (s *Store) DeleteAuthRequestInfo(ctx context.Context, state string) error {
	return s.q.DeleteAuthRequest(ctx, state)
}

// GCExpired deletes oauth_auth_requests rows whose expires_at is in the past.
// Returns the number of rows deleted. Safe to call concurrently with reads.
func (s *Store) GCExpired(ctx context.Context) (int64, error) {
	return s.q.DeleteExpiredAuthRequests(ctx, time.Now().UTC().Format(time.RFC3339Nano))
}
