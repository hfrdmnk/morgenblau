package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/secret"
)

// schemaSQL mirrors internal/database/migrations/*_oauth_tables.sql; inlined to keep the test hermetic.
const schemaSQL = `
CREATE TABLE oauth_sessions (
    did         TEXT NOT NULL,
    session_id  TEXT NOT NULL,
    data        BLOB NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (did, session_id)
);
CREATE TABLE oauth_auth_requests (
    state       TEXT PRIMARY KEY,
    data        BLOB NOT NULL,
    created_at  TEXT NOT NULL,
    expires_at  TEXT NOT NULL
);
CREATE INDEX oauth_auth_requests_expires_at_idx
    ON oauth_auth_requests (expires_at);
`

func newMemDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "store.db") + "?_pragma=foreign_keys(on)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.ExecContext(context.Background(), schemaSQL); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func testKeyset(t *testing.T) *secret.Keyset {
	t.Helper()
	key := make([]byte, secret.KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	ks, err := secret.NewKeyset(key)
	if err != nil {
		t.Fatal(err)
	}
	return ks
}

func newStore(t *testing.T) *Store {
	t.Helper()
	return New(newMemDB(t), testKeyset(t))
}

func sampleSession(did string, sid string) oauth.ClientSessionData {
	d, _ := syntax.ParseDID(did)
	return oauth.ClientSessionData{
		AccountDID:              d,
		SessionID:               sid,
		HostURL:                 "https://service.example.com",
		AuthServerURL:           "https://as.example.com",
		AuthServerTokenEndpoint: "https://as.example.com/oauth/token",
		Scopes:                  []string{"atproto"},
		AccessToken:             "access-" + sid,
		RefreshToken:            "refresh-" + sid,
		DPoPPrivateKeyMultibase: "zfake",
	}
}

func sampleAuthRequest(state string) oauth.AuthRequestData {
	return oauth.AuthRequestData{
		State:                   state,
		AuthServerURL:           "https://as.example.com",
		Scopes:                  []string{"atproto"},
		RequestURI:              "urn:ietf:params:oauth:request_uri:" + state,
		AuthServerTokenEndpoint: "https://as.example.com/oauth/token",
		PKCEVerifier:            "verifier-" + state,
		DPoPAuthServerNonce:     "nonce",
		DPoPPrivateKeyMultibase: "zfake",
	}
}

func TestSession_PutGet_RoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in := sampleSession("did:plc:alice", "sid-1")
	if err := s.SaveSession(ctx, in); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	got, err := s.GetSession(ctx, in.AccountDID, in.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.AccountDID != in.AccountDID || got.SessionID != in.SessionID {
		t.Errorf("keys differ: got (%s, %s)", got.AccountDID, got.SessionID)
	}
	if got.AccessToken != in.AccessToken || got.RefreshToken != in.RefreshToken {
		t.Errorf("tokens differ")
	}
}

func TestSession_Get_UnknownReturnsErr(t *testing.T) {
	s := newStore(t)
	d, _ := syntax.ParseDID("did:plc:nobody")
	_, err := s.GetSession(context.Background(), d, "missing")
	if err == nil {
		t.Fatal("expected error for missing session, got nil")
	}
}

