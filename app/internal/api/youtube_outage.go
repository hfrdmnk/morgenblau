package api

import (
	"context"
	"time"

	"morgenblau/internal/database/db"
	"morgenblau/internal/feedfinder"
)

type YouTubeFeedHealth interface {
	CountYouTubeFeedFailures(ctx context.Context, failingSince string) (db.CountYouTubeFeedFailuresRow, error)
}

func youtubeDown(ctx context.Context, health YouTubeFeedHealth, now time.Time) (bool, error) {
	feeds, err := health.CountYouTubeFeedFailures(ctx, feedfinder.YouTubeOutageCutoff(now))
	if err != nil {
		return false, err
	}
	return feedfinder.YouTubeFeedsDown(feeds.Total, feeds.Failing), nil
}
