package discoverrank

import (
	"testing"
	"time"
)

func TestRankTrending_QualityBar_TwoDistinctReposNeverTrends(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://two-repos", Kind: "rss", Followers: []Follower{
			followerAt("did:plc:repo1", TierStrong, SignalSubscribe, testNow),
			followerAt("did:plc:repo2", TierStrong, SignalSubscribe, testNow),
		}},
		{Key: "https://three-repos", Kind: "rss", Followers: []Follower{
			followerAt("did:plc:repo1", TierStrong, SignalSubscribe, testNow),
			followerAt("did:plc:repo2", TierStrong, SignalSubscribe, testNow),
			followerAt("did:plc:repo3", TierStrong, SignalSubscribe, testNow),
		}},
	}

	got := RankTrending(candidates, nil, 8, testSeed, testNow)

	if len(got) != 1 || got[0].Key != "https://three-repos" {
		t.Fatalf("got = %+v, want only https://three-repos (2 distinct repos must never trend)", got)
	}
}

func TestRankTrending_ExcludesAlreadySubscribedAndHidden(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://subscribed", Kind: "rss", Followers: []Follower{
			followerAt("did:plc:repo1", TierStrong, SignalSubscribe, testNow),
			followerAt("did:plc:repo2", TierStrong, SignalSubscribe, testNow),
			followerAt("did:plc:repo3", TierStrong, SignalSubscribe, testNow),
		}},
		{Key: "https://fresh", Kind: "rss", Followers: []Follower{
			followerAt("did:plc:repo1", TierStrong, SignalSubscribe, testNow),
			followerAt("did:plc:repo2", TierStrong, SignalSubscribe, testNow),
			followerAt("did:plc:repo3", TierStrong, SignalSubscribe, testNow),
		}},
	}
	excluded := map[string]struct{}{"https://subscribed": {}}

	got := RankTrending(candidates, excluded, 8, testSeed, testNow)

	if len(got) != 1 || got[0].Key != "https://fresh" {
		t.Fatalf("got = %+v, want only https://fresh", got)
	}
}

func TestRankTrending_CapsAtMax(t *testing.T) {
	var candidates []Candidate
	for i := 0; i < 10; i++ {
		candidates = append(candidates, Candidate{
			Key:  string(rune('a'+i)) + "-source",
			Kind: "rss",
			Followers: []Follower{
				followerAt("did:plc:repo1", TierStrong, SignalSubscribe, testNow),
				followerAt("did:plc:repo2", TierStrong, SignalSubscribe, testNow),
				followerAt("did:plc:repo3", TierStrong, SignalSubscribe, testNow),
			},
		})
	}

	got := RankTrending(candidates, nil, 8, testSeed, testNow)

	if len(got) != 8 {
		t.Fatalf("len = %d, want 8", len(got))
	}
}

func TestRankTrending_NoTrustTerm_ManyRepoSharesCanOutrankFewRepoSubscribes(t *testing.T) {
	manySharing := Candidate{Key: "https://many-share", Kind: "rss", Followers: []Follower{
		followerAt("did:plc:repo1", TierStrong, SignalShare, testNow),
		followerAt("did:plc:repo2", TierStrong, SignalShare, testNow),
		followerAt("did:plc:repo3", TierStrong, SignalShare, testNow),
		followerAt("did:plc:repo4", TierStrong, SignalShare, testNow),
		followerAt("did:plc:repo5", TierStrong, SignalShare, testNow),
	}}
	fewSubscribing := Candidate{Key: "https://few-subscribe", Kind: "rss", Followers: []Follower{
		followerAt("did:plc:repo6", TierStrong, SignalSubscribe, testOldAt),
		followerAt("did:plc:repo7", TierStrong, SignalSubscribe, testOldAt),
		followerAt("did:plc:repo8", TierStrong, SignalSubscribe, testOldAt),
	}}

	got := RankTrending([]Candidate{fewSubscribing, manySharing}, nil, 8, testSeed, testNow)

	if len(got) != 2 || got[0].Key != "https://many-share" {
		t.Fatalf("got = %+v, want https://many-share first (5*0.7 share weight > 3*1.0 subscribe weight)", got)
	}
}

func TestRankTrending_OneSignalPerRepoPerSource_StrongestWins(t *testing.T) {
	// Same repo subscribes and shares: must count once toward the quality bar, not six times.
	followers := []Follower{
		followerAt("did:plc:repo1", TierStrong, SignalSubscribe, testOldAt),
		followerAt("did:plc:repo2", TierStrong, SignalSubscribe, testOldAt),
	}
	for i := 0; i < 5; i++ {
		followers = append(followers, followerAt("did:plc:repo1", TierStrong, SignalShare, testNow))
	}
	candidates := []Candidate{{Key: "https://mixed", Kind: "rss", Followers: followers}}

	got := RankTrending(candidates, nil, 8, testSeed, testNow)

	if len(got) != 0 {
		t.Fatalf("got = %+v, want none (only 2 distinct repos despite 7 raw signals)", got)
	}
}

