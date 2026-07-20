package discoverrank

import (
	"testing"
	"time"
)

const testSeed int64 = 42

// testOldAt sits 100 standing-signal half-lives back, so its recency-lean bonus is negligible (<0.002 of base weight) and old test followers behave like flat-weight ones.
var testNow = mustTime("2026-07-09T12:00:00Z")
var testOldAt = testNow.Add(-100 * standingLeanHalfLife)

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func rank(candidates []Candidate, excluded map[string]struct{}, max int, seed int64) []Suggestion {
	return Rank(candidates, excluded, max, seed, testNow)
}

func subscribeSignal() Signal { return Signal{Kind: SignalSubscribe, At: testOldAt} }

func strongFollowers(dids ...string) []Follower {
	out := make([]Follower, len(dids))
	for i, d := range dids {
		out[i] = Follower{DID: d, Tier: TierStrong, Signal: subscribeSignal()}
	}
	return out
}

func weakFollowers(network Network, dids ...string) []Follower {
	out := make([]Follower, len(dids))
	for i, d := range dids {
		out[i] = Follower{DID: d, Tier: TierWeak, Network: network, Signal: subscribeSignal()}
	}
	return out
}

func TestRank_OrdersByDistinctFollowerCountDescending(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://a", Kind: "rss", Followers: strongFollowers("did:plc:alice")},
		{Key: "https://b", Kind: "rss", Followers: strongFollowers("did:plc:alice", "did:plc:bob", "did:plc:carol")},
		{Key: "https://c", Kind: "rss", Followers: strongFollowers("did:plc:alice", "did:plc:bob")},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	wantOrder := []string{"https://b", "https://c", "https://a"}
	for i, key := range wantOrder {
		if got[i].Key != key {
			t.Errorf("got[%d].Key = %q, want %q", i, got[i].Key, key)
		}
	}
}

func TestRank_ExcludesAlreadySubscribed(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://a", Kind: "rss", Followers: strongFollowers("did:plc:alice")},
		{Key: "https://b", Kind: "rss", Followers: strongFollowers("did:plc:alice")},
	}
	already := map[string]struct{}{"https://a": {}}

	got := rank(candidates, already, 8, testSeed)

	if len(got) != 1 || got[0].Key != "https://b" {
		t.Fatalf("got = %+v, want only https://b", got)
	}
}

func TestRank_CapsAtMax(t *testing.T) {
	var candidates []Candidate
	for i := 0; i < 10; i++ {
		candidates = append(candidates, Candidate{
			Key:       string(rune('a' + i)),
			Kind:      "rss",
			Followers: strongFollowers("did:plc:alice"),
		})
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 8 {
		t.Fatalf("len = %d, want 8", len(got))
	}
}

func TestRank_DropsCandidatesWithNoFollowers(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://a", Kind: "rss", Followers: nil},
		{Key: "https://b", Kind: "rss", Followers: strongFollowers("did:plc:alice")},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 1 || got[0].Key != "https://b" {
		t.Fatalf("got = %+v, want only https://b", got)
	}
}

func TestRank_ReasonNamesCountAndDeterministicTopFollower(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://a", Kind: "rss", Followers: strongFollowers("did:plc:carol", "did:plc:alice", "did:plc:bob")},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Reason.StrongCount != 3 {
		t.Errorf("Reason.StrongCount = %d, want 3", got[0].Reason.StrongCount)
	}
	if got[0].Reason.WeakCount != 0 {
		t.Errorf("Reason.WeakCount = %d, want 0", got[0].Reason.WeakCount)
	}
	// Same signal kind for all three, so alphabetically-first DID breaks the tie.
	if got[0].Reason.TopFollowerDID != "did:plc:alice" {
		t.Errorf("Reason.TopFollowerDID = %q, want did:plc:alice", got[0].Reason.TopFollowerDID)
	}
	if got[0].Reason.TopFollowerNetwork != "" {
		t.Errorf("Reason.TopFollowerNetwork = %q, want empty for a strong-tier top follower", got[0].Reason.TopFollowerNetwork)
	}
	if got[0].Reason.TopSignal != SignalSubscribe {
		t.Errorf("Reason.TopSignal = %v, want SignalSubscribe", got[0].Reason.TopSignal)
	}
}

