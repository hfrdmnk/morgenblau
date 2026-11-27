package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/sync/singleflight"

	"morgenblau/internal/database/db"
	"morgenblau/internal/youtube"
)

type TranscriptReader interface {
	GetVideoTranscript(ctx context.Context, entryID int64) (db.GetVideoTranscriptRow, error)
}

type TranscriptWriter interface {
	UpsertVideoTranscript(ctx context.Context, arg db.UpsertVideoTranscriptParams) error
}

type TranscriptFetcher interface {
	Fetch(ctx context.Context, videoID string) (youtube.Transcript, error)
}

type transcriptWire struct {
	Status   string        `json:"status"`
	Language string        `json:"language,omitempty"`
	Kind     string        `json:"kind,omitempty"`
	Cues     []youtube.Cue `json:"cues,omitempty"`
}

const (
	transcriptNewVideoAge     = 48 * time.Hour
	transcriptNewVideoRecheck = 6 * time.Hour
	transcriptRecheck         = 7 * 24 * time.Hour
)

func EntryTranscriptHandler(reader EntryReader, store TranscriptReader, writer TranscriptWriter, fetcher TranscriptFetcher) http.Handler {
	var flight singleflight.Group
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entry, _, _, ok := loadAndAuthorize(w, r, reader)
		if !ok {
			return
		}
		privateResponse(w)
		videoID := youtube.VideoID(entry.Url)
		if videoID == "" {
			writeJSON(w, transcriptWire{Status: "none"})
			return
		}

		row, err := store.GetVideoTranscript(r.Context(), entry.ID)
		switch {
		case err == nil && row.VideoID == videoID && !transcriptStale(row, entry.PublishedAt, time.Now()):
			writeStoredTranscript(w, row)
			return
		case err != nil && !errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}

		ctx := context.WithoutCancel(r.Context())
		v, err, _ := flight.Do(strconv.FormatInt(entry.ID, 10), func() (any, error) {
			return fetchTranscript(ctx, fetcher, writer, entry.ID, videoID)
		})
		if err != nil {
			slog.Warn("/api/entries/{slug}/transcript: fetch failed", "video", videoID, "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "transcript unavailable")
			return
		}
		writeStoredTranscript(w, v.(db.GetVideoTranscriptRow))
	})
}

func fetchTranscript(ctx context.Context, fetcher TranscriptFetcher, writer TranscriptWriter, entryID int64, videoID string) (db.GetVideoTranscriptRow, error) {
	transcript, err := fetcher.Fetch(ctx, videoID)
	row := db.GetVideoTranscriptRow{VideoID: videoID, Status: "none", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	switch {
	case errors.Is(err, youtube.ErrNoTranscript):
	case err != nil:
		return db.GetVideoTranscriptRow{}, err
	default:
		raw, err := json.Marshal(transcript)
		if err != nil {
			return db.GetVideoTranscriptRow{}, err
		}
		stored := string(raw)
		row.Status = "ready"
		row.Transcript = &stored
	}
	if err := writer.UpsertVideoTranscript(ctx, db.UpsertVideoTranscriptParams{EntryID: entryID, VideoID: row.VideoID, Status: row.Status, Transcript: row.Transcript, CheckedAt: row.CheckedAt}); err != nil {
		slog.Warn("/api/entries/{slug}/transcript: persist failed", "video", videoID, "err", err)
	}
	return row, nil
}

func transcriptStale(row db.GetVideoTranscriptRow, publishedAt string, now time.Time) bool {
	if row.Status == "ready" {
		return false
	}
	checked, err := time.Parse(time.RFC3339, row.CheckedAt)
	if err != nil {
		return true
	}
	window := transcriptRecheck
	if published, err := time.Parse(time.RFC3339, publishedAt); err == nil && now.Sub(published) < transcriptNewVideoAge {
		window = transcriptNewVideoRecheck
	}
	return now.Sub(checked) >= window
}

func writeStoredTranscript(w http.ResponseWriter, row db.GetVideoTranscriptRow) {
	if row.Status != "ready" {
		writeJSON(w, transcriptWire{Status: "none"})
		return
	}
	var transcript youtube.Transcript
	if err := json.Unmarshal([]byte(*row.Transcript), &transcript); err != nil {
		writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
		return
	}
	writeJSON(w, transcriptWire{Status: "ready", Language: transcript.Language, Kind: transcript.Kind, Cues: transcript.Cues})
}
