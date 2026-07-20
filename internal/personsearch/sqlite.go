package personsearch

import (
	"context"

	"morgenblau/internal/database/db"
)

// PresenceQuerier is the slice of *db.Queries SQLitePresenceReader reads from; wire to the reader pool.
type PresenceQuerier interface {
	PersonSearchPresence(ctx context.Context, dids []string) ([]string, error)
	PersonSearchTasteHints(ctx context.Context, arg db.PersonSearchTasteHintsParams) ([]*string, error)
}

// SQLitePresenceReader adapts the generated queries to PresenceReader.
type SQLitePresenceReader struct {
	q PresenceQuerier
}

func NewSQLitePresenceReader(q PresenceQuerier) *SQLitePresenceReader {
	return &SQLitePresenceReader{q: q}
}

// Presence folds the present-DID rows into a map; a DID absent from the query result is absent from the map (zero value false).
func (r *SQLitePresenceReader) Presence(ctx context.Context, dids []string) (map[string]bool, error) {
	rows, err := r.q.PersonSearchPresence(ctx, dids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, did := range rows {
		out[did] = true
	}
	return out, nil
}

func (r *SQLitePresenceReader) TasteHints(ctx context.Context, did string, max int) ([]string, error) {
	rows, err := r.q.PersonSearchTasteHints(ctx, db.PersonSearchTasteHintsParams{RepoDid: did, Limit: int64(max)})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, title := range rows {
		if title != nil {
			out = append(out, *title)
		}
	}
	return out, nil
}
