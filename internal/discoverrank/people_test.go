package discoverrank

import (
	"testing"
	"time"
)

func rankPeople(candidates []PersonCandidate, excluded map[string]struct{}, max int, seed int64) []PersonSuggestion {
	return RankPeople(candidates, excluded, max, seed, testNow)
}

func TestRankPeople_ExcludesPeopleWithNoReaderNetworkActivity(t *testing.T) {
	candidates := []PersonCandidate{
		{DID: "did:plc:ghost", BlueskyFollow: true}, // graph proximity but zero activity
		{DID: "did:plc:active", BlueskyFollow: true, Activity: []Signal{subscribeSignal()}},
	}

	got := rankPeople(candidates, nil, 8, testSeed)

	if len(got) != 1 || got[0].DID != "did:plc:active" {
		t.Fatalf("got = %+v, want only the person with reader-network activity", got)
	}
}

func TestRankPeople_ExplicitEligibilityAllowsZeroScoredActivity(t *testing.T) {
	candidates := []PersonCandidate{
		{DID: "did:plc:preview-only", BlueskyFollow: true, Eligible: true},
	}

	got := rankPeople(candidates, nil, 8, testSeed)

	if len(got) != 1 || got[0].DID != "did:plc:preview-only" {
		t.Fatalf("got = %+v, want explicitly eligible person with zero scored activity", got)
	}
}

func TestRankPeople_BlueskyOnlyCandidateProducesBlueskyReason(t *testing.T) {
	candidates := []PersonCandidate{
		{DID: "did:plc:alice", BlueskyFollow: true, Activity: []Signal{subscribeSignal()}},
	}

	got := rankPeople(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("got = %+v, want 1", got)
	}
	r := got[0].Reason
	if !r.BlueskyFollow || r.TangledFollow || r.FollowedByDID != "" {
		t.Errorf("Reason = %+v, want only BlueskyFollow set", r)
	}
}

func TestRankPeople_TangledOnlyCandidateProducesTangledReason(t *testing.T) {
	candidates := []PersonCandidate{
		{DID: "did:plc:bob", TangledFollow: true, Activity: []Signal{subscribeSignal()}},
	}

	got := rankPeople(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("got = %+v, want 1", got)
	}
	r := got[0].Reason
	if !r.TangledFollow || r.BlueskyFollow || r.FollowedByDID != "" {
		t.Errorf("Reason = %+v, want only TangledFollow set", r)
	}
}

func TestRankPeople_OneHopCandidateProducesFollowedByReason(t *testing.T) {
	candidates := []PersonCandidate{
		{DID: "did:plc:carol", FollowedByDID: "did:plc:alice", Activity: []Signal{subscribeSignal()}},
	}

	got := rankPeople(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("got = %+v, want 1", got)
	}
	r := got[0].Reason
	if r.FollowedByDID != "did:plc:alice" || r.BlueskyFollow || r.TangledFollow {
		t.Errorf("Reason = %+v, want only FollowedByDID set", r)
	}
}

func TestRankPeople_TasteOverlapOutranksStrangerAtEqualActivity(t *testing.T) {
	candidates := []PersonCandidate{
		{DID: "did:plc:stranger", BlueskyFollow: true, Activity: []Signal{subscribeSignal()}},
		{DID: "did:plc:overlaps", BlueskyFollow: true, Activity: []Signal{subscribeSignal()}, SharedSourceCount: 4},
	}

	got := rankPeople(candidates, nil, 8, testSeed)

	if len(got) != 2 || got[0].DID != "did:plc:overlaps" {
		t.Fatalf("got = %+v, want the 4-shared-source person ranked first at equal activity", got)
	}
}

func TestRankPeople_SeededRotation_NearTieDistinctScoresVary(t *testing.T) {
	// Continuous activity scores would freeze ranking without banded shuffling, same as TestRank's version.
	candidates := []PersonCandidate{
		{DID: "did:plc:alice", BlueskyFollow: true, Activity: []Signal{{Kind: SignalSubscribe, At: testNow}}},
		{DID: "did:plc:bob", BlueskyFollow: true, Activity: []Signal{{Kind: SignalSubscribe, At: testNow.Add(-5 * 24 * time.Hour)}}},
	}

	seen := map[string]bool{}
	for seed := int64(1); seed <= 28; seed++ {
		got := rankPeople(candidates, nil, 8, seed)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		seen[got[0].DID] = true
	}
	if !seen["did:plc:alice"] || !seen["did:plc:bob"] {
		t.Errorf("28 seeds never rotated a near-tie pair (leaders seen: %v)", seen)
	}
}

// --- Save-privacy invariant. SPEC <saving-sharing>: saves are anonymous-batch-only, never used for personal eligibility or scoring. ---

func TestRankPeople_AllSaveActivityFailsEligibility(t *testing.T) {
	candidates := []PersonCandidate{
		{DID: "did:plc:saver", BlueskyFollow: true, Activity: []Signal{{Kind: SignalSave, At: testNow}}},
	}

	got := rankPeople(candidates, nil, 8, testSeed)

	if len(got) != 0 {
		t.Errorf("got = %+v, want no suggestion for a person whose only activity is a save", got)
	}
}