func TestRank_CarriesTitleAndSiteURLAndKind(t *testing.T) {
	candidates := []Candidate{
		{
			Key:       "at://did:plc:pub/site.standard.publication/3p",
			Kind:      "standardfeed",
			Title:     "Example Zine",
			SiteURL:   "https://example.com",
			Followers: strongFollowers("did:plc:alice"),
		},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Kind != "standardfeed" || got[0].Title != "Example Zine" || got[0].SiteURL != "https://example.com" {
		t.Errorf("got = %+v", got[0])
	}
}

func TestRank_EmptyInputYieldsEmptyOutput(t *testing.T) {
	got := rank(nil, nil, 8, testSeed)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

// --- Trust tiers and the weak-tier cap. SPEC <discovery> Trust tiers, Weak-tier cap. ---

func TestRank_WeakTierCap_ManyWeakEndorsementsCannotOutrankMultipleStrongFollows(t *testing.T) {
	// Key acceptance scenario: breadth on the weak tier must not bury depth on the strong tier.
	manyWeak := make([]string, 50)
	for i := range manyWeak {
		manyWeak[i] = "did:plc:bsky" + string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	candidates := []Candidate{
		{Key: "https://strong-favorite", Kind: "rss", Followers: strongFollowers("did:plc:alice", "did:plc:bob", "did:plc:carol")},
		{Key: "https://viral-on-bluesky", Kind: "rss", Followers: weakFollowers(NetworkBluesky, manyWeak...)},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Key != "https://strong-favorite" {
		t.Errorf("got[0].Key = %q, want the multi-strong-follow source to rank first despite 50 weak endorsements on the other", got[0].Key)
	}
}

func TestRank_WeakTierNeverOutranksTwoStrongFollowersNoMatterHowWide(t *testing.T) {
	// The weak-tier cap is an absolute ceiling: two strong subscribers' worth outranks any number of weak-tier-only endorsements.
	weakDIDs := make([]string, 500)
	for i := range weakDIDs {
		weakDIDs[i] = "did:plc:w" + string(rune('a'+i%26)) + string(rune('0'+i/26)) + string(rune('0'+i%7))
	}
	candidates := []Candidate{
		{Key: "https://two-strong", Kind: "rss", Followers: strongFollowers("did:plc:alice", "did:plc:bob")},
		{Key: "https://n-weak", Kind: "rss", Followers: weakFollowers(NetworkBluesky, weakDIDs...)},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 2 || got[0].Key != "https://two-strong" {
		t.Fatalf("got = %+v, want https://two-strong first regardless of weak-tier breadth", got)
	}
}

func TestRank_StrongAndWeakContributionsCombineBeforeCap(t *testing.T) {
	// 1 strong (~1.0) + 2 weak (~0.5) stays below the ~2.0 cap ceiling, so it must rank below a pure-strong source with 2 strong followers (~2.0).
	candidates := []Candidate{
		{Key: "https://mixed", Kind: "rss", Followers: append(strongFollowers("did:plc:alice"), weakFollowers(NetworkBluesky, "did:plc:w1", "did:plc:w2")...)},
		{Key: "https://two-strong", Kind: "rss", Followers: strongFollowers("did:plc:bob", "did:plc:carol")},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 2 || got[0].Key != "https://two-strong" {
		t.Fatalf("got = %+v, want https://two-strong first (2 strong > 1 strong + 2 weak)", got)
	}
}

func TestRank_ReasonPrefersStrongTopFollowerOverWeak(t *testing.T) {
	candidates := []Candidate{
		{
			Key:  "https://mixed",
			Kind: "rss",
			Followers: append(
				weakFollowers(NetworkBluesky, "did:plc:aaa-weak"),
				strongFollowers("did:plc:zzz-strong")...,
			),
		},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Reason.TopFollowerDID != "did:plc:zzz-strong" {
		t.Errorf("Reason.TopFollowerDID = %q, want the strong-tier follower even though it sorts later alphabetically", got[0].Reason.TopFollowerDID)
	}
	if got[0].Reason.TopFollowerNetwork != "" {
		t.Errorf("Reason.TopFollowerNetwork = %q, want empty for a strong-tier top follower", got[0].Reason.TopFollowerNetwork)
	}
	if got[0].Reason.StrongCount != 1 || got[0].Reason.WeakCount != 1 {
		t.Errorf("Reason = %+v, want StrongCount=1 WeakCount=1", got[0].Reason)
	}
}

func TestRank_ReasonCreditsWeakFollowerWhenNoStrongContributor(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://bsky-only", Kind: "rss", Followers: weakFollowers(NetworkBluesky, "did:plc:carol", "did:plc:alice")},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Reason.TopFollowerDID != "did:plc:alice" {
		t.Errorf("Reason.TopFollowerDID = %q, want did:plc:alice (alphabetically first weak follower)", got[0].Reason.TopFollowerDID)
	}
	if got[0].Reason.TopFollowerNetwork != NetworkBluesky {
		t.Errorf("Reason.TopFollowerNetwork = %q, want %q", got[0].Reason.TopFollowerNetwork, NetworkBluesky)
	}
	wantDIDs := []string{"did:plc:alice", "did:plc:carol"}
	if !sameOrder(got[0].Reason.FollowerDIDs, wantDIDs) {
		t.Errorf("Reason.FollowerDIDs = %v, want %v", got[0].Reason.FollowerDIDs, wantDIDs)
	}
}

func TestRank_ReasonNamesTangledNetwork(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://tangled-only", Kind: "rss", Followers: weakFollowers(NetworkTangled, "did:plc:dev")},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 1 || got[0].Reason.TopFollowerNetwork != NetworkTangled {
		t.Fatalf("got = %+v, want TopFollowerNetwork = %q", got, NetworkTangled)
	}
}

// --- Tier resolution. SPEC <discovery>: a person in both tiers takes the higher, not the sum. ---

func TestResolveFollowerTiers_TableDriven(t *testing.T) {
	cases := []struct {
		name        string
		strongDIDs  []string
		weakFollows []WeakFollow
		wantDID     string
		wantTier    Tier
		wantNetwork Network
	}{
		{
			name:       "strong only",
			strongDIDs: []string{"did:plc:alice"},
			wantDID:    "did:plc:alice",
			wantTier:   TierStrong,
		},
		{
			name:        "weak only, bluesky",
			weakFollows: []WeakFollow{{DID: "did:plc:alice", Network: NetworkBluesky}},
			wantDID:     "did:plc:alice",
			wantTier:    TierWeak,
			wantNetwork: NetworkBluesky,
		},
		{
			name:        "weak only, tangled",
			weakFollows: []WeakFollow{{DID: "did:plc:alice", Network: NetworkTangled}},
			wantDID:     "did:plc:alice",
			wantTier:    TierWeak,
			wantNetwork: NetworkTangled,
		},
		{
			name:        "present in both tiers takes the higher (strong)",
			strongDIDs:  []string{"did:plc:alice"},
			weakFollows: []WeakFollow{{DID: "did:plc:alice", Network: NetworkBluesky}},
			wantDID:     "did:plc:alice",
			wantTier:    TierStrong,
			wantNetwork: "",
		},
		{
			name: "present in both weak networks: first occurrence wins",
			weakFollows: []WeakFollow{
				{DID: "did:plc:alice", Network: NetworkBluesky},
				{DID: "did:plc:alice", Network: NetworkTangled},
			},
			wantDID:     "did:plc:alice",
			wantTier:    TierWeak,
			wantNetwork: NetworkBluesky,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveFollowerTiers(tc.strongDIDs, tc.weakFollows)
			f, ok := got[tc.wantDID]
			if !ok {
				t.Fatalf("ResolveFollowerTiers(%v, %v) = %+v, missing %q", tc.strongDIDs, tc.weakFollows, got, tc.wantDID)
			}
			if f.Tier != tc.wantTier {
				t.Errorf("Tier = %v, want %v", f.Tier, tc.wantTier)
			}
			if f.Network != tc.wantNetwork {
				t.Errorf("Network = %q, want %q", f.Network, tc.wantNetwork)
			}
		})
	}
}

func TestResolveFollowerTiers_EmptyInputsYieldEmptyMap(t *testing.T) {
	got := ResolveFollowerTiers(nil, nil)
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty", got)
	}
}

func TestResolveFollowerTiers_DisjointStrongAndWeakBothPresent(t *testing.T) {
	got := ResolveFollowerTiers(
		[]string{"did:plc:strong1"},
		[]WeakFollow{{DID: "did:plc:weak1", Network: NetworkTangled}},
	)
	if len(got) != 2 {
		t.Fatalf("got = %+v, want 2 entries", got)
	}
	if got["did:plc:strong1"].Tier != TierStrong {
		t.Errorf("strong1 tier = %v, want TierStrong", got["did:plc:strong1"].Tier)
	}
	if got["did:plc:weak1"].Tier != TierWeak || got["did:plc:weak1"].Network != NetworkTangled {
		t.Errorf("weak1 = %+v, want TierWeak/tangled", got["did:plc:weak1"])
	}
}

// --- Seeded rotation. SPEC <discovery>. ---

func tiedCandidates(n int) []Candidate {
	// All candidates share one follower, so they land in the same score band and order is decided entirely by the shuffle.
	out := make([]Candidate, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Candidate{
			Key:       string(rune('a'+i)) + "-source",
			Kind:      "rss",
			Followers: strongFollowers("did:plc:alice"),
		})
	}
	return out
}

func keysOf(suggestions []Suggestion) []string {
	keys := make([]string, len(suggestions))
	for i, s := range suggestions {
		keys[i] = s.Key
	}
	return keys
}

func sameOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRank_SameSeed_StableOrder(t *testing.T) {
	candidates := tiedCandidates(6)

	first := rank(candidates, nil, 8, testSeed)
	second := rank(candidates, nil, 8, testSeed)

	if !sameOrder(keysOf(first), keysOf(second)) {
		t.Errorf("same seed produced different order:\n%v\n%v", keysOf(first), keysOf(second))
	}
}

func TestRank_DifferentSeed_DifferentOrderWithinTiedBand(t *testing.T) {
	candidates := tiedCandidates(6)
	seedA := int64(1)
	seedB := int64(2)

	first := rank(candidates, nil, 8, seedA)
	second := rank(candidates, nil, 8, seedB)

	if sameOrder(keysOf(first), keysOf(second)) {
		t.Errorf("different seeds produced identical order: %v", keysOf(first))
	}
}

func TestRank_ShuffleAppliesBeforeCap_DifferentCutBetweenSeeds(t *testing.T) {
	// Which 4 get dropped must vary with the seed, not just their order within the surviving set.
	candidates := tiedCandidates(12)
	seedA := int64(1)
	seedB := int64(2)

	a := rank(candidates, nil, 8, seedA)
	b := rank(candidates, nil, 8, seedB)

	setA := map[string]struct{}{}
	for _, k := range keysOf(a) {
		setA[k] = struct{}{}
	}
	differs := false
	for _, k := range keysOf(b) {
		if _, ok := setA[k]; !ok {
			differs = true
			break
		}
	}
	if !differs {
		t.Errorf("expected a different page cut between seeds, got identical sets: %v", keysOf(a))
	}
}

func TestRank_SeededRotation_NearTieDistinctScoresVary(t *testing.T) {
	// Scores are continuous and almost never exactly equal, so rotation must treat lean-scale differences as one band; an exact-equality shuffle would leave real-world lists frozen forever.
	a := Candidate{Key: "https://a", Kind: "rss", Followers: []Follower{followerAt("did:plc:alice", TierStrong, SignalSubscribe, testNow)}}
	b := Candidate{Key: "https://b", Kind: "rss", Followers: []Follower{followerAt("did:plc:bob", TierStrong, SignalSubscribe, testNow.Add(-5*24*time.Hour))}}

	seen := map[string]bool{}
	for seed := int64(1); seed <= 28; seed++ {
		got := rank([]Candidate{a, b}, nil, 8, seed)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		seen[got[0].Key] = true
	}
	if !seen["https://a"] || !seen["https://b"] {
		t.Errorf("28 seeds never rotated a near-tie pair (leaders seen: %v)", seen)
	}
}

func TestRank_ShuffleOnlyReordersWithinBand_NotAcrossBands(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://high", Kind: "rss", Followers: strongFollowers("did:plc:alice", "did:plc:bob", "did:plc:carol")},
		{Key: "https://low-a", Kind: "rss", Followers: strongFollowers("did:plc:alice")},
		{Key: "https://low-b", Kind: "rss", Followers: strongFollowers("did:plc:bob")},
	}

	for _, seed := range []int64{1, 2, 3, 999} {
		got := rank(candidates, nil, 8, seed)
		if len(got) != 3 || got[0].Key != "https://high" {
			t.Fatalf("seed %d: higher-scored band must stay first, got %v", seed, keysOf(got))
		}
	}
}

// --- Full signal model. SPEC <discovery> Signal ordering. ---

func followerAt(did string, tier Tier, kind SignalKind, at time.Time) Follower {
	return Follower{DID: did, Tier: tier, Signal: Signal{Kind: kind, At: at}}
}

func TestRank_SignalOrdering_PairwiseAtEqualRecency(t *testing.T) {
	// Same DID, same tier, same recency anchor: only the signal kind differs.
	cases := []struct {
		name      string
		strongest SignalKind
		weakest   SignalKind
	}{
		{"author beats subscribe", SignalAuthor, SignalSubscribe},
		{"subscribe beats share", SignalSubscribe, SignalShare},
		{"author beats share", SignalAuthor, SignalShare},
		// Save omitted here: a save-only candidate is now excluded from personal Rank entirely rather than merely outranked (see TestRank_CandidateWithOnlySaveSignalsNeverSuggested); save's ordinal position is covered on the trending path in trending_test.go.
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidates := []Candidate{
				{Key: "https://strongest", Kind: "rss", Followers: []Follower{followerAt("did:plc:alice", TierStrong, tc.strongest, testNow)}},
				{Key: "https://weakest", Kind: "rss", Followers: []Follower{followerAt("did:plc:alice", TierStrong, tc.weakest, testNow)}},
			}
			got := rank(candidates, nil, 8, testSeed)
			if len(got) != 2 || got[0].Key != "https://strongest" {
				t.Fatalf("got = %+v, want https://strongest ranked first", got)
			}
		})
	}
}

