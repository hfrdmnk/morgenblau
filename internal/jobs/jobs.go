// Package jobs is the in-memory tracker the refresh-pill polls against.
// Lifecycle status only — no counts, no progress percentages (SPEC
// <feed-sources>). Finished jobs GC after a short retention window so
// /api/jobs/active doesn't accumulate ghosts.
package jobs

import (
	"crypto/rand"
	"errors"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/oklog/ulid/v2"
)

// Status is the lifecycle state of a job. Intentionally minimal — anything
// finer-grained breaks the calm-brand promise ("no counts, no progress").
type Status string

const (
	StatusPending Status = "pending"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

// Kind classifies what work the job is doing.
type Kind string

const (
	KindSyncUser      Kind = "sync_user"
	KindFetchOneFeed  Kind = "fetch_one_feed"
)

// Trigger is metadata-only telemetry; the work the job does is identical.
type Trigger string

const (
	TriggerLogin   Trigger = "login"
	TriggerManual  Trigger = "manual"
	TriggerAddFeed Trigger = "add"
)

// Job is what /api/jobs/{id} returns. Exported fields are the wire format.
type Job struct {
	ID         string    `json:"id"`
	Kind       Kind      `json:"kind"`
	Trigger    Trigger   `json:"trigger,omitempty"`
	UserDID    string    `json:"-"` // never leaked over the wire
	Status     Status    `json:"status"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
}

// Default GC retention — finished jobs disappear from ActiveForUser after
// this window so subsequent reloads of /consume don't see ghost pills.
const DefaultRetention = 5 * time.Minute

// ErrNotFound is returned by Get for unknown ids.
var ErrNotFound = errors.New("jobs: not found")

// ErrForbidden is returned by Get when the id belongs to another user.
var ErrForbidden = errors.New("jobs: forbidden")

// Tracker is the in-memory job map. Concurrency-safe.
type Tracker struct {
	mu        sync.Mutex
	jobs      map[string]*Job
	retention time.Duration
	now       func() time.Time
	entropy   *ulid.MonotonicEntropy
}

// New builds a tracker with the default retention window.
func New() *Tracker {
	return NewWithOptions(DefaultRetention, time.Now)
}

// NewWithOptions exposes retention + clock for tests.
func NewWithOptions(retention time.Duration, now func() time.Time) *Tracker {
	return &Tracker{
		jobs:      make(map[string]*Job),
		retention: retention,
		now:       now,
		entropy:   ulid.Monotonic(rand.Reader, 0),
	}
}

// Create registers a new job in pending state and returns its id. The caller
// is expected to call SetRunning when work starts and SetDone / SetFailed at
// the end.
func (t *Tracker) Create(kind Kind, userDID syntax.DID, trigger Trigger) *Job {
	t.mu.Lock()
	defer t.mu.Unlock()
	id := ulid.MustNew(ulid.Timestamp(t.now()), t.entropy).String()
	j := &Job{
		ID:        id,
		Kind:      kind,
		Trigger:   trigger,
		UserDID:   userDID.String(),
		Status:    StatusPending,
		StartedAt: t.now(),
	}
	t.jobs[id] = j
	return j
}

// CreateOrReturnExisting atomically returns the in-flight (kind, did) job
// within the guard window if one exists, otherwise creates a new pending job
// and returns it. The boolean is true when an existing job was returned.
func (t *Tracker) CreateOrReturnExisting(kind Kind, userDID syntax.DID, trigger Trigger, guard time.Duration) (*Job, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	did := userDID.String()
	cutoff := t.now().Add(-guard)
	for _, j := range t.jobs {
		if j.UserDID != did || j.Kind != kind {
			continue
		}
		if j.Status == StatusDone || j.Status == StatusFailed {
			continue
		}
		if j.StartedAt.After(cutoff) {
			return cloneJob(j), true
		}
	}
	id := ulid.MustNew(ulid.Timestamp(t.now()), t.entropy).String()
	j := &Job{
		ID:        id,
		Kind:      kind,
		Trigger:   trigger,
		UserDID:   did,
		Status:    StatusPending,
		StartedAt: t.now(),
	}
	t.jobs[id] = j
	return cloneJob(j), false
}

// SetRunning marks the job as running. Idempotent.
func (t *Tracker) SetRunning(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[id]; ok {
		j.Status = StatusRunning
	}
}

// SetDone marks the job as done with the current timestamp.
func (t *Tracker) SetDone(id string) {
	t.transition(id, StatusDone)
}

// SetFailed marks the job as failed with the current timestamp.
func (t *Tracker) SetFailed(id string) {
	t.transition(id, StatusFailed)
}

func (t *Tracker) transition(id string, s Status) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[id]; ok {
		j.Status = s
		j.FinishedAt = t.now()
	}
}

// Get returns the job for id and verifies ownership against userDID.
func (t *Tracker) Get(id string, userDID syntax.DID) (*Job, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	j, ok := t.jobs[id]
	if !ok {
		return nil, ErrNotFound
	}
	if j.UserDID != userDID.String() {
		return nil, ErrForbidden
	}
	return cloneJob(j), nil
}

// ActiveForUser returns the most recent in-flight job for the user (any one
// of pending/running). Returns nil if none. Finished jobs older than the
// retention window are GC'd here too — keeps the polling fast path tidy.
func (t *Tracker) ActiveForUser(userDID syntax.DID) *Job {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.gcLocked()
	did := userDID.String()
	var best *Job
	for _, j := range t.jobs {
		if j.UserDID != did {
			continue
		}
		if j.Status == StatusDone || j.Status == StatusFailed {
			continue
		}
		if best == nil || j.StartedAt.After(best.StartedAt) {
			best = j
		}
	}
	if best == nil {
		return nil
	}
	return cloneJob(best)
}

// ExistingInFlight returns an in-flight sync_user job for userDID whose
// StartedAt is within the SPEC <feed-sources> guard window. Used by
// SyncUser to coalesce duplicate login/manual refreshes without re-fetching.
func (t *Tracker) ExistingInFlight(kind Kind, userDID syntax.DID, guard time.Duration) *Job {
	t.mu.Lock()
	defer t.mu.Unlock()
	did := userDID.String()
	cutoff := t.now().Add(-guard)
	for _, j := range t.jobs {
		if j.UserDID != did || j.Kind != kind {
			continue
		}
		if j.Status == StatusDone || j.Status == StatusFailed {
			continue
		}
		if j.StartedAt.After(cutoff) {
			return cloneJob(j)
		}
	}
	return nil
}

// GC sweeps finished jobs older than the retention window. Called
// automatically from ActiveForUser, exposed for direct test invocation.
func (t *Tracker) GC() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.gcLocked()
}

func (t *Tracker) gcLocked() {
	cutoff := t.now().Add(-t.retention)
	for id, j := range t.jobs {
		if j.Status != StatusDone && j.Status != StatusFailed {
			continue
		}
		if j.FinishedAt.Before(cutoff) {
			delete(t.jobs, id)
		}
	}
}

func cloneJob(j *Job) *Job {
	c := *j
	return &c
}
