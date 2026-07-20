package discoverrank

import "testing"

func TestRankPageContinuesAfterRemovedPriorItem(t *testing.T) {
	follower := Follower{
		DID:    "did:plc:reader",
		Tier:   TierStrong,
		Signal: Signal{Kind: SignalSubscribe, At: testNow},
	}
	candidates := []Candidate{
		{Key: "https://a.example/feed", Followers: []Follower{follower}},
		{Key: "https://b.example/feed", Followers: []Follower{follower}},
		{Key: "https://c.example/feed", Followers: []Follower{follower}},
		{Key: "https://d.example/feed", Followers: []Follower{follower}},
	}

	first := RankPage(candidates, nil, 2, testSeed, testNow, nil)
	if len(first.Items) != 2 || !first.HasMore {
		t.Fatalf("first page = %+v, want two items and HasMore", first)
	}

	removedKey := first.Items[1].Value.Key
	remaining := make([]Candidate, 0, len(candidates)-1)
	for _, candidate := range candidates {
		if candidate.Key != removedKey {
			remaining = append(remaining, candidate)
		}
	}

	second := RankPage(remaining, nil, 2, testSeed, testNow, &first.Items[1].Position)
	if len(second.Items) != 2 {
		t.Fatalf("second page = %+v, want the two remaining items", second)
	}

	seen := map[string]struct{}{
		first.Items[0].Value.Key: {},
		first.Items[1].Value.Key: {},
	}
	for _, item := range second.Items {
		if _, duplicate := seen[item.Value.Key]; duplicate {
			t.Fatalf("second page repeated %q from the first page", item.Value.Key)
		}
	}
	if second.HasMore {
		t.Fatal("second page HasMore = true, want exhausted")
	}
}

func TestRankPeoplePageReportsMoreThenExhausts(t *testing.T) {
	candidates := []PersonCandidate{
		{DID: "did:plc:a", Activity: []Signal{{Kind: SignalSubscribe, At: testNow}}},
		{DID: "did:plc:b", Activity: []Signal{{Kind: SignalSubscribe, At: testNow}}},
		{DID: "did:plc:c", Activity: []Signal{{Kind: SignalSubscribe, At: testNow}}},
	}

	first := RankPeoplePage(candidates, nil, 2, testSeed, testNow, nil)
	if len(first.Items) != 2 || !first.HasMore {
		t.Fatalf("first page = %+v, want two items and HasMore", first)
	}

	second := RankPeoplePage(candidates, nil, 2, testSeed, testNow, &first.Items[1].Position)
	if len(second.Items) != 1 || second.HasMore {
		t.Fatalf("second page = %+v, want one final item", second)
	}
}
