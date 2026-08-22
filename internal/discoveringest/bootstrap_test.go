package discoveringest

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"morgenblau/internal/database/db"
)

const testSegment = "seg_0000000000.jss"

// fakeArchive serves the Replay endpoints off canned plans and pre-built blocks. A nil receiver answers as an empty sealed archive, so a live-tail test needs no setup.
type fakeArchive struct {
	mu       sync.Mutex
	plans    []planOutput
	requests []planInput
	blocks   map[string][]byte
	segments map[string][]byte
	fetched  []string
}

func blockKey(segment string, index int64) string {
	return segment + "/" + strconv.FormatInt(index, 10)
}

func (a *fakeArchive) serve(w http.ResponseWriter, r *http.Request) {
	if a == nil {
		if r.URL.Path == planPath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sealedTipSeq":0,"plannedThroughSeq":0,"segments":[]}`))
			return
		}
		http.NotFound(w, r)
		return
	}
	switch r.URL.Path {
	case planPath:
		a.servePlan(w, r)
	case blockPath:
		index, err := strconv.ParseInt(r.URL.Query().Get("blockIndex"), 10, 64)
		if err != nil {
			http.Error(w, "bad blockIndex", http.StatusBadRequest)
			return
		}
		a.serveBlob(w, blockKey(r.URL.Query().Get("segment"), index), a.blocks)
	case segmentPath:
		a.serveBlob(w, r.URL.Query().Get("name"), a.segments)
	default:
		http.NotFound(w, r)
	}
}

func (a *fakeArchive) servePlan(w http.ResponseWriter, r *http.Request) {
	var in planInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad plan input", http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	a.requests = append(a.requests, in)
	page := planOutput{}
	if idx := len(a.requests) - 1; idx < len(a.plans) {
		page = a.plans[idx]
	} else if len(a.plans) > 0 {
		// A repeat call past the script means the backfill is done; answer with the tip and no work.
		last := a.plans[len(a.plans)-1]
		page = planOutput{SealedTipSeq: last.SealedTipSeq, PlannedThroughSeq: last.SealedTipSeq}
	}
	a.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(page)
}

func (a *fakeArchive) serveBlob(w http.ResponseWriter, key string, from map[string][]byte) {
	a.mu.Lock()
	body, ok := from[key]
	a.fetched = append(a.fetched, key)
	a.mu.Unlock()
	if !ok {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(body)
}

func (a *fakeArchive) planInputs() []planInput {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]planInput(nil), a.requests...)
}

func (a *fakeArchive) downloads() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.fetched...)
}

func subscriptionRow(t *testing.T, seq int64, did, rkey string) archiveRow {
	t.Helper()
	return archiveRow{
		Seq: seq, Kind: archiveKindCreate, Collection: "blue.morgen.feed.subscription",
		DID: did, Rkey: rkey, Rev: "3rrrrrrrrrrr2",
		Payload: mustCBOR(t, map[string]any{
			"$type":     "blue.morgen.feed.subscription",
			"title":     "Example Publication",
			"createdAt": "2026-03-01T00:00:00Z",
		}),
	}
}

func TestBootstrap_MirrorsTheArchiveThenTailsFromTheSealedTip(t *testing.T) {
	rec := &trace{}
	archive := &fakeArchive{
		plans: []planOutput{{
			SealedTipSeq:      50,
			PlannedThroughSeq: 50,
			Segments: []planSegment{{
				Name: testSegment, Mode: modeBlocks, MinSeq: 1, MaxSeq: 50,
				Blocks: []blockRange{{First: 0, Last: 0}},
			}},
		}},
		blocks: map[string][]byte{},
	}
	archive.blocks[blockKey(testSegment, 0)] = buildBlock(t, []archiveRow{subscriptionRow(t, 10, testDID, "3aaaaaaaaaaa2")})

	srv := newJetstreamServer(t, rec)
	srv.archive = archive
	store := newFakeStore(rec)
	h := startConsumerWith(t, srv, rec, store, &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "upsert:3aaaaaaaaaaa2", "connect:0")
	if _, ok := store.recordAt(testDID, "blue.morgen.feed.subscription", "3aaaaaaaaaaa2"); !ok {
		t.Error("archived record was not mirrored")
	}
	if got := srv.query(t, 0).Get("cursor"); got != "50" {
		t.Errorf("live cursor = %q, want the sealed tip 50", got)
	}
	row := h.store.cursorRow()
	if row == nil || row.Seq != 50 || row.BootstrapTipSeq != nil {
		t.Errorf("cursor row after cutover = %+v", row)
	}
}

