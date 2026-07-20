package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"morgenblau/internal/discoverrank"
)

const (
	discoverPageSize   = 8
	discoverPoolTarget = 4
)

type discoverPoolCursor struct {
	After *discoverrank.Position `json:"a,omitempty"`
	Done  bool                   `json:"d,omitempty"`
}

type discoverCursor struct {
	Version  int                `json:"v"`
	Kind     string             `json:"k"`
	Seed     int64              `json:"s"`
	RankedAt int64              `json:"t"`
	Personal discoverPoolCursor `json:"p"`
	Trending discoverPoolCursor `json:"g"`
}

type discoverPageWire[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type balancedDiscoverPage[T any] struct {
	Personal []discoverrank.Ranked[T]
	Trending []discoverrank.Ranked[T]
	Cursor   discoverCursor
	HasMore  bool
}

func newDiscoverCursor(kind string, now time.Time, random io.Reader) (discoverCursor, error) {
	var seedBytes [8]byte
	if _, err := io.ReadFull(random, seedBytes[:]); err != nil {
		return discoverCursor{}, err
	}
	return discoverCursor{
		Version:  1,
		Kind:     kind,
		Seed:     int64(binary.LittleEndian.Uint64(seedBytes[:])),
		RankedAt: now.UnixNano(),
	}, nil
}

func encodeDiscoverCursor(cursor discoverCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeDiscoverCursor(raw, kind string) (discoverCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return discoverCursor{}, err
	}
	var cursor discoverCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return discoverCursor{}, err
	}
	if cursor.Version != 1 || cursor.Kind != kind || cursor.RankedAt <= 0 {
		return discoverCursor{}, errors.New("invalid discover cursor")
	}
	for _, pool := range []discoverPoolCursor{cursor.Personal, cursor.Trending} {
		if pool.After != nil && pool.After.Key == "" {
			return discoverCursor{}, errors.New("invalid discover cursor position")
		}
	}
	return cursor, nil
}

func discoverCursorFromRequest(w http.ResponseWriter, r *http.Request, kind string) (discoverCursor, bool) {
	raw := r.URL.Query().Get("cursor")
	if raw != "" {
		cursor, err := decodeDiscoverCursor(raw, kind)
		if err != nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "invalid cursor")
			return discoverCursor{}, false
		}
		return cursor, true
	}

	cursor, err := newDiscoverCursor(kind, time.Now().UTC(), rand.Reader)
	if err != nil {
		slog.Error("discover cursor seed failed", "kind", kind, "err", err)
		writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
		return discoverCursor{}, false
	}
	return cursor, true
}

func discoverNextCursor(cursor discoverCursor, hasMore bool) (string, error) {
	if !hasMore {
		return "", nil
	}
	return encodeDiscoverCursor(cursor)
}

func balanceDiscoverPages[T any](cursor discoverCursor, personal, trending discoverrank.Page[T]) balancedDiscoverPage[T] {
	personalCount := min(discoverPoolTarget, len(personal.Items))
	trendingCount := min(discoverPoolTarget, len(trending.Items))
	remaining := discoverPageSize - personalCount - trendingCount

	personalExtra := min(remaining, len(personal.Items)-personalCount)
	personalCount += personalExtra
	remaining -= personalExtra
	trendingCount += min(remaining, len(trending.Items)-trendingCount)

	selectedPersonal := personal.Items[:personalCount]
	selectedTrending := trending.Items[:trendingCount]
	cursor.Personal = advanceDiscoverPool(cursor.Personal, selectedPersonal, personal)
	cursor.Trending = advanceDiscoverPool(cursor.Trending, selectedTrending, trending)

	return balancedDiscoverPage[T]{
		Personal: selectedPersonal,
		Trending: selectedTrending,
		Cursor:   cursor,
		HasMore:  !cursor.Personal.Done || !cursor.Trending.Done,
	}
}

func advanceDiscoverPool[T any](cursor discoverPoolCursor, selected []discoverrank.Ranked[T], page discoverrank.Page[T]) discoverPoolCursor {
	if len(selected) > 0 {
		position := selected[len(selected)-1].Position
		cursor.After = &position
	}
	cursor.Done = cursor.Done || (len(selected) == len(page.Items) && !page.HasMore)
	return cursor
}
