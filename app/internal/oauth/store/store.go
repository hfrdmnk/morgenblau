// Package store implements indigo's ClientAuthStore over SQLite.
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
	"morgenblau/internal/secret"
)

// authRequestTTL: AT Proto spec mandates a 10-minute window for in-flight authorizations.
const authRequestTTL = 10 * time.Minute

// Store is a SQLite-backed ClientAuthStore with a per-(did, session_id) mutex registry serializing session refresh.
type Store struct {
	db     *sql.DB
	q      *db.Queries
	keyset *secret.Keyset

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

var _ oauth.ClientAuthStore = (*Store)(nil)

// New builds a Store; the *sql.DB is borrowed (caller must Close it), and keyset is required.
func New(database *sql.DB, keyset *secret.Keyset) *Store {
	return &Store{
		db:     database,
		q:      db.New(database),
		keyset: keyset,
		locks:  make(map[string]*sync.Mutex),
	}
}

func (s *Store) sessionLockKey(did syntax.DID, sid string) string {
	return did.String() + "\x00" + sid
}

// LockSession serializes callers for the same (did, sid); different keys run in parallel.
// Hold it across GetSession -> use -> SaveSession, or concurrent refreshes can clobber each other.
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

func (s *Store) SaveSession(ctx context.Context, sess oauth.ClientSessionData) error {
	data, err := json.Marshal(&sess)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	sealed, err := s.keyset.Seal(data)
	if err != nil {
		return fmt.Errorf("seal session: %w", err)
	}
	return s.q.PutSession(ctx, db.PutSessionParams{
		Did:       sess.AccountDID.String(),
		SessionID: sess.SessionID,
		Data:      sealed,
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
	data, err := s.keyset.Open(raw)
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}
	var out oauth.ClientSessionData
	if err := json.Unmarshal(data, &out); err != nil {
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

func (s *Store) SaveAuthRequestInfo(ctx context.Context, info oauth.AuthRequestData) error {
	data, err := json.Marshal(&info)
	if err != nil {
		return fmt.Errorf("marshal auth request: %w", err)
	}
	sealed, err := s.keyset.Seal(data)
	if err != nil {
		return fmt.Errorf("seal auth request: %w", err)
	}
	now := time.Now().UTC()
	return s.q.PutAuthRequest(ctx, db.PutAuthRequestParams{
		State:     info.State,
		Data:      sealed,
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
	data, err := s.keyset.Open(row.Data)
	if err != nil {
		return nil, fmt.Errorf("open auth request: %w", err)
	}
	var out oauth.AuthRequestData
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal auth request: %w", err)
	}
	return &out, nil
}

func (s *Store) DeleteAuthRequestInfo(ctx context.Context, state string) error {
	return s.q.DeleteAuthRequest(ctx, state)
}

// GCExpired deletes expired oauth_auth_requests rows; safe to call concurrently with reads.
func (s *Store) GCExpired(ctx context.Context) (int64, error) {
	return s.q.DeleteExpiredAuthRequests(ctx, time.Now().UTC().Format(time.RFC3339Nano))
}