func TestBootstrap_PagesUntilTheSealedTip(t *testing.T) {
	rec := &trace{}
	second := "seg_0000000001.jss"
	archive := &fakeArchive{
		plans: []planOutput{
			{
				SealedTipSeq: 100, PlannedThroughSeq: 60,
				Segments: []planSegment{{Name: testSegment, Mode: modeBlocks, MinSeq: 1, MaxSeq: 60, Blocks: []blockRange{{First: 0, Last: 0}}}},
			},
			{
				SealedTipSeq: 100, PlannedThroughSeq: 100,
				Segments: []planSegment{{Name: second, Mode: modeBlocks, MinSeq: 61, MaxSeq: 100, Blocks: []blockRange{{First: 0, Last: 0}}}},
			},
		},
		blocks: map[string][]byte{},
	}
	archive.blocks[blockKey(testSegment, 0)] = buildBlock(t, []archiveRow{subscriptionRow(t, 20, testDID, "3aaaaaaaaaaa2")})
	archive.blocks[blockKey(second, 0)] = buildBlock(t, []archiveRow{subscriptionRow(t, 80, otherDID, "3bbbbbbbbbbb2")})

	srv := newJetstreamServer(t, rec)
	srv.archive = archive
	startConsumerWith(t, srv, rec, newFakeStore(rec), &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "upsert:3aaaaaaaaaaa2", "upsert:3bbbbbbbbbbb2", "connect:0")
	inputs := archive.planInputs()
	if len(inputs) < 2 {
		t.Fatalf("plan calls = %d, want at least 2", len(inputs))
	}
	// The second page pins beforeSeq to the tip the first page reported, so the range never floats.
	if inputs[1].AfterSeq != 60 || inputs[1].BeforeSeq != 100 {
		t.Errorf("second plan = %+v, want afterSeq 60 beforeSeq 100", inputs[1])
	}
	if got := srv.query(t, 0).Get("cursor"); got != "100" {
		t.Errorf("live cursor = %q, want 100", got)
	}
}

func TestBootstrap_ResumesFromItsPersistedContinuation(t *testing.T) {
	rec := &trace{}
	tip, through := int64(100), int64(60)
	cursors := &fakeCursors{}
	cursors.set(db.GetDiscoverIngestCursorRow{
		Seq: 5, BootstrapTipSeq: &tip, BootstrapThroughSeq: &through, UpdatedAt: "2026-08-01T00:00:00Z",
	})

	archive := &fakeArchive{
		plans: []planOutput{{
			SealedTipSeq: 100, PlannedThroughSeq: 100,
			Segments: []planSegment{{Name: testSegment, Mode: modeBlocks, MinSeq: 61, MaxSeq: 100, Blocks: []blockRange{{First: 3, Last: 3}}}},
		}},
		blocks: map[string][]byte{},
	}
	archive.blocks[blockKey(testSegment, 3)] = buildBlock(t, []archiveRow{subscriptionRow(t, 80, testDID, "3aaaaaaaaaaa2")})

	srv := newJetstreamServer(t, rec)
	srv.archive = archive
	startConsumerWith(t, srv, rec, newFakeStore(rec), cursors, &fakeFetcher{rec: rec})

	rec.await(t, "upsert:3aaaaaaaaaaa2", "connect:0")
	inputs := archive.planInputs()
	// The tip is already pinned, so the resume must not re-discover it from the live seq.
	if inputs[0].AfterSeq != 60 || inputs[0].BeforeSeq != 100 {
		t.Errorf("resume plan = %+v, want afterSeq 60 beforeSeq 100", inputs[0])
	}
	if got := archive.downloads(); len(got) != 1 || got[0] != blockKey(testSegment, 3) {
		t.Errorf("downloads = %v, want only the unconsumed block", got)
	}
}

func TestBootstrap_SegmentModeWalksTheWholeFile(t *testing.T) {
	rec := &trace{}
	archive := &fakeArchive{
		plans: []planOutput{{
			SealedTipSeq: 50, PlannedThroughSeq: 50,
			Segments: []planSegment{{Name: testSegment, Mode: modeSegment, MinSeq: 1, MaxSeq: 50}},
		}},
		segments: map[string][]byte{},
	}
	archive.segments[testSegment] = buildSegment(t, [][]byte{
		buildBlock(t, []archiveRow{subscriptionRow(t, 10, testDID, "3aaaaaaaaaaa2")}),
		buildBlock(t, []archiveRow{subscriptionRow(t, 20, otherDID, "3bbbbbbbbbbb2")}),
	})

	srv := newJetstreamServer(t, rec)
	srv.archive = archive
	startConsumerWith(t, srv, rec, newFakeStore(rec), &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "upsert:3aaaaaaaaaaa2", "upsert:3bbbbbbbbbbb2")
}

