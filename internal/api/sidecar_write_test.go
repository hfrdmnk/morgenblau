package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
)

const (
	sweepCollection     = "blue.morgen.test.sweep"
	existenceCollection = "blue.morgen.test.existence"
	sidecarCollection   = "blue.morgen.test.sidecar"
)

func sweepTestSession() *oauth.ClientSession {
	d, _ := syntax.ParseDID("did:plc:alice")
	return &oauth.ClientSession{Data: &oauth.ClientSessionData{AccountDID: d, SessionID: "sid-1"}}
}

func TestSweepDuplicates_DeletesAllMatches(t *testing.T) {
	pds := &fakePDS{listed: map[string][]atprepo.ListedRecord{
		sweepCollection: {
			{URI: "at://did:plc:alice/" + sweepCollection + "/3a", Value: map[string]any{"subject": "want"}},
			{URI: "at://did:plc:alice/" + sweepCollection + "/3b", Value: map[string]any{"subject": "want"}},
			{URI: "at://did:plc:alice/" + sweepCollection + "/3c", Value: map[string]any{"subject": "other"}},
		},
	}}
	rr := httptest.NewRecorder()
	ok := sweepDuplicates(context.Background(), rr, sweepTestSession(), pds, "test op", syntax.NSID(sweepCollection), stringField("subject"), "want")

	if !ok {
		t.Fatalf("sweepDuplicates returned false, want true; body = %s", rr.Body.String())
	}
	want := []string{sweepCollection + "/3a", sweepCollection + "/3b"}
	if len(pds.deleted) != len(want) {
		t.Fatalf("deleted = %v, want %v", pds.deleted, want)
	}
	for i := range want {
		if pds.deleted[i] != want[i] {
			t.Errorf("deleted[%d] = %q, want %q", i, pds.deleted[i], want[i])
		}
	}
}

func TestSweepDuplicates_ToleratesZeroMatches(t *testing.T) {
	pds := &fakePDS{listed: map[string][]atprepo.ListedRecord{
		sweepCollection: {
			{URI: "at://did:plc:alice/" + sweepCollection + "/3c", Value: map[string]any{"subject": "other"}},
		},
	}}
	rr := httptest.NewRecorder()
	ok := sweepDuplicates(context.Background(), rr, sweepTestSession(), pds, "test op", syntax.NSID(sweepCollection), stringField("subject"), "want")

	if !ok {
		t.Fatalf("sweepDuplicates returned false, want true; body = %s", rr.Body.String())
	}
	if len(pds.deleted) != 0 {
		t.Errorf("deleted = %v, want none", pds.deleted)
	}
}

func TestSweepDuplicates_ListError_Writes502(t *testing.T) {
	pds := &fakePDS{listErr: errors.New("pds down")}
	rr := httptest.NewRecorder()
	ok := sweepDuplicates(context.Background(), rr, sweepTestSession(), pds, "test op", syntax.NSID(sweepCollection), stringField("subject"), "want")

	if ok {
		t.Fatal("sweepDuplicates returned true, want false on list error")
	}
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
	if len(pds.deleted) != 0 {
		t.Errorf("deleted = %v, want none", pds.deleted)
	}
}

func TestSweepDuplicates_DeleteError_StopsAndWrites502(t *testing.T) {
	pds := &fakePDS{
		listed: map[string][]atprepo.ListedRecord{
			sweepCollection: {
				{URI: "at://did:plc:alice/" + sweepCollection + "/3a", Value: map[string]any{"subject": "want"}},
				{URI: "at://did:plc:alice/" + sweepCollection + "/3b", Value: map[string]any{"subject": "want"}},
			},
		},
		deleteErr: errors.New("pds down"),
	}
	rr := httptest.NewRecorder()
	ok := sweepDuplicates(context.Background(), rr, sweepTestSession(), pds, "test op", syntax.NSID(sweepCollection), stringField("subject"), "want")

	if ok {
		t.Fatal("sweepDuplicates returned true, want false on delete error")
	}
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
	// The first matching delete is attempted (and fails); the loop must not continue past it.
	if len(pds.deleted) != 0 {
		t.Errorf("deleted = %v, want none recorded since fakePDS.DeleteRecord fails before appending", pds.deleted)
	}
}

// --- writeSidecarPair (half B prototype) ---

func TestWriteSidecarPair_ExistenceOnly_CreatesExistence(t *testing.T) {
	pds := &fakePDS{}
	rr := httptest.NewRecorder()
	spec := sidecarWriteSpec{
		Existence:           map[string]any{"subject": "example"},
		ExistenceCollection: syntax.NSID(existenceCollection),
		ExistenceOp:         "existence create failed",
	}
	result, ok := writeSidecarPair(context.Background(), rr, sweepTestSession(), pds, spec)

	if !ok {
		t.Fatalf("writeSidecarPair returned false; body = %s", rr.Body.String())
	}
	if result.ExistenceRef == nil {
		t.Fatal("ExistenceRef is nil, want the created ref")
	}
	if result.SidecarRkey != "" {
		t.Errorf("SidecarRkey = %q, want empty (no sidecar in spec)", result.SidecarRkey)
	}
	if pds.creates != 1 {
		t.Errorf("creates = %d, want 1", pds.creates)
	}
}

