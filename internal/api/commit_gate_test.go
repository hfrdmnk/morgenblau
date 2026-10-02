package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
)

func TestCommitGate_RefusesASecondCommitBeforeItReachesThePDS(t *testing.T) {
	pds := &fakePDS{}
	gate := CommitGate{Repo: pds}
	sess := mirrorSession()
	var first, second error
	OneCommitPerRequest(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, first = gate.CreateRecord(r.Context(), sess, syntax.NSID(saveCollection), map[string]any{"itemUrl": "https://example.com/a"})
		second = gate.DeleteRecord(r.Context(), sess, syntax.NSID(saveCollection), "3la1")
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))

	if first != nil {
		t.Fatalf("first commit: %v", first)
	}
	if !errors.Is(second, errCommitAlreadySpent) {
		t.Fatalf("second commit err = %v, want errCommitAlreadySpent", second)
	}
	if pds.creates != 1 || len(pds.deleted) != 0 {
		t.Fatalf("creates = %d, deletes = %v; the refused commit must never reach the PDS", pds.creates, pds.deleted)
	}
}

func TestCommitGate_EveryCommitKindSpendsTheBudget(t *testing.T) {
	sess := mirrorSession()
	commits := map[string]func(context.Context, CommitGate) error{
		"create": func(ctx context.Context, g CommitGate) error {
			_, err := g.CreateRecord(ctx, sess, syntax.NSID(saveCollection), map[string]any{})
			return err
		},
		"put": func(ctx context.Context, g CommitGate) error {
			_, err := g.PutRecord(ctx, sess, syntax.NSID(saveCollection), "3la", map[string]any{})
			return err
		},
		"delete": func(ctx context.Context, g CommitGate) error {
			return g.DeleteRecord(ctx, sess, syntax.NSID(saveCollection), "3la")
		},
		"applyWrites": func(ctx context.Context, g CommitGate) error {
			_, err := g.ApplyWrites(ctx, sess, []atprepo.RecordWrite{{Collection: syntax.NSID(saveCollection), Rkey: "3la", Delete: true}})
			return err
		},
	}
	for first, spend := range commits {
		for second, again := range commits {
			t.Run(first+" then "+second, func(t *testing.T) {
				gate := CommitGate{Repo: &fakePDS{}}
				var errs [2]error
				OneCommitPerRequest(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
					errs[0] = spend(r.Context(), gate)
					errs[1] = again(r.Context(), gate)
				})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))
				if errs[0] != nil || !errors.Is(errs[1], errCommitAlreadySpent) {
					t.Fatalf("errs = %v, want the first to pass and the second to be refused", errs)
				}
			})
		}
	}
}

func TestCommitGate_EachRequestGetsItsOwnCommit(t *testing.T) {
	pds := &fakePDS{}
	gate := CommitGate{Repo: pds}
	h := OneCommitPerRequest(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := gate.CreateRecord(r.Context(), mirrorSession(), syntax.NSID(saveCollection), map[string]any{}); err != nil {
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	for range 2 {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want each request to spend its own commit", rr.Code)
		}
	}
	if pds.creates != 2 {
		t.Fatalf("creates = %d, want 2", pds.creates)
	}
}

func TestCommitGate_RefusesCommitsOutsideABudgetedRequest(t *testing.T) {
	pds := &fakePDS{}
	_, err := CommitGate{Repo: pds}.CreateRecord(context.Background(), mirrorSession(), syntax.NSID(saveCollection), map[string]any{})
	if !errors.Is(err, errCommitAlreadySpent) || pds.creates != 0 {
		t.Fatalf("err = %v, creates = %d; an unwrapped route must fail closed", err, pds.creates)
	}
}

func TestCommitGate_ReadsPassThroughWithoutSpending(t *testing.T) {
	pds := &fakePDS{listed: map[string][]atprepo.ListedRecord{saveCollection: {{URI: "at://did:plc:alice/" + saveCollection + "/3la"}}}}
	gate := CommitGate{Repo: pds}
	var listErr, getErr, commitErr error
	OneCommitPerRequest(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, listErr = gate.ListRecords(r.Context(), mirrorSession(), syntax.NSID(saveCollection))
		_, getErr = gate.GetRecord(r.Context(), mirrorSession(), syntax.NSID(saveCollection), "3la")
		_, commitErr = gate.CreateRecord(r.Context(), mirrorSession(), syntax.NSID(saveCollection), map[string]any{})
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))
	if listErr != nil || getErr != nil || commitErr != nil {
		t.Fatalf("list = %v, get = %v, commit = %v; reads must not spend the commit", listErr, getErr, commitErr)
	}
}
