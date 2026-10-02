package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"morgenblau/internal/session"

	"morgenblau/internal/atprepo"
)

const (
	existenceCollection = "blue.morgen.test.existence"
	sidecarCollection   = "blue.morgen.test.sidecar"
)

func sweepTestSession() *session.Session {
	d, _ := syntax.ParseDID("did:plc:alice")
	return &session.Session{Data: &session.Data{AccountDID: d, SessionID: "sid-1"}}
}

// --- deleteStandardSubscription ---

func standardDeletePDS() *fakePDS {
	return &fakePDS{listed: map[string][]atprepo.ListedRecord{
		standardSubCollection: {
			{URI: "at://did:plc:alice/" + standardSubCollection + "/3a", Value: map[string]any{"publication": testPublication}},
			{URI: "at://did:plc:alice/" + standardSubCollection + "/3b", Value: map[string]any{"publication": testPublication}},
			{URI: "at://did:plc:alice/" + standardSubCollection + "/3c", Value: map[string]any{"publication": "at://did:plc:other/site.standard.publication/3x"}},
		},
		subscriptionCollection: {
			{URI: "at://did:plc:alice/" + subscriptionCollection + "/3side", Value: map[string]any{}},
		},
	}}
}

func TestDeleteStandardSubscription_RemovesDuplicatesAndSidecarInOneCommit(t *testing.T) {
	pds := standardDeletePDS()
	rr := httptest.NewRecorder()
	if !deleteStandardSubscription(context.Background(), rr, sweepTestSession(), pds, testPublication, "3side") {
		t.Fatalf("returned false; body = %s", rr.Body.String())
	}
	want := []string{standardSubCollection + "/3a", standardSubCollection + "/3b", subscriptionCollection + "/3side"}
	if pds.applyCalls != 1 || !slices.Equal(pds.appliedDeletes, want) || len(pds.deleted) != 0 {
		t.Fatalf("applyWrites calls = %d, deletes = %v, single deletes = %v; want one commit deleting %v", pds.applyCalls, pds.appliedDeletes, pds.deleted, want)
	}
}

func TestDeleteStandardSubscription_SkipsASidecarThePDSNoLongerHolds(t *testing.T) {
	pds := standardDeletePDS()
	rr := httptest.NewRecorder()
	if !deleteStandardSubscription(context.Background(), rr, sweepTestSession(), pds, testPublication, "3gone") {
		t.Fatalf("returned false; body = %s", rr.Body.String())
	}
	want := []string{standardSubCollection + "/3a", standardSubCollection + "/3b"}
	if !slices.Equal(pds.appliedDeletes, want) {
		t.Fatalf("deletes = %v, want %v", pds.appliedDeletes, want)
	}
}

func TestDeleteStandardSubscription_FailedCommitDeletesNothing(t *testing.T) {
	pds := standardDeletePDS()
	pds.applyErr = errors.New("pds down")
	rr := httptest.NewRecorder()
	if deleteStandardSubscription(context.Background(), rr, sweepTestSession(), pds, testPublication, "3side") {
		t.Fatal("returned true, want false when the commit fails")
	}
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
	if len(pds.listed[standardSubCollection]) != 3 || len(pds.listed[subscriptionCollection]) != 1 {
		t.Errorf("listed = %v, want every record still present", pds.listed)
	}
}

func TestDeleteStandardSubscription_ListError_Writes502(t *testing.T) {
	pds := &fakePDS{listErr: errors.New("pds down")}
	rr := httptest.NewRecorder()
	if deleteStandardSubscription(context.Background(), rr, sweepTestSession(), pds, testPublication, "3side") {
		t.Fatal("returned true, want false on list error")
	}
	if rr.Code != http.StatusBadGateway || pds.applyCalls != 0 {
		t.Errorf("status = %d, applyWrites calls = %d; want 502 and no commit", rr.Code, pds.applyCalls)
	}
}