func TestSession_MultipleSessionsPerDID(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	a := sampleSession("did:plc:alice", "sid-1")
	b := sampleSession("did:plc:alice", "sid-2")
	if err := s.SaveSession(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSession(ctx, b); err != nil {
		t.Fatal(err)
	}
	gotA, err := s.GetSession(ctx, a.AccountDID, a.SessionID)
	if err != nil {
		t.Fatalf("GetSession A: %v", err)
	}
	gotB, err := s.GetSession(ctx, b.AccountDID, b.SessionID)
	if err != nil {
		t.Fatalf("GetSession B: %v", err)
	}
	if gotA.AccessToken == gotB.AccessToken {
		t.Errorf("expected independent rows")
	}
	if err := s.DeleteSession(ctx, a.AccountDID, a.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, a.AccountDID, a.SessionID); err == nil {
		t.Errorf("deleted A still present")
	}
	if _, err := s.GetSession(ctx, b.AccountDID, b.SessionID); err != nil {
		t.Errorf("B should still exist: %v", err)
	}
}

func TestSession_SaveUpserts(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in := sampleSession("did:plc:alice", "sid-1")
	if err := s.SaveSession(ctx, in); err != nil {
		t.Fatal(err)
	}
	in.AccessToken = "rotated"
	if err := s.SaveSession(ctx, in); err != nil {
		t.Fatalf("second SaveSession should upsert: %v", err)
	}
	got, err := s.GetSession(ctx, in.AccountDID, in.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "rotated" {
		t.Errorf("upsert didn't replace tokens: %q", got.AccessToken)
	}
}

func TestAuthRequest_PutGet_RoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in := sampleAuthRequest("state-1")
	if err := s.SaveAuthRequestInfo(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetAuthRequestInfo(ctx, "state-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != in.State || got.PKCEVerifier != in.PKCEVerifier {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func TestAuthRequest_Get_UnknownReturnsErr(t *testing.T) {
	s := newStore(t)
	_, err := s.GetAuthRequestInfo(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing auth request")
	}
}

func TestAuthRequest_Delete(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in := sampleAuthRequest("state-1")
	if err := s.SaveAuthRequestInfo(ctx, in); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAuthRequestInfo(ctx, "state-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAuthRequestInfo(ctx, "state-1"); err == nil {
		t.Error("auth request still present after delete")
	}
}

// Read-time expiry check backs GC-driven cleanup, so callers never consume a stale request.
func TestAuthRequest_ExpiredNotReturned(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in := sampleAuthRequest("state-old")
	if err := s.SaveAuthRequestInfo(ctx, in); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE oauth_auth_requests SET expires_at = ? WHERE state = ?`,
		past, "state-old",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAuthRequestInfo(ctx, "state-old"); err == nil {
		t.Error("expected error for expired auth request, got nil")
	}
}

func TestAuthRequest_GCDeletesExpired(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.SaveAuthRequestInfo(ctx, sampleAuthRequest("fresh")); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAuthRequestInfo(ctx, sampleAuthRequest("stale")); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE oauth_auth_requests SET expires_at = ? WHERE state = ?`,
		past, "stale",
	); err != nil {
		t.Fatal(err)
	}

	n, err := s.GCExpired(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("GCExpired returned %d, want 1", n)
	}

	if _, err := s.GetAuthRequestInfo(ctx, "fresh"); err != nil {
		t.Errorf("fresh row should still exist: %v", err)
	}
}

// Two goroutines racing the same key see exactly one refresh; the loser reads the winner's fresh token.
func TestLockSession_OnlyOneRefresh_SameKey(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in := sampleSession("did:plc:alice", "sid-1")
	in.AccessToken = "stale"
	if err := s.SaveSession(ctx, in); err != nil {
		t.Fatal(err)
	}

	const numGoroutines = 8
	var refreshCount atomic.Int32
	start := make(chan struct{})
	done := make(chan struct{}, numGoroutines)

	for range numGoroutines {
		go func() {
			defer func() { done <- struct{}{} }()
			<-start
			unlock := s.LockSession(in.AccountDID, in.SessionID)
			defer unlock()
			cur, err := s.GetSession(ctx, in.AccountDID, in.SessionID)
			if err != nil {
				t.Errorf("GetSession: %v", err)
				return
			}
			if cur.AccessToken == "stale" {
				// simulate AS round-trip
				time.Sleep(20 * time.Millisecond)
				refreshCount.Add(1)
				cur.AccessToken = "fresh"
				if err := s.SaveSession(ctx, *cur); err != nil {
					t.Errorf("SaveSession: %v", err)
				}
			}
		}()
	}
	close(start)
	for range numGoroutines {
		<-done
	}

	if got := refreshCount.Load(); got != 1 {
		t.Errorf("refreshCount = %d, want 1 (refresh race)", got)
	}
	final, err := s.GetSession(ctx, in.AccountDID, in.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if final.AccessToken != "fresh" {
		t.Errorf("final access token = %q, want fresh", final.AccessToken)
	}
}

// Independent (did, sid) pairs must not serialize their locks.
func TestLockSession_IndependentKeysRunInParallel(t *testing.T) {
	s := newStore(t)
	didA, _ := syntax.ParseDID("did:plc:alice")
	didB, _ := syntax.ParseDID("did:plc:bob")

	const hold = 100 * time.Millisecond
	var wg sync.WaitGroup
	wg.Add(2)
	startA, startB := make(chan struct{}), make(chan struct{})
	doneA, doneB := make(chan time.Time, 1), make(chan time.Time, 1)

	go func() {
		defer wg.Done()
		<-startA
		unlock := s.LockSession(didA, "sid-1")
		time.Sleep(hold)
		unlock()
		doneA <- time.Now()
	}()
	go func() {
		defer wg.Done()
		<-startB
		unlock := s.LockSession(didB, "sid-1")
		time.Sleep(hold)
		unlock()
		doneB <- time.Now()
	}()

	t0 := time.Now()
	close(startA)
	close(startB)
	wg.Wait()
	elapsed := time.Since(t0)

	// Shared locks would force ≥200ms serial; parallel finishes in ~100ms (slack for CI scheduling).
	if elapsed > 180*time.Millisecond {
		t.Errorf("independent keys serialized (elapsed %v, holds %v)", elapsed, hold)
	}
}

func TestSession_DataBlobIsEncrypted(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in := sampleSession("did:plc:alice", "sid-1")
	if err := s.SaveSession(ctx, in); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	row := s.db.QueryRowContext(ctx,
		`SELECT data FROM oauth_sessions WHERE did = ? AND session_id = ?`,
		in.AccountDID.String(), in.SessionID,
	)
	if err := row.Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(raw, []byte("{")) {
		t.Errorf("session blob is plaintext JSON at rest: %s", raw[:min(40, len(raw))])
	}
	if bytes.Contains(raw, []byte(in.AccessToken)) || bytes.Contains(raw, []byte(in.RefreshToken)) {
		t.Error("session blob leaks token material at rest")
	}
}

// Rotated keyset [newKey, oldKey] must still decrypt a session sealed under oldKey (the SESSION_STORE_KEYS rotation path).
func TestStore_KeyRotation_ReadsOldSession(t *testing.T) {
	db := newMemDB(t)
	ctx := context.Background()

	oldKey := make([]byte, secret.KeySize)
	if _, err := rand.Read(oldKey); err != nil {
		t.Fatal(err)
	}
	oldSet, _ := secret.NewKeyset(oldKey)
	in := sampleSession("did:plc:alice", "sid-1")
	if err := New(db, oldSet).SaveSession(ctx, in); err != nil {
		t.Fatal(err)
	}

	newKey := make([]byte, secret.KeySize)
	if _, err := rand.Read(newKey); err != nil {
		t.Fatal(err)
	}
	rotated, _ := secret.NewKeyset(newKey, oldKey)
	got, err := New(db, rotated).GetSession(ctx, in.AccountDID, in.SessionID)
	if err != nil {
		t.Fatalf("rotated store couldn't read old session: %v", err)
	}
	if got.AccessToken != in.AccessToken {
		t.Errorf("token mismatch after rotation: %q", got.AccessToken)
	}
}

func TestSatisfiesInterface(t *testing.T) {
	var _ oauth.ClientAuthStore = (*Store)(nil)
}