func TestRank_ReactionDecay_IsMonotoneWithAge(t *testing.T) {
	// Ages sit a score band apart, so fresh vs one day vs a month cross band boundaries and the assertion holds by decay, not by shuffle luck.
	fresh := []Candidate{
		{Key: "https://fresh", Kind: "rss", Followers: []Follower{followerAt("did:plc:alice", TierStrong, SignalShare, testNow)}},
	}
	old := []Candidate{
		{Key: "https://old", Kind: "rss", Followers: []Follower{followerAt("did:plc:alice", TierStrong, SignalShare, testNow.Add(-24*time.Hour))}},
	}
	veryOld := []Candidate{
		{Key: "https://very-old", Kind: "rss", Followers: []Follower{followerAt("did:plc:alice", TierStrong, SignalShare, testNow.Add(-30*24*time.Hour))}},
	}

	combined := append(append(fresh, old...), veryOld...)
	got := rank(combined, nil, 8, testSeed)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	wantOrder := []string{"https://fresh", "https://old", "https://very-old"}
	for i, key := range wantOrder {
		if got[i].Key != key {
			t.Errorf("got[%d].Key = %q, want %q (decay must be strictly monotone with age): %v", i, got[i].Key, key, keysOf(got))
		}
	}
}

// --- Save-privacy invariant. SPEC <saving-sharing>: saves are anonymous-batch-only, never used in personal ranking. ---

