package discoverrank

import "time"

// TrendingFollowerWeight puts follower count and decayed share activity on the same scale.
const TrendingFollowerWeight = SubscribeBaseWeight * StrongWeight

// TrendingPersonCandidate is a person surfaced for the People tab's "Trending in the reader network" section, before ranking.
type TrendingPersonCandidate struct {
	DID string
	// FollowerDIDs are repos following this DID via blue.morgen.graph.follow or app.skyreader.social.follow; duplicates collapse rather than double-count.
	FollowerDIDs []string
	// ShareActivity is this person's own share signals, network-wide.
	ShareActivity []Signal
	// Eligible mirrors PersonCandidate.Activity: true when this person has any reader-network record, independent of the follower/share terms that score them.
	Eligible bool
}

// RankPeopleTrending ranks People's "Trending" section: follower count plus decayed share activity, no trust term and no per-user reason.
func RankPeopleTrending(candidates []TrendingPersonCandidate, excluded map[string]struct{}, max int, seed int64, now time.Time) []PersonSuggestion {
	return pageValues(RankPeopleTrendingPage(candidates, excluded, max, seed, now, nil))
}

func trendingPersonScore(c TrendingPersonCandidate, now time.Time) float64 {
	score := float64(distinctStrings(c.FollowerDIDs)) * TrendingFollowerWeight
	for _, s := range c.ShareActivity {
		score += signalWeight(s, now)
	}
	return score
}

func distinctStrings(vals []string) int {
	seen := make(map[string]struct{}, len(vals))
	for _, v := range vals {
		seen[v] = struct{}{}
	}
	return len(seen)
}
