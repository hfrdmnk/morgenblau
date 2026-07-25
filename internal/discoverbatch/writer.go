package discoverbatch

import (
	"context"
	"time"

	"morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
)

// Writer replaces one repo's aggregate rows inside a transaction. SPEC <discovery>: diff, not accumulate.
// The Delete/Rebuild pairs refresh the precomputed quality-bar counts the trending reads join against.
type Writer interface {
	DeleteDiscoverTrendingSignalsForRepo(ctx context.Context, repoDid string) error
	InsertDiscoverTrendingSignal(ctx context.Context, arg db.InsertDiscoverTrendingSignalParams) error
	DeleteDiscoverTrendingFollowsForRepo(ctx context.Context, repoDid string) error
	InsertDiscoverTrendingFollow(ctx context.Context, arg db.InsertDiscoverTrendingFollowParams) error
	DeleteDiscoverTrendingSourceCounts(ctx context.Context) error
	RebuildDiscoverTrendingSourceCounts(ctx context.Context) error
	DeleteDiscoverTrendingFollowCounts(ctx context.Context) error
	RebuildDiscoverTrendingFollowCounts(ctx context.Context) error
}

// ReplaceRepoSignals deletes then reinserts a repo's rows in one transaction so reruns diff rather than accumulate. SPEC <discovery>.
func ReplaceRepoSignals(ctx context.Context, w Writer, repoDID string, signals map[string]RepoSource, fetchedAt string) error {
	if err := w.DeleteDiscoverTrendingSignalsForRepo(ctx, repoDID); err != nil {
		return err
	}
	for key, s := range signals {
		if err := w.InsertDiscoverTrendingSignal(ctx, db.InsertDiscoverTrendingSignalParams{
			RepoDid:    repoDID,
			SourceKey:  key,
			Kind:       s.Kind,
			Title:      nilIfEmpty(s.Title),
			SiteUrl:    nilIfEmpty(s.SiteURL),
			SignalKind: s.Signal.Kind.String(),
			SignalAt:   formatOptionalTime(s.Signal.At),
			FetchedAt:  fetchedAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceRepoFollows applies the same diff/replace contract as ReplaceRepoSignals, scoped to the follower aggregate.
func ReplaceRepoFollows(ctx context.Context, w Writer, repoDID string, follows []discovercrawl.ReaderNetworkFollow, fetchedAt string) error {
	if err := w.DeleteDiscoverTrendingFollowsForRepo(ctx, repoDID); err != nil {
		return err
	}
	for _, f := range follows {
		if err := w.InsertDiscoverTrendingFollow(ctx, db.InsertDiscoverTrendingFollowParams{
			RepoDid:    repoDID,
			SubjectDid: f.DID,
			FetchedAt:  fetchedAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func formatOptionalTime(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