func TestRank_CandidateWithOnlySaveSignalsNeverSuggested(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://save-only", Kind: "rss", Followers: []Follower{followerAt("did:plc:saver", TierStrong, SignalSave, testNow)}},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 0 {
		t.Fatalf("got = %+v, want no suggestion for a candidate whose only followers carry a save signal", got)
	}
}

func TestRank_MixedSignalCandidateNeverCreditsSaveAsTopSignal(t *testing.T) {
	candidates := []Candidate{
		{
			Key:  "https://mixed-save",
			Kind: "rss",
			Followers: append(
				[]Follower{followerAt("did:plc:strong-saver", TierStrong, SignalSave, testNow)},
				weakFollowers(NetworkBluesky, "did:plc:weak-subscriber")...,
			),
		},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Reason.TopSignal == SignalSave {
		t.Errorf("Reason.TopSignal = %v, want never SignalSave, even though the save is the strong-tier, freshest signal present", got[0].Reason.TopSignal)
	}
	if got[0].Reason.TopFollowerDID != "did:plc:weak-subscriber" {
		t.Errorf("Reason.TopFollowerDID = %q, want the weak subscriber once the save-only strong follower is excluded from consideration", got[0].Reason.TopFollowerDID)
	}
}

func TestRank_OneSignalPerPersonPerSource_StrongestWins(t *testing.T) {
	// The same person subscribed and shared five items: they must count once, at the subscribe signal, not five times at the weaker share signal.
	person := "did:plc:alice"
	followers := []Follower{
		followerAt(person, TierStrong, SignalSubscribe, testOldAt),
	}
	for i := 0; i < 5; i++ {
		followers = append(followers, followerAt(person, TierStrong, SignalShare, testNow))
	}
	candidateWithSubscribeAndShares := Candidate{Key: "https://mixed", Kind: "rss", Followers: followers}

	got := rank([]Candidate{candidateWithSubscribeAndShares}, nil, 8, testSeed)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Reason.StrongCount != 1 {
		t.Errorf("Reason.StrongCount = %d, want 1 (one person, collapsed to one signal)", got[0].Reason.StrongCount)
	}
	if got[0].Reason.TopSignal != SignalSubscribe {
		t.Errorf("Reason.TopSignal = %v, want SignalSubscribe (subscribe beats share regardless of share count)", got[0].Reason.TopSignal)
	}
}

func TestRank_OneSignalPerPersonPerSource_ScoreMatchesSubscribeOnly(t *testing.T) {
	person := "did:plc:alice"
	followers := []Follower{followerAt(person, TierStrong, SignalSubscribe, testOldAt)}
	for i := 0; i < 5; i++ {
		followers = append(followers, followerAt(person, TierStrong, SignalShare, testNow))
	}
	withShares := Candidate{Key: "https://with-shares", Kind: "rss", Followers: followers}
	subscribeOnly := Candidate{Key: "https://subscribe-only", Kind: "rss", Followers: []Follower{followerAt(person, TierStrong, SignalSubscribe, testOldAt)}}

	// A sentinel guaranteed to score lower reveals whether the extra shares changed the score: both rankings must place their strong-tier candidate first, in the same relative position.
	sentinel := Candidate{Key: "https://sentinel", Kind: "rss", Followers: weakFollowers(NetworkBluesky, "did:plc:z")}

	gotWith := rank([]Candidate{withShares, sentinel}, nil, 8, testSeed)
	gotWithout := rank([]Candidate{subscribeOnly, sentinel}, nil, 8, testSeed)

	if len(gotWith) != 2 || len(gotWithout) != 2 {
		t.Fatalf("gotWith = %+v, gotWithout = %+v", gotWith, gotWithout)
	}
	if gotWith[0].Key != "https://with-shares" || gotWithout[0].Key != "https://subscribe-only" {
		t.Fatalf("gotWith = %+v, gotWithout = %+v, want both strong candidates first", gotWith, gotWithout)
	}
}

func TestRank_RecencyLean_NewerSubscriptionOutranksOlder(t *testing.T) {
	newer := Candidate{Key: "https://newer", Kind: "rss", Followers: []Follower{followerAt("did:plc:alice", TierStrong, SignalSubscribe, testNow)}}
	older := Candidate{Key: "https://older", Kind: "rss", Followers: []Follower{followerAt("did:plc:alice", TierStrong, SignalSubscribe, testNow.Add(-90*24*time.Hour))}}

	got := rank([]Candidate{older, newer}, nil, 8, testSeed)
	if len(got) != 2 || got[0].Key != "https://newer" {
		t.Fatalf("got = %+v, want the newer subscription ranked first", got)
	}
}

func TestRank_RecencyLean_ActivelyPublishingAuthorOutranksDormant(t *testing.T) {
	active := Candidate{Key: "https://active", Kind: "standardfeed", Followers: []Follower{followerAt("did:plc:alice", TierStrong, SignalAuthor, testNow)}}
	dormant := Candidate{Key: "https://dormant", Kind: "standardfeed", Followers: []Follower{followerAt("did:plc:alice", TierStrong, SignalAuthor, testNow.Add(-365*24*time.Hour))}}

	got := rank([]Candidate{dormant, active}, nil, 8, testSeed)
	if len(got) != 2 || got[0].Key != "https://active" {
		t.Fatalf("got = %+v, want the actively-publishing author ranked first", got)
	}
}

func TestRank_RecencyLean_UnknownTimestampReadsAsNeutralNotFresh(t *testing.T) {
	// An unknown createdAt (zero time.Time) must not out-rank a known, merely-old timestamp: unknown is neutral, never treated as freshest.
	unknown := Candidate{Key: "https://unknown", Kind: "rss", Followers: []Follower{followerAt("did:plc:alice", TierStrong, SignalSubscribe, time.Time{})}}
	knownFresh := Candidate{Key: "https://known-fresh", Kind: "rss", Followers: []Follower{followerAt("did:plc:alice", TierStrong, SignalSubscribe, testNow)}}

	got := rank([]Candidate{unknown, knownFresh}, nil, 8, testSeed)
	if len(got) != 2 || got[0].Key != "https://known-fresh" {
		t.Fatalf("got = %+v, want the known-fresh subscription ranked first, not the unknown-timestamp one", got)
	}
}

func TestRank_AuthorSignalRanksTopEvenAgainstManySubscribers(t *testing.T) {
	// SPEC <discovery>, <social-layer>: author base weight (1.5) exceeds any single subscriber's (1.0), even against a handful of ordinary subscribes.
	authored := Candidate{Key: "https://authored", Kind: "standardfeed", Followers: []Follower{followerAt("did:plc:alice", TierStrong, SignalAuthor, testOldAt)}}
	subscribed := Candidate{Key: "https://subscribed", Kind: "rss", Followers: strongFollowers("did:plc:bob")}

	got := rank([]Candidate{subscribed, authored}, nil, 8, testSeed)
	if len(got) != 2 || got[0].Key != "https://authored" {
		t.Fatalf("got = %+v, want the authored publication ranked first", got)
	}
	if got[0].Reason.TopSignal != SignalAuthor {
		t.Errorf("Reason.TopSignal = %v, want SignalAuthor", got[0].Reason.TopSignal)
	}
}

// --- Self tier. SPEC <discovery> The user's own foreign records: self > reader-network follow > Bluesky/Tangled. ---

func TestRank_SelfTierOutranksStrongTierAtEqualSignalKind(t *testing.T) {
	self := Candidate{Key: "https://self", Kind: "rss", Followers: []Follower{
		{DID: "did:plc:me", Tier: TierSelf, SelfApp: SelfSourceSkyreader, Signal: subscribeSignal()},
	}}
	strong := Candidate{Key: "https://strong", Kind: "rss", Followers: strongFollowers("did:plc:alice")}

	got := rank([]Candidate{strong, self}, nil, 8, testSeed)
	if len(got) != 2 || got[0].Key != "https://self" {
		t.Fatalf("got = %+v, want the self-tier candidate ranked first", got)
	}
}

func TestRank_SelfTierReasonCarriesSelfApp(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://self", Kind: "rss", Followers: []Follower{
			{DID: "did:plc:me", Tier: TierSelf, SelfApp: SelfSourceGlean, Signal: subscribeSignal()},
		}},
	}

	got := rank(candidates, nil, 8, testSeed)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Reason.SelfApp != SelfSourceGlean {
		t.Errorf("Reason.SelfApp = %q, want %q", got[0].Reason.SelfApp, SelfSourceGlean)
	}
	if got[0].Reason.TopSignal != SignalSubscribe {
		t.Errorf("Reason.TopSignal = %v, want SignalSubscribe", got[0].Reason.TopSignal)
	}
	if got[0].Reason.StrongCount != 0 || got[0].Reason.WeakCount != 0 {
		t.Errorf("Reason = %+v, want StrongCount=0 WeakCount=0 (self isn't a followed person)", got[0].Reason)
	}
}

