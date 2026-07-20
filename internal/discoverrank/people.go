package discoverrank

import "time"

// PersonCandidate is a person surfaced for the People tab's "For you" section, before ranking.
type PersonCandidate struct {
	DID string
	// BlueskyFollow, TangledFollow: SPEC <discovery> People candidate classes 1/2.
	BlueskyFollow bool
	TangledFollow bool
	// FollowedByDID is the alphabetically-first reader-network friend who follows this candidate; empty if not reached that way.
	FollowedByDID string
	// Activity is this candidate's own reader-network records; a non-empty Activity is the hard eligibility bar, regardless of graph proximity.
	Activity []Signal
	// SharedSourceCount is the taste-overlap bonus input: canonical source keys this candidate shares with the viewer's own subscriptions.
	SharedSourceCount int
}

// PersonReason is the structured basis for a person suggestion; the frontend phrases it into English.
type PersonReason struct {
	BlueskyFollow     bool
	TangledFollow     bool
	FollowedByDID     string
	SharedSourceCount int
}

// PersonSuggestion is one ranked "For you" person card.
type PersonSuggestion struct {
	DID    string
	Reason PersonReason
}

// tasteOverlapWeight is scaled off SubscribeBaseWeight so a handful of shared sources outranks a stranger at equal activity, without overlap alone dwarfing higher activity.
const tasteOverlapWeight = SubscribeBaseWeight * 0.5

// RankPeople filters, scores, shuffles, and caps candidates like Rank, but scores a person's own activity rather than a source's followers.
// Saves never reach personal eligibility or scoring: SPEC <saving-sharing> confines them to the anonymous trending batch (RankPeopleTrending), so they're dropped here before eligibility/score logic runs.
func RankPeople(candidates []PersonCandidate, excluded map[string]struct{}, max int, seed int64, now time.Time) []PersonSuggestion {
	return pageValues(RankPeoplePage(candidates, excluded, max, seed, now, nil))
}

// personScore sums decayed activity weight plus the taste-overlap bonus.
func personScore(c PersonCandidate, now time.Time) float64 {
	var activity float64
	for _, s := range c.Activity {
		activity += signalWeight(s, now)
	}
	return activity + float64(c.SharedSourceCount)*tasteOverlapWeight
}

// dropSaveSignals strips save signals before eligibility/score logic runs, so a save alone can never clear eligibility and can never contribute to a person's score.
func dropSaveSignals(activity []Signal) []Signal {
	out := make([]Signal, 0, len(activity))
	for _, s := range activity {
		if s.Kind == SignalSave {
			continue
		}
		out = append(out, s)
	}
	return out
}
