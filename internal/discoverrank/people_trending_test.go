package discoverrank

import "testing"

func rankPeopleTrending(candidates []TrendingPersonCandidate, excluded map[string]struct{}, max int, seed int64) []PersonSuggestion {
	return RankPeopleTrending(candidates, excluded, max, seed, testNow)
}

func eligibleTrendingCandidate(did string, followers ...string) TrendingPersonCandidate {
	return TrendingPersonCandidate{DID: did, FollowerDIDs: followers, Eligible: true}
}

func TestRankPeopleTrending_QualityBar_TwoDistinctFollowersNeverTrends(t *testing.T) {
	candidates := []TrendingPersonCandidate{
		eligibleTrendingCandidate("did:plc:two-followers", "did:plc:repo1", "did:plc:repo2"),
		eligibleTrendingCandidate("did:plc:three-followers", "did:plc:repo1", "did:plc:repo2", "did:plc:repo3"),
	}

	got := rankPeopleTrending(candidates, nil, 8, testSeed)

	if len(got) != 1 || got[0].DID != "did:plc:three-followers" {
		t.Fatalf("got = %+v, want only did:plc:three-followers (2 distinct followers must never trend)", got)
	}
}

func TestRankPeopleTrending_ExcludesIneligibleCandidates(t *testing.T) {
	candidates := []TrendingPersonCandidate{
		{DID: "did:plc:no-presence", FollowerDIDs: []string{"did:plc:repo1", "did:plc:repo2", "did:plc:repo3"}, Eligible: false},
		eligibleTrendingCandidate("did:plc:has-presence", "did:plc:repo1", "did:plc:repo2", "did:plc:repo3"),
	}

	got := rankPeopleTrending(candidates, nil, 8, testSeed)

	if len(got) != 1 || got[0].DID != "did:plc:has-presence" {
		t.Fatalf("got = %+v, want only the eligible (reader-network-present) candidate", got)
	}
}

func TestRankPeopleTrending_ExcludesAlreadyFollowedAndHiddenDIDs(t *testing.T) {
	candidates := []TrendingPersonCandidate{
		eligibleTrendingCandidate("did:plc:followed", "did:plc:repo1", "did:plc:repo2", "did:plc:repo3"),
		eligibleTrendingCandidate("did:plc:fresh", "did:plc:repo1", "did:plc:repo2", "did:plc:repo3"),
	}
	excluded := map[string]struct{}{"did:plc:followed": {}}

	got := rankPeopleTrending(candidates, excluded, 8, testSeed)

	if len(got) != 1 || got[0].DID != "did:plc:fresh" {
		t.Fatalf("got = %+v, want only did:plc:fresh", got)
	}
}

func TestRankPeopleTrending_MoreFollowersOutranksFewer(t *testing.T) {
	candidates := []TrendingPersonCandidate{
		eligibleTrendingCandidate("did:plc:three", "did:plc:repo1", "did:plc:repo2", "did:plc:repo3"),
		eligibleTrendingCandidate("did:plc:five", "did:plc:repo1", "did:plc:repo2", "did:plc:repo3", "did:plc:repo4", "did:plc:repo5"),
	}

	got := rankPeopleTrending(candidates, nil, 8, testSeed)

	if len(got) != 2 || got[0].DID != "did:plc:five" {
		t.Fatalf("got = %+v, want did:plc:five ranked first (more distinct followers)", got)
	}
}

func TestRankPeopleTrending_DuplicateFollowerDIDsCountOnceForBothBarAndScore(t *testing.T) {
	// Duplicates can occur before table-level dedup (e.g. a follow via both blue.morgen and app.skyreader); must not inflate past one distinct contribution.
	candidates := []TrendingPersonCandidate{
		{DID: "did:plc:dup", FollowerDIDs: []string{"did:plc:repo1", "did:plc:repo1", "did:plc:repo2"}, Eligible: true},
	}

	got := rankPeopleTrending(candidates, nil, 8, testSeed)

	if len(got) != 0 {
		t.Fatalf("got = %+v, want none (only 2 distinct followers despite 3 raw entries)", got)
	}
}

func TestRankPeopleTrending_DecayedShareActivityBreaksATie(t *testing.T) {
	base := []string{"did:plc:repo1", "did:plc:repo2", "did:plc:repo3"}
	candidates := []TrendingPersonCandidate{
		{DID: "did:plc:quiet", FollowerDIDs: base, Eligible: true},
		{DID: "did:plc:active-sharer", FollowerDIDs: base, Eligible: true, ShareActivity: []Signal{
			{Kind: SignalShare, At: testNow},
			{Kind: SignalShare, At: testNow},
		}},
	}

	got := rankPeopleTrending(candidates, nil, 8, testSeed)

	if len(got) != 2 || got[0].DID != "did:plc:active-sharer" {
		t.Fatalf("got = %+v, want did:plc:active-sharer ranked first (equal followers, more decayed share activity)", got)
	}
}

func TestRankPeopleTrending_NoReasonCarried(t *testing.T) {
	// Mirrors RankTrending: trending has no per-user contributor to credit.
	candidates := []TrendingPersonCandidate{
		eligibleTrendingCandidate("did:plc:alice", "did:plc:repo1", "did:plc:repo2", "did:plc:repo3"),
	}

	got := rankPeopleTrending(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("got = %+v, want 1", got)
	}
	zero := PersonReason{}
	if got[0].Reason != zero {
		t.Errorf("Reason = %+v, want zero value (no per-user reason for trending)", got[0].Reason)
	}
}

func TestRankPeopleTrending_CapsAtMax(t *testing.T) {
	var candidates []TrendingPersonCandidate
	for i := 0; i < 10; i++ {
		candidates = append(candidates, eligibleTrendingCandidate(
			"did:plc:p"+string(rune('a'+i)), "did:plc:repo1", "did:plc:repo2", "did:plc:repo3",
		))
	}

	got := rankPeopleTrending(candidates, nil, 8, testSeed)

	if len(got) != 8 {
		t.Fatalf("len = %d, want 8", len(got))
	}
}

func TestRankPeopleTrending_SeededRotation_SameSeedStableDifferentSeedVaries(t *testing.T) {
	var candidates []TrendingPersonCandidate
	for i := 0; i < 6; i++ {
		candidates = append(candidates, eligibleTrendingCandidate(
			"did:plc:p"+string(rune('a'+i)), "did:plc:repo1", "did:plc:repo2", "did:plc:repo3",
		))
	}
	seedA := int64(1)
	seedB := int64(2)

	first := rankPeopleTrending(candidates, nil, 8, seedA)
	second := rankPeopleTrending(candidates, nil, 8, seedA)
	other := rankPeopleTrending(candidates, nil, 8, seedB)

	if !sameOrder(didsOf(first), didsOf(second)) {
		t.Errorf("same seed produced different order:\n%v\n%v", didsOf(first), didsOf(second))
	}
	if sameOrder(didsOf(first), didsOf(other)) {
		t.Errorf("different seeds produced identical order: %v", didsOf(first))
	}
}

func TestRankPeopleTrending_EmptyInputYieldsEmptyOutput(t *testing.T) {
	got := rankPeopleTrending(nil, nil, 8, testSeed)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func didsOf(suggestions []PersonSuggestion) []string {
	dids := make([]string, len(suggestions))
	for i, s := range suggestions {
		dids[i] = s.DID
	}
	return dids
}