func TestRank_SelfTierReasonTakesPriorityOverStrongAndWeakOnSameCandidate(t *testing.T) {
	candidates := []Candidate{
		{
			Key:  "https://mixed",
			Kind: "rss",
			Followers: append(
				append(strongFollowers("did:plc:alice", "did:plc:bob", "did:plc:carol"), weakFollowers(NetworkBluesky, "did:plc:w1")...),
				Follower{DID: "did:plc:me", Tier: TierSelf, SelfApp: SelfSourceSkyreader, Signal: subscribeSignal()},
			),
		},
	}

	got := rank(candidates, nil, 8, testSeed)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Reason.SelfApp != SelfSourceSkyreader {
		t.Errorf("Reason.SelfApp = %q, want %q even though strong/weak followers are also present", got[0].Reason.SelfApp, SelfSourceSkyreader)
	}
	// All contributors share self's signal kind (subscribe), so all of them count; self only decides which reason is displayed.
	if got[0].Reason.StrongCount != 3 || got[0].Reason.WeakCount != 1 {
		t.Errorf("Reason = %+v, want StrongCount=3 WeakCount=1", got[0].Reason)
	}
}

func TestRank_ReasonCount_OnlyFollowersCarryingTopSignalCount(t *testing.T) {
	// 1 author + 2 subscribers: subscribers don't carry the top signal (author), so they must not inflate StrongCount.
	candidates := []Candidate{
		{
			Key:  "https://mixed-strong",
			Kind: "standardfeed",
			Followers: []Follower{
				followerAt("did:plc:author1", TierStrong, SignalAuthor, testOldAt),
				followerAt("did:plc:sub1", TierStrong, SignalSubscribe, testOldAt),
				followerAt("did:plc:sub2", TierStrong, SignalSubscribe, testOldAt),
			},
		},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Reason.TopSignal != SignalAuthor {
		t.Fatalf("Reason.TopSignal = %v, want SignalAuthor", got[0].Reason.TopSignal)
	}
	if got[0].Reason.StrongCount != 1 {
		t.Errorf("Reason.StrongCount = %d, want 1 (only the author carries the top signal; the two subscribers must not inflate the count)", got[0].Reason.StrongCount)
	}
	if got[0].Reason.WeakCount != 0 {
		t.Errorf("Reason.WeakCount = %d, want 0", got[0].Reason.WeakCount)
	}
}

func TestRank_ReasonCount_WeakFollowersNotCarryingTopSignalExcludedFromWeakCount(t *testing.T) {
	// A strong author sets TopSignal=author; weak-tier subscribers don't carry it, so WeakCount must exclude them.
	candidates := []Candidate{
		{
			Key:  "https://mixed-weak",
			Kind: "standardfeed",
			Followers: []Follower{
				followerAt("did:plc:author1", TierStrong, SignalAuthor, testOldAt),
				{DID: "did:plc:w1", Tier: TierWeak, Network: NetworkBluesky, Signal: subscribeSignal()},
				{DID: "did:plc:w2", Tier: TierWeak, Network: NetworkBluesky, Signal: subscribeSignal()},
			},
		},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Reason.TopSignal != SignalAuthor {
		t.Fatalf("Reason.TopSignal = %v, want SignalAuthor", got[0].Reason.TopSignal)
	}
	if got[0].Reason.StrongCount != 1 {
		t.Errorf("Reason.StrongCount = %d, want 1", got[0].Reason.StrongCount)
	}
	if got[0].Reason.WeakCount != 0 {
		t.Errorf("Reason.WeakCount = %d, want 0 (weak subscribers don't carry the top signal author)", got[0].Reason.WeakCount)
	}
}

// --- FollowerDIDs (avatar stack). SPEC <discovery>: the avatar stack depicts exactly the people StrongCount/WeakCount count. ---

func TestRank_ReasonFollowerDIDs_MatchesSignalScopedCounts(t *testing.T) {
	candidates := []Candidate{
		{
			Key:  "https://mixed-signals",
			Kind: "rss",
			Followers: []Follower{
				followerAt("did:plc:sub-alice", TierStrong, SignalSubscribe, testOldAt),
				followerAt("did:plc:sub-bob", TierStrong, SignalSubscribe, testOldAt),
				followerAt("did:plc:sharer", TierStrong, SignalShare, testOldAt),
			},
		},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	r := got[0].Reason
	if r.TopSignal != SignalSubscribe {
		t.Fatalf("Reason.TopSignal = %v, want SignalSubscribe", r.TopSignal)
	}
	if len(r.FollowerDIDs) != r.StrongCount {
		t.Fatalf("len(FollowerDIDs) = %d, want StrongCount = %d", len(r.FollowerDIDs), r.StrongCount)
	}
	want := map[string]bool{"did:plc:sub-alice": true, "did:plc:sub-bob": true}
	for _, did := range r.FollowerDIDs {
		if !want[did] {
			t.Errorf("FollowerDIDs contains %q, want only the two subscribers", did)
		}
	}
}

func TestRank_ReasonFollowerDIDs_CapsAtThree_StrongTierFirst(t *testing.T) {
	candidates := []Candidate{
		{
			Key:  "https://wide-reach",
			Kind: "rss",
			Followers: append(
				strongFollowers("did:plc:strong-a", "did:plc:strong-b"),
				weakFollowers(NetworkBluesky, "did:plc:weak-a", "did:plc:weak-b", "did:plc:weak-c")...,
			),
		},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	dids := got[0].Reason.FollowerDIDs
	if len(dids) != 3 {
		t.Fatalf("len(FollowerDIDs) = %d, want 3", len(dids))
	}
	if dids[0] != "did:plc:strong-a" || dids[1] != "did:plc:strong-b" {
		t.Errorf("FollowerDIDs = %v, want the two strong DIDs to lead", dids)
	}
}

func TestRank_ReasonFollowerDIDs_FirstElementMatchesTopFollowerDID(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://a", Kind: "rss", Followers: strongFollowers("did:plc:carol", "did:plc:alice", "did:plc:bob")},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	r := got[0].Reason
	if len(r.FollowerDIDs) == 0 {
		t.Fatalf("FollowerDIDs empty, want at least 1")
	}
	if r.FollowerDIDs[0] != r.TopFollowerDID {
		t.Errorf("FollowerDIDs[0] = %q, want TopFollowerDID %q", r.FollowerDIDs[0], r.TopFollowerDID)
	}
}

func TestRank_ReasonFollowerDIDs_EmptyWhenSelfCredited(t *testing.T) {
	candidates := []Candidate{
		{
			Key:  "https://self-plus-strong",
			Kind: "rss",
			Followers: append(
				strongFollowers("did:plc:alice", "did:plc:bob"),
				Follower{DID: "did:plc:me", Tier: TierSelf, SelfApp: SelfSourceSkyreader, Signal: subscribeSignal()},
			),
		},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if len(got[0].Reason.FollowerDIDs) != 0 {
		t.Errorf("Reason.FollowerDIDs = %v, want empty when self is credited", got[0].Reason.FollowerDIDs)
	}
}

func TestRank_ReasonFollowerDIDs_SingleFollowerYieldsSliceOfOne(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://solo", Kind: "rss", Followers: strongFollowers("did:plc:alice")},
	}

	got := rank(candidates, nil, 8, testSeed)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	want := []string{"did:plc:alice"}
	if !sameOrder(got[0].Reason.FollowerDIDs, want) {
		t.Errorf("Reason.FollowerDIDs = %v, want %v", got[0].Reason.FollowerDIDs, want)
	}
}

func TestSignalKind_String(t *testing.T) {
	cases := map[SignalKind]string{
		SignalAuthor:    "author",
		SignalSubscribe: "subscribe",
		SignalShare:     "share",
		SignalSave:      "save",
	}
	for kind, want := range cases {
		if got := kind.String(); got != want {
			t.Errorf("SignalKind(%d).String() = %q, want %q", kind, got, want)
		}
	}
}
