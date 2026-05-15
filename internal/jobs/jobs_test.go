package jobs

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

func didFor(t *testing.T, s string) syntax.DID {
	t.Helper()
	d, err := syntax.ParseDID(s)
	if err != nil {
		t.Fatalf("ParseDID: %v", err)
	}
	return d
}

func TestCreate_UniqueIDs(t *testing.T) {
	tr := New()
	alice := didFor(t, "did:plc:alice")
	seen := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		j := tr.Create(KindSyncUser, alice, TriggerLogin)
		if _, dup := seen[j.ID]; dup {
			t.Fatalf("dup id: %s", j.ID)
		}
		seen[j.ID] = struct{}{}
	}
}

func TestCreate_ConcurrentUniqueness(t *testing.T) {
	tr := New()
	alice := didFor(t, "did:plc:alice")
	const n = 200
	ids := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			ids[i] = tr.Create(KindSyncUser, alice, TriggerLogin).ID
		}()
	}
	wg.Wait()
	seen := make(map[string]struct{}, n)
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			t.Fatalf("dup id under contention: %s", id)
		}
		seen[id] = struct{}{}
	}
}

func TestTransitions(t *testing.T) {
	tr := New()
	alice := didFor(t, "did:plc:alice")
	j := tr.Create(KindSyncUser, alice, TriggerManual)
	if j.Status != StatusPending {
		t.Errorf("create status = %q", j.Status)
	}
	tr.SetRunning(j.ID)
	got, err := tr.Get(j.ID, alice)
	if err != nil || got.Status != StatusRunning {
		t.Errorf("running: %+v %v", got, err)
	}
	tr.SetDone(j.ID)
	got, err = tr.Get(j.ID, alice)
	if err != nil || got.Status != StatusDone {
		t.Errorf("done: %+v %v", got, err)
	}
	if got.FinishedAt.IsZero() {
		t.Errorf("finishedAt zero on done")
	}
}

func TestGet_ForbiddenAcrossUsers(t *testing.T) {
	tr := New()
	alice := didFor(t, "did:plc:alice")
	bob := didFor(t, "did:plc:bob")
	j := tr.Create(KindSyncUser, alice, TriggerLogin)
	_, err := tr.Get(j.ID, bob)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestGet_NotFound(t *testing.T) {
	tr := New()
	_, err := tr.Get("missing", didFor(t, "did:plc:alice"))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestActiveForUser_FiltersByUser(t *testing.T) {
	tr := New()
	alice := didFor(t, "did:plc:alice")
	bob := didFor(t, "did:plc:bob")
	bobJob := tr.Create(KindSyncUser, bob, TriggerLogin)
	_ = bobJob
	if j := tr.ActiveForUser(alice); j != nil {
		t.Errorf("alice should have no active job, got %+v", j)
	}
	aliceJob := tr.Create(KindSyncUser, alice, TriggerLogin)
	if j := tr.ActiveForUser(alice); j == nil || j.ID != aliceJob.ID {
		t.Errorf("ActiveForUser(alice) = %+v, want %s", j, aliceJob.ID)
	}
}

func TestActiveForUser_IgnoresFinished(t *testing.T) {
	tr := New()
	alice := didFor(t, "did:plc:alice")
	j := tr.Create(KindSyncUser, alice, TriggerLogin)
	tr.SetDone(j.ID)
	if got := tr.ActiveForUser(alice); got != nil {
		t.Errorf("finished job should not appear in active: %+v", got)
	}
}

func TestGC_RetentionWindow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tr := NewWithOptions(50*time.Millisecond, func() time.Time { return now })
	alice := didFor(t, "did:plc:alice")
	j := tr.Create(KindSyncUser, alice, TriggerManual)
	tr.SetDone(j.ID)

	// Advance clock past retention.
	now = now.Add(time.Second)
	tr.GC()
	if _, err := tr.Get(j.ID, alice); !errors.Is(err, ErrNotFound) {
		t.Errorf("post-GC Get err = %v, want ErrNotFound", err)
	}
}

func TestExistingInFlight_Guard(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tr := NewWithOptions(DefaultRetention, func() time.Time { return now })
	alice := didFor(t, "did:plc:alice")
	j := tr.Create(KindSyncUser, alice, TriggerLogin)
	tr.SetRunning(j.ID)
	if got := tr.ExistingInFlight(KindSyncUser, alice, 5*time.Minute); got == nil || got.ID != j.ID {
		t.Errorf("guard miss: got %+v", got)
	}

	// After advancing past guard, the job should no longer match.
	now = now.Add(6 * time.Minute)
	if got := tr.ExistingInFlight(KindSyncUser, alice, 5*time.Minute); got != nil {
		t.Errorf("guard should expire, got %+v", got)
	}
}
