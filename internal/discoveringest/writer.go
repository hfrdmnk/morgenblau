package discoveringest

import (
	"context"
	"time"

	"morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
)

// rebuildWriter is one repo's write batch: the aggregate replace surface plus the dirty-mark clear, so both land or neither does.
// The Delete/Rebuild pairs refresh the precomputed quality-bar counts every trending read joins against.
type rebuildWriter interface {
	DeleteDiscoverTrendingSignalsForRepo(ctx context.Context, repoDid string) error
	InsertDiscoverTrendingSignal(ctx context.Context, arg db.InsertDiscoverTrendingSignalParams) error
	DeleteDiscoverTrendingFollowsForRepo(ctx context.Context, repoDid string) error
	InsertDiscoverTrendingFollow(ctx context.Context, arg db.InsertDiscoverTrendingFollowParams) error
	DeleteDiscoverTrendingSourceCounts(ctx context.Context) error
	RebuildDiscoverTrendingSourceCounts(ctx context.Context) error
	DeleteDiscoverTrendingFollowCounts(ctx context.Context) error
	RebuildDiscoverTrendingFollowCounts(ctx context.Context) error
	DeleteTapDirtyRepo(ctx context.Context, arg db.DeleteTapDirtyRepoParams) error
}

// replaceRepoSignals deletes then reinserts a repo's rows in one transaction so reruns diff rather than accumulate. SPEC <discovery>.
func replaceRepoSignals(ctx context.Context, w rebuildWriter, repoDID string, signals map[string]repoSource, fetchedAt string) error {
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

// replaceRepoFollows applies the same diff/replace contract as replaceRepoSignals, scoped to the follower aggregate.
func replaceRepoFollows(ctx context.Context, w rebuildWriter, repoDID string, follows []discovercrawl.ReaderNetworkFollow, fetchedAt string) error {
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
