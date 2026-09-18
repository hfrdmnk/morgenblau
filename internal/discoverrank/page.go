package discoverrank

import (
	"sort"
	"time"
)

type Position struct {
	Band    int    `json:"b"`
	Shuffle uint64 `json:"s"`
	Key     string `json:"k"`
}

type Ranked[T any] struct {
	Value    T
	Position Position
}

type Page[T any] struct {
	Items   []Ranked[T]
	HasMore bool
}

type scoredValue[T any] struct {
	key   string
	band  int
	value T
}

func RankPage(candidates []Candidate, excluded map[string]struct{}, limit int, seed int64, now time.Time, after *Position) Page[Suggestion] {
	personal := make([]Candidate, len(candidates))
	for i, candidate := range candidates {
		candidate.Followers = dropSaveFollowers(candidate.Followers)
		personal[i] = candidate
	}
	return rankScoredPage(personal, excluded, limit, seed, now, after)
}

func RankTrendingPage(candidates []Candidate, excluded map[string]struct{}, limit int, seed int64, now time.Time, after *Position) Page[Suggestion] {
	filtered := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if distinctFollowerDIDs(candidate.Followers) < MinDistinctRepos {
			continue
		}
		candidate.Followers = asRepoSignals(candidate.Followers)
		filtered = append(filtered, candidate)
	}
	return rankScoredPage(filtered, excluded, limit, seed, now, after)
}

func rankScoredPage(candidates []Candidate, excluded map[string]struct{}, limit int, seed int64, now time.Time, after *Position) Page[Suggestion] {
	values := make([]scoredValue[Suggestion], 0, len(candidates))
	for _, candidate := range candidates {
		if len(candidate.Followers) == 0 {
			continue
		}
		if _, duplicate := excluded[candidate.Key]; duplicate {
			continue
		}
		values = append(values, scoredValue[Suggestion]{
			key:  candidate.Key,
			band: scoreBand(score(candidate.Followers, now)),
			value: Suggestion{
				Key:     candidate.Key,
				Kind:    candidate.Kind,
				Title:   candidate.Title,
				SiteURL: candidate.SiteURL,
				Reason:  reasonFor(candidate.Followers),
			},
		})
	}
	return paginate(values, limit, seed, after)
}

func RankPeoplePage(candidates []PersonCandidate, excluded map[string]struct{}, limit int, seed int64, now time.Time, after *Position) Page[PersonSuggestion] {
	values := make([]scoredValue[PersonSuggestion], 0, len(candidates))
	for _, candidate := range candidates {
		candidate.Activity = dropSaveSignals(candidate.Activity)
		if !candidate.Eligible && len(candidate.Activity) == 0 {
			continue
		}
		if _, duplicate := excluded[candidate.DID]; duplicate {
			continue
		}
		values = append(values, scoredValue[PersonSuggestion]{
			key:  candidate.DID,
			band: scoreBand(personScore(candidate, now)),
			value: PersonSuggestion{
				DID: candidate.DID,
				Reason: PersonReason{
					BlueskyFollow:     candidate.BlueskyFollow,
					TangledFollow:     candidate.TangledFollow,
					FollowedByDID:     candidate.FollowedByDID,
					SharedSourceCount: candidate.SharedSourceCount,
				},
			},
		})
	}
	return paginate(values, limit, seed, after)
}

func RankPeopleTrendingPage(candidates []TrendingPersonCandidate, excluded map[string]struct{}, limit int, seed int64, now time.Time, after *Position) Page[PersonSuggestion] {
	values := make([]scoredValue[PersonSuggestion], 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.Eligible || distinctStrings(candidate.FollowerDIDs) < MinDistinctRepos {
			continue
		}
		if _, duplicate := excluded[candidate.DID]; duplicate {
			continue
		}
		values = append(values, scoredValue[PersonSuggestion]{
			key:   candidate.DID,
			band:  scoreBand(trendingPersonScore(candidate, now)),
			value: PersonSuggestion{DID: candidate.DID},
		})
	}
	return paginate(values, limit, seed, after)
}

func paginate[T any](values []scoredValue[T], limit int, seed int64, after *Position) Page[T] {
	if limit <= 0 {
		return Page[T]{Items: []Ranked[T]{}}
	}

	ranked := make([]Ranked[T], 0, len(values))
	for _, value := range values {
		position := Position{
			Band:    value.band,
			Shuffle: shuffleKey(seed, value.key),
			Key:     value.key,
		}
		if after != nil && !positionAfter(position, *after) {
			continue
		}
		ranked = append(ranked, Ranked[T]{Value: value.value, Position: position})
	}

	sort.Slice(ranked, func(i, j int) bool {
		return positionBefore(ranked[i].Position, ranked[j].Position)
	})

	hasMore := len(ranked) > limit
	if hasMore {
		ranked = ranked[:limit]
	}
	return Page[T]{Items: ranked, HasMore: hasMore}
}

func positionBefore(a, b Position) bool {
	if a.Band != b.Band {
		return a.Band > b.Band
	}
	if a.Shuffle != b.Shuffle {
		return a.Shuffle < b.Shuffle
	}
	return a.Key < b.Key
}

func positionAfter(position, cursor Position) bool {
	return position != cursor && !positionBefore(position, cursor)
}

func pageValues[T any](page Page[T]) []T {
	values := make([]T, 0, len(page.Items))
	for _, item := range page.Items {
		values = append(values, item.Value)
	}
	return values
}