func TestDeleteStandardSubscription_NothingLeftCommitsNothing(t *testing.T) {
	pds := &fakePDS{}
	rr := httptest.NewRecorder()
	if !deleteStandardSubscription(context.Background(), rr, sweepTestSession(), pds, testPublication, "") {
		t.Fatalf("returned false; body = %s", rr.Body.String())
	}
	if pds.applyCalls != 0 {
		t.Errorf("applyWrites calls = %d, want 0", pds.applyCalls)
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

func TestWriteSidecarPair_ExistenceAndSidecar_UsesOneAtomicWrite(t *testing.T) {
	pds := &fakePDS{}
	rr := httptest.NewRecorder()
	spec := sidecarWriteSpec{
		Existence:           map[string]any{"subject": "example"},
		ExistenceCollection: syntax.NSID(existenceCollection),
		ExistenceRkey:       syntax.RecordKey("3exist"),
		ExistenceOp:         "existence create failed",
		Sidecar:             map[string]any{"note": "extra"},
		SidecarCollection:   syntax.NSID(sidecarCollection),
		SidecarCreateRkey:   syntax.RecordKey("3side"),
		SidecarOp:           "sidecar create failed",
	}
	result, ok := writeSidecarPair(context.Background(), rr, sweepTestSession(), pds, spec)

	if !ok {
		t.Fatalf("writeSidecarPair returned false; body = %s", rr.Body.String())
	}
	if result.ExistenceRef == nil {
		t.Fatal("ExistenceRef is nil, want the created ref")
	}
	if result.SidecarRkey == "" {
		t.Fatal("SidecarRkey is empty, want the newly created rkey")
	}
	if pds.applyCalls != 1 || len(pds.applied) != 2 {
		t.Fatalf("applyWrites calls=%d writes=%v, want one atomic call with two writes", pds.applyCalls, pds.applied)
	}
	if pds.applied[0].collection != existenceCollection {
		t.Errorf("first write = %q, want the existence record first", pds.applied[0].collection)
	}
	if pds.applied[1].collection != sidecarCollection {
		t.Errorf("second write = %q, want the sidecar second", pds.applied[1].collection)
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

func TestWriteSidecarPair_AtomicWriteFails_502_NoExistenceRef(t *testing.T) {
	pds := &fakePDS{applyErr: errors.New("pds down")}
	rr := httptest.NewRecorder()
	spec := sidecarWriteSpec{
		Existence:           map[string]any{"subject": "example"},
		ExistenceCollection: syntax.NSID(existenceCollection),
		ExistenceRkey:       syntax.RecordKey("3exist"),
		ExistenceOp:         "existence create failed",
		Sidecar:             map[string]any{"note": "extra"},
		SidecarCollection:   syntax.NSID(sidecarCollection),
		SidecarCreateRkey:   syntax.RecordKey("3side"),
		SidecarOp:           "sidecar create failed",
	}
	result, ok := writeSidecarPair(context.Background(), rr, sweepTestSession(), pds, spec)

	if ok {
		t.Fatal("writeSidecarPair returned true, want false")
	}
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
	if result.ExistenceRef != nil {
		t.Errorf("ExistenceRef = %+v, want nil after the atomic request failed", result.ExistenceRef)
	}
	if pds.applyCalls != 1 || len(pds.applied) != 0 || pds.creates != 0 {
		t.Errorf("applyWrites calls=%d applied=%v creates=%d, want failed atomic call and no committed records", pds.applyCalls, pds.applied, pds.creates)
	}
}

func TestWriteSidecarPair_AmbiguousCommitConfirmsByRereadingBothRecords(t *testing.T) {
	pds := &fakePDS{applyAfterCommitErr: errors.New("connection lost after commit")}
	rr := httptest.NewRecorder()
	spec := sidecarWriteSpec{
		Existence:           map[string]any{"subject": "example"},
		ExistenceCollection: syntax.NSID(existenceCollection),
		ExistenceRkey:       syntax.RecordKey("3exist"),
		Sidecar:             map[string]any{"note": "extra"},
		SidecarCollection:   syntax.NSID(sidecarCollection),
		SidecarCreateRkey:   syntax.RecordKey("3side"),
	}

	result, ok := writeSidecarPair(context.Background(), rr, sweepTestSession(), pds, spec)
	if !ok {
		t.Fatalf("writeSidecarPair returned false after both records were confirmed; body = %s", rr.Body.String())
	}
	if result.ExistenceRef == nil || result.ExistenceRef.URI != "at://did:plc:alice/"+existenceCollection+"/3exist" || result.SidecarRkey != "3side" {
		t.Errorf("result = %+v, want refs for both committed records", result)
	}
	if pds.getCalls != 2 {
		t.Errorf("GetRecord calls = %d, want one readback per record", pds.getCalls)
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