// The planner has no false negatives but plenty of false positives, so the exact predicate has to run client-side.
func TestBootstrap_DropsRowsOutsideTheWindowAndOffNetworkCollections(t *testing.T) {
	rec := &trace{}
	tip, through := int64(100), int64(50)
	cursors := &fakeCursors{}
	cursors.set(db.GetDiscoverIngestCursorRow{Seq: 0, BootstrapTipSeq: &tip, BootstrapThroughSeq: &through, UpdatedAt: "2026-08-01T00:00:00Z"})

	offNetwork := subscriptionRow(t, 70, testDID, "3offnetworka2")
	offNetwork.Collection = "app.bsky.feed.post"

	archive := &fakeArchive{
		plans: []planOutput{{
			SealedTipSeq: 100, PlannedThroughSeq: 100,
			Segments: []planSegment{{Name: testSegment, Mode: modeBlocks, MinSeq: 1, MaxSeq: 200, Blocks: []blockRange{{First: 0, Last: 0}}}},
		}},
		blocks: map[string][]byte{},
	}
	archive.blocks[blockKey(testSegment, 0)] = buildBlock(t, []archiveRow{
		subscriptionRow(t, 40, testDID, "3belowwindow2"),
		offNetwork,
		subscriptionRow(t, 75, testDID, "3inwindowaaa2"),
		subscriptionRow(t, 150, testDID, "3abovewindow2"),
	})

	srv := newJetstreamServer(t, rec)
	srv.archive = archive
	store := newFakeStore(rec)
	startConsumerWith(t, srv, rec, store, cursors, &fakeFetcher{rec: rec})

	rec.await(t, "upsert:3inwindowaaa2", "connect:0")
	rec.refute(t, "upsert:3belowwindow2")
	rec.refute(t, "upsert:3abovewindow2")
	rec.refute(t, "upsert:3offnetworka2")
	if store.recordCount() != 1 {
		t.Errorf("records = %d, want only the in-window tracked row", store.recordCount())
	}
}

// Account markers ride inline through the archive, so a repo deleted before the tip must not survive a bootstrap.
func TestBootstrap_AppliesInlineAccountDeletion(t *testing.T) {
	rec := &trace{}
	archive := &fakeArchive{
		plans: []planOutput{{
			SealedTipSeq: 50, PlannedThroughSeq: 50,
			Segments: []planSegment{{Name: testSegment, Mode: modeBlocks, MinSeq: 1, MaxSeq: 50, Blocks: []blockRange{{First: 0, Last: 0}}}},
		}},
		blocks: map[string][]byte{},
	}
	archive.blocks[blockKey(testSegment, 0)] = buildBlock(t, []archiveRow{
		subscriptionRow(t, 10, testDID, "3aaaaaaaaaaa2"),
		{
			Seq: 11, Kind: archiveKindAccount, DID: testDID,
			Payload: mustCBOR(t, map[string]any{"did": testDID, "active": false, "status": "deleted"}),
		},
	})

	srv := newJetstreamServer(t, rec)
	srv.archive = archive
	store := newFakeStore(rec)
	startConsumerWith(t, srv, rec, store, &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "upsert:3aaaaaaaaaaa2", "purge:"+testDID)
	if store.recordCount() != 0 {
		t.Errorf("records = %d, want the deleted repo purged", store.recordCount())
	}
}

func TestBootstrap_EmptyArchiveTailsFromTheLiveTip(t *testing.T) {
	rec := &trace{}
	srv := newJetstreamServer(t, rec)
	startConsumerWith(t, srv, rec, newFakeStore(rec), &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "connect:0")
	if got := srv.query(t, 0).Get("cursor"); got != "" {
		t.Errorf("cursor = %q, want none so the tail starts at the live tip", got)
	}
}

func TestBootstrap_PlanCarriesEveryTrackedCollection(t *testing.T) {
	rec := &trace{}
	archive := &fakeArchive{plans: []planOutput{{SealedTipSeq: 0, PlannedThroughSeq: 0}}}
	srv := newJetstreamServer(t, rec)
	srv.archive = archive
	startConsumerWith(t, srv, rec, newFakeStore(rec), &fakeCursors{}, &fakeFetcher{rec: rec})

	rec.await(t, "connect:0")
	inputs := archive.planInputs()
	if len(inputs) == 0 {
		t.Fatal("no plan call")
	}
	if len(inputs[0].Collections) != len(Collections) {
		t.Fatalf("collections = %v", inputs[0].Collections)
	}
	// kinds is deliberately unset: omitting it admits the DID-level marker blocks a collection-filtered plan would otherwise skip.
	raw, _ := json.Marshal(inputs[0])
	if strings.Contains(string(raw), `"kinds"`) {
		t.Errorf("plan input carried kinds: %s", raw)
	}
}