func TestWriteSidecarPair_ExistenceAndSidecar_CreatesBothInOrder(t *testing.T) {
	pds := &fakePDS{}
	rr := httptest.NewRecorder()
	spec := sidecarWriteSpec{
		Existence:           map[string]any{"subject": "example"},
		ExistenceCollection: syntax.NSID(existenceCollection),
		ExistenceOp:         "existence create failed",
		Sidecar:             map[string]any{"note": "extra"},
		SidecarCollection:   syntax.NSID(sidecarCollection),
		SidecarOp:           "sidecar create failed",
	}
	result, ok := writeSidecarPair(context.Background(), rr, sweepTestSession(), pds, spec)

	if !ok {
		t.Fatalf("writeSidecarPair returned false; body = %s", rr.Body.String())
	}
	if result.SidecarRkey == "" {
		t.Fatal("SidecarRkey is empty, want the newly created rkey")
	}
	if len(pds.created) != 2 {
		t.Fatalf("created = %v, want 2 writes", pds.created)
	}
	if pds.created[0].collection != existenceCollection {
		t.Errorf("first create = %q, want the existence record written first", pds.created[0].collection)
	}
	if pds.created[1].collection != sidecarCollection {
		t.Errorf("second create = %q, want the sidecar written second", pds.created[1].collection)
	}
}

func TestWriteSidecarPair_SidecarOnly_CreatesWhenNoRkeyKnown(t *testing.T) {
	pds := &fakePDS{}
	rr := httptest.NewRecorder()
	spec := sidecarWriteSpec{
		Sidecar:           map[string]any{"note": "extra"},
		SidecarCollection: syntax.NSID(sidecarCollection),
		SidecarOp:         "sidecar write failed",
	}
	result, ok := writeSidecarPair(context.Background(), rr, sweepTestSession(), pds, spec)

	if !ok {
		t.Fatalf("writeSidecarPair returned false; body = %s", rr.Body.String())
	}
	if result.ExistenceRef != nil {
		t.Error("ExistenceRef is set, want nil (no existence in spec)")
	}
	if pds.creates != 1 || pds.puts != 0 {
		t.Errorf("creates=%d puts=%d, want 1 create and 0 puts", pds.creates, pds.puts)
	}
	if result.SidecarRkey == "" {
		t.Fatal("SidecarRkey is empty, want the newly created rkey")
	}
}

func TestWriteSidecarPair_SidecarOnly_PutsWhenRkeyKnown(t *testing.T) {
	pds := &fakePDS{}
	rr := httptest.NewRecorder()
	spec := sidecarWriteSpec{
		Sidecar:           map[string]any{"note": "extra"},
		SidecarCollection: syntax.NSID(sidecarCollection),
		SidecarRkey:       "3existing",
		SidecarOp:         "sidecar write failed",
	}
	result, ok := writeSidecarPair(context.Background(), rr, sweepTestSession(), pds, spec)

	if !ok {
		t.Fatalf("writeSidecarPair returned false; body = %s", rr.Body.String())
	}
	if pds.puts != 1 || pds.creates != 0 {
		t.Errorf("puts=%d creates=%d, want 1 put and 0 creates", pds.puts, pds.creates)
	}
	if pds.lastPutRkey != "3existing" {
		t.Errorf("put rkey = %q, want 3existing", pds.lastPutRkey)
	}
	if result.SidecarRkey != "3existing" {
		t.Errorf("SidecarRkey = %q, want 3existing", result.SidecarRkey)
	}
}

func TestWriteSidecarPair_ExistenceFails_502_SidecarNeverAttempted(t *testing.T) {
	pds := &fakePDS{createErr: map[string]error{existenceCollection: errors.New("pds down")}}
	rr := httptest.NewRecorder()
	spec := sidecarWriteSpec{
		Existence:           map[string]any{"subject": "example"},
		ExistenceCollection: syntax.NSID(existenceCollection),
		ExistenceOp:         "existence create failed",
		Sidecar:             map[string]any{"note": "extra"},
		SidecarCollection:   syntax.NSID(sidecarCollection),
		SidecarOp:           "sidecar create failed",
	}
	_, ok := writeSidecarPair(context.Background(), rr, sweepTestSession(), pds, spec)

	if ok {
		t.Fatal("writeSidecarPair returned true, want false")
	}
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
	if pds.creates != 0 {
		t.Errorf("creates = %d, want 0 (sidecar must not be attempted)", pds.creates)
	}
}

func TestWriteSidecarPair_SidecarFails_502_ExistenceRefStillReturned(t *testing.T) {
	pds := &fakePDS{createErr: map[string]error{sidecarCollection: errors.New("pds down")}}
	rr := httptest.NewRecorder()
	spec := sidecarWriteSpec{
		Existence:           map[string]any{"subject": "example"},
		ExistenceCollection: syntax.NSID(existenceCollection),
		ExistenceOp:         "existence create failed",
		Sidecar:             map[string]any{"note": "extra"},
		SidecarCollection:   syntax.NSID(sidecarCollection),
		SidecarOp:           "sidecar create failed",
	}
	result, ok := writeSidecarPair(context.Background(), rr, sweepTestSession(), pds, spec)

	if ok {
		t.Fatal("writeSidecarPair returned true, want false")
	}
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
	// The existence record already committed on the PDS before the sidecar failed; reconcile can still adopt it bare.
	if result.ExistenceRef == nil {
		t.Error("ExistenceRef is nil, want the already-committed existence ref")
	}
}

func TestWriteSidecarPair_SidecarPutFails_502(t *testing.T) {
	pds := &fakePDS{putErr: errors.New("pds down")}
	rr := httptest.NewRecorder()
	spec := sidecarWriteSpec{
		Sidecar:           map[string]any{"note": "extra"},
		SidecarCollection: syntax.NSID(sidecarCollection),
		SidecarRkey:       "3existing",
		SidecarOp:         "sidecar write failed",
	}
	_, ok := writeSidecarPair(context.Background(), rr, sweepTestSession(), pds, spec)

	if ok {
		t.Fatal("writeSidecarPair returned true, want false")
	}
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}