func TestRankTrending_SeededRotation_SameSeedStableDifferentSeedVaries(t *testing.T) {
	var candidates []Candidate
	for i := 0; i < 6; i++ {
		candidates = append(candidates, Candidate{
			Key:  string(rune('a'+i)) + "-source",
			Kind: "rss",
			Followers: []Follower{
				followerAt("did:plc:repo1", TierStrong, SignalSubscribe, testOldAt),
				followerAt("did:plc:repo2", TierStrong, SignalSubscribe, testOldAt),
				followerAt("did:plc:repo3", TierStrong, SignalSubscribe, testOldAt),
			},
		})
	}
	seedA := int64(1)
	seedB := int64(2)

	first := RankTrending(candidates, nil, 8, seedA, testNow)
	second := RankTrending(candidates, nil, 8, seedA, testNow)
	other := RankTrending(candidates, nil, 8, seedB, testNow)

	if !sameOrder(keysOf(first), keysOf(second)) {
		t.Errorf("same seed produced different order:\n%v\n%v", keysOf(first), keysOf(second))
	}
	if sameOrder(keysOf(first), keysOf(other)) {
		t.Errorf("different seeds produced identical order: %v", keysOf(first))
	}
}

func TestRankTrending_EmptyInputYieldsEmptyOutput(t *testing.T) {
	got := RankTrending(nil, nil, 8, testSeed, testNow)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

// --- Save signals. SPEC <discovery> save-privacy invariant: saves score only on the trending path. ---

func TestRankTrending_SavesDecayTheSameWayAsShares(t *testing.T) {
	// Re-scoped from the personal-ranking suite: Rank now excludes save signals entirely, but RankTrending still scores them (SaveBaseWeight/reactionDecay), so the decay behavior belongs here.
	fresh := Candidate{Key: "https://fresh", Kind: "rss", Followers: []Follower{
		followerAt("did:plc:repo1", TierStrong, SignalSave, testNow),
		followerAt("did:plc:repo2", TierStrong, SignalSave, testNow),
		followerAt("did:plc:repo3", TierStrong, SignalSave, testNow),
	}}
	old := Candidate{Key: "https://old", Kind: "rss", Followers: []Follower{
		followerAt("did:plc:repo1", TierStrong, SignalSave, testNow.Add(-60*24*time.Hour)),
		followerAt("did:plc:repo2", TierStrong, SignalSave, testNow.Add(-60*24*time.Hour)),
		followerAt("did:plc:repo3", TierStrong, SignalSave, testNow.Add(-60*24*time.Hour)),
	}}

	got := RankTrending([]Candidate{fresh, old}, nil, 8, testSeed, testNow)
	if len(got) != 2 || got[0].Key != "https://fresh" {
		t.Fatalf("got = %+v, want the fresh save ranked above the older one", got)
	}
}

func TestRankTrending_SignalOrdering_OtherKindsBeatSave(t *testing.T) {
	// The "vs save" cases dropped from TestRank_SignalOrdering_PairwiseAtEqualRecency (personal Rank now excludes save-only candidates rather than merely outranking them) still hold on the trending path, where save signals are scored.
	cases := []struct {
		name      string
		strongest SignalKind
	}{
		{"author beats save", SignalAuthor},
		{"subscribe beats save", SignalSubscribe},
		{"share beats save", SignalShare},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidates := []Candidate{
				{Key: "https://strongest", Kind: "rss", Followers: []Follower{
					followerAt("did:plc:repo1", TierStrong, tc.strongest, testNow),
					followerAt("did:plc:repo2", TierStrong, tc.strongest, testNow),
					followerAt("did:plc:repo3", TierStrong, tc.strongest, testNow),
				}},
				{Key: "https://weakest", Kind: "rss", Followers: []Follower{
					followerAt("did:plc:repo4", TierStrong, SignalSave, testNow),
					followerAt("did:plc:repo5", TierStrong, SignalSave, testNow),
					followerAt("did:plc:repo6", TierStrong, SignalSave, testNow),
				}},
			}
			got := RankTrending(candidates, nil, 8, testSeed, testNow)
			if len(got) != 2 || got[0].Key != "https://strongest" {
				t.Fatalf("got = %+v, want https://strongest ranked first", got)
			}
		})
	}
}