func TestRankPeople_SavesInMixedActivityAddZeroToScore(t *testing.T) {
	withSave := PersonCandidate{
		DID:           "did:plc:with-save",
		BlueskyFollow: true,
		Activity:      []Signal{subscribeSignal(), {Kind: SignalSave, At: testNow}},
	}
	subscribeOnly := PersonCandidate{
		DID:           "did:plc:subscribe-only",
		BlueskyFollow: true,
		Activity:      []Signal{subscribeSignal()},
	}
	// Scores strictly between subscribeOnly's and what an unfiltered fresh save would add on top of it: reveals whether the save moved the score.
	sentinel := PersonCandidate{
		DID:           "did:plc:sentinel",
		BlueskyFollow: true,
		Activity:      []Signal{{Kind: SignalSubscribe, At: testNow}},
	}

	gotWith := rankPeople([]PersonCandidate{withSave, sentinel}, nil, 8, testSeed)
	gotWithout := rankPeople([]PersonCandidate{subscribeOnly, sentinel}, nil, 8, testSeed)

	if len(gotWith) != 2 || len(gotWithout) != 2 {
		t.Fatalf("gotWith = %+v, gotWithout = %+v", gotWith, gotWithout)
	}
	if gotWithout[0].DID != "did:plc:sentinel" {
		t.Fatalf("gotWithout = %+v, want the sentinel first as a sanity baseline", gotWithout)
	}
	if gotWith[0].DID != "did:plc:sentinel" {
		t.Errorf("gotWith = %+v, want the sentinel still first (a save must add zero to the person score)", gotWith)
	}
}

func TestRankPeople_ExcludesGivenDIDs(t *testing.T) {
	candidates := []PersonCandidate{
		{DID: "did:plc:alice", BlueskyFollow: true, Activity: []Signal{subscribeSignal()}},
		{DID: "did:plc:bob", BlueskyFollow: true, Activity: []Signal{subscribeSignal()}},
	}
	excluded := map[string]struct{}{"did:plc:alice": {}}

	got := rankPeople(candidates, excluded, 8, testSeed)

	if len(got) != 1 || got[0].DID != "did:plc:bob" {
		t.Fatalf("got = %+v, want only bob", got)
	}
}

func TestRankPeople_CapsAtMax(t *testing.T) {
	var candidates []PersonCandidate
	for i := 0; i < 10; i++ {
		candidates = append(candidates, PersonCandidate{
			DID:           "did:plc:p" + string(rune('a'+i)),
			BlueskyFollow: true,
			Activity:      []Signal{subscribeSignal()},
		})
	}

	got := rankPeople(candidates, nil, 8, testSeed)

	if len(got) != 8 {
		t.Fatalf("len = %d, want 8", len(got))
	}
}

func TestRankPeople_MoreActivityOutranksLessAtEqualOverlap(t *testing.T) {
	candidates := []PersonCandidate{
		{DID: "did:plc:quiet", BlueskyFollow: true, Activity: []Signal{subscribeSignal()}},
		{DID: "did:plc:prolific", BlueskyFollow: true, Activity: []Signal{
			subscribeSignal(), subscribeSignal(), subscribeSignal(),
		}},
	}

	got := rankPeople(candidates, nil, 8, testSeed)

	if len(got) != 2 || got[0].DID != "did:plc:prolific" {
		t.Fatalf("got = %+v, want the more-active person ranked first", got)
	}
}

func TestRankPeople_SameSeedSameDay_DeterministicOrder(t *testing.T) {
	var candidates []PersonCandidate
	for i := 0; i < 6; i++ {
		candidates = append(candidates, PersonCandidate{
			DID:           "did:plc:p" + string(rune('a'+i)),
			BlueskyFollow: true,
			Activity:      []Signal{subscribeSignal()},
		})
	}

	first := rankPeople(candidates, nil, 8, testSeed)
	second := rankPeople(candidates, nil, 8, testSeed)

	if len(first) != len(second) {
		t.Fatalf("len(first) = %d, len(second) = %d", len(first), len(second))
	}
	for i := range first {
		if first[i].DID != second[i].DID {
			t.Errorf("order differs at [%d]: %q != %q", i, first[i].DID, second[i].DID)
		}
	}
}

func TestRankPeople_DifferentSeed_CanReorderTiedCandidates(t *testing.T) {
	var candidates []PersonCandidate
	for i := 0; i < 6; i++ {
		candidates = append(candidates, PersonCandidate{
			DID:           "did:plc:p" + string(rune('a'+i)),
			BlueskyFollow: true,
			Activity:      []Signal{subscribeSignal()},
		})
	}

	a := rankPeople(candidates, nil, 8, 1)
	b := rankPeople(candidates, nil, 8, 2)

	same := true
	for i := range a {
		if a[i].DID != b[i].DID {
			same = false
			break
		}
	}
	if same {
		t.Error("expected a different seed to reorder tied candidates at least once")
	}
}
