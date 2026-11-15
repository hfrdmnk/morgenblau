package api

import (
	"context"

	"morgenblau/internal/database/db"
)

// YouTubeFeedHealth is the shared feed-catalog read behind youtubeDown.
type YouTubeFeedHealth interface {
	CountYouTubeFeedFailures(ctx context.Context) (db.CountYouTubeFeedFailuresRow, error)
}

// youtubeOutageMinFailing keeps a few dead channels on a small server from reading as an outage.
const youtubeOutageMinFailing = 3

// youtubeDown reports an outage of YouTube's feed server, which fails every channel at once for hours: at least half of the followed YouTube feeds failing together.
func youtubeDown(ctx context.Context, health YouTubeFeedHealth) (bool, error) {
	feeds, err := health.CountYouTubeFeedFailures(ctx)
	if err != nil {
		return false, err
	}
	return feeds.Failing >= youtubeOutageMinFailing && feeds.Failing*2 >= feeds.Total, nil
}
