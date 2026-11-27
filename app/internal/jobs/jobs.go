package jobs

import (
	"crypto/rand"
	"errors"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/oklog/ulid/v2"
)

type Status string

const (
	StatusPending Status = "pending"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

type Kind string

const (
	KindSyncUser     Kind = "sync_user"
	KindFetchOneFeed Kind = "fetch_one_feed"
)

type Trigger string

const (
	TriggerLogin   Trigger = "login"
	TriggerManual  Trigger = "manual"
	TriggerAddFeed Trigger = "add"
)

type Job struct {
	ID         string    `json:"id"`
	Kind       Kind      `json:"kind"`
	Trigger    Trigger   `json:"trigger,omitempty"`
	UserDID    string    `json:"-"`
	Status     Status    `json:"status"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
}

const DefaultRetention = 5 * time.Minute

var ErrNotFound = errors.New("jobs: not found")

var ErrForbidden = errors.New("jobs: forbidden")

type Tracker struct {
	mu           sync.Mutex
	jobs         map[string]*Job
	latestSync   map[string]*Job
	syncFailures map[string]*Job
	retention    time.Duration
	now          func() time.Time
	entropy      *ulid.MonotonicEntropy
}

func New() *Tracker {
	return NewWithOptions(DefaultRetention, time.Now)
}

func NewWithOptions(retention time.Duration, now func() time.Time) *Tracker {
	return &Tracker{
		jobs:         make(map[string]*Job),
		latestSync:   make(map[string]*Job),
		syncFailures: make(map[string]*Job),
		retention:    retention,
		now:          now,
		entropy:      ulid.Monotonic(rand.Reader, 0),
	}
}

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
	t.trackSyncLocked(j)
	return j
}

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
	t.trackSyncLocked(j)
	return cloneJob(j), false
}

func (t *Tracker) SetRunning(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[id]; ok {
		j.Status = StatusRunning
		j.FinishedAt = time.Time{}
		t.trackSyncLocked(j)
	}
}

func (t *Tracker) SetDone(id string) {
	t.transition(id, StatusDone)
}

func (t *Tracker) SetFailed(id string) {
	t.transition(id, StatusFailed)
}

func (t *Tracker) transition(id string, s Status) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[id]; ok {
		j.Status = s
		if s == StatusDone || s == StatusFailed {
			j.FinishedAt = t.now()
		} else {
			j.FinishedAt = time.Time{}
		}
		t.trackSyncLocked(j)
	}
}

func (t *Tracker) trackSyncLocked(j *Job) {
	if j.Kind != KindSyncUser {
		return
	}
	did := j.UserDID
	if jobIsNewer(j, t.latestSync[did]) {
		t.latestSync[did] = j
	}
	switch j.Status {
	case StatusFailed:
		if jobIsNewer(j, t.syncFailures[did]) {
			t.syncFailures[did] = j
		}
	case StatusDone:
		if failed := t.syncFailures[did]; failed != nil && jobIsNewer(j, failed) {
			delete(t.syncFailures, did)
		}
	}
}

func jobIsNewer(job, current *Job) bool {
	if current == nil || job.StartedAt.After(current.StartedAt) {
		return true
	}
	return job.StartedAt.Equal(current.StartedAt) && job.ID > current.ID
}

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

func (t *Tracker) LatestSyncForUser(userDID syntax.DID) *Job {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.gcLocked()
	did := userDID.String()
	if latest := t.latestSync[did]; latest != nil && (latest.Status == StatusPending || latest.Status == StatusRunning) {
		return cloneJob(latest)
	}
	if failed := t.syncFailures[did]; failed != nil {
		return cloneJob(failed)
	}
	if latest := t.latestSync[did]; latest != nil {
		return cloneJob(latest)
	}
	return nil
}

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
			if t.latestSync[j.UserDID] == j {
				delete(t.latestSync, j.UserDID)
			}
		}
	}
}

func cloneJob(j *Job) *Job {
	c := *j
	return &c
}
