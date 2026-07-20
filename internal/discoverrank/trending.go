package discoverrank

import (
	"time"

	"morgenblau/internal/discoverlang"
)

// MinDistinctRepos is the trending quality bar; kills single-repo spam/self-promotion.
const MinDistinctRepos = 3

// RankTrending mirrors Rank's scoring core (same weights, decay, shuffle, cap) but drops
// sub-MinDistinctRepos candidates and forces every Follower to TierStrong, since these represent contributing repos rather than followed people.
// Calls rankScored directly, not Rank: this is the one path allowed to score save signals from the anonymous batch. SPEC <discovery> save-privacy invariant.
func RankTrending(candidates []Candidate, excluded map[string]struct{}, max int, seed int64, now time.Time) []Suggestion {
	return pageValues(RankTrendingPage(candidates, excluded, max, seed, now, nil))
}

// FilterByLanguage drops candidates outside the languages the user demonstrably reads; kept separate from RankTrending so its signature and tests stay untouched.
func FilterByLanguage(candidates []Candidate, reader map[discoverlang.Language]struct{}) []Candidate {
	out := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		if discoverlang.Passes(c.Language, reader) {
			out = append(out, c)
		}
	}
	return out
}

func distinctFollowerDIDs(followers []Follower) int {
	seen := make(map[string]struct{}, len(followers))
	for _, f := range followers {
		seen[f.DID] = struct{}{}
	}
	return len(seen)
}

// asRepoSignals forces every Follower to TierStrong so RankTrending's "no trust term" guarantee holds regardless of what the caller passed in.
func asRepoSignals(followers []Follower) []Follower {
	out := make([]Follower, len(followers))
	for i, f := range followers {
		out[i] = Follower{DID: f.DID, Tier: TierStrong, Signal: f.Signal}
	}
	return out
}
