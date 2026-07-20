package personsearch

import (
	"context"
	"log/slog"
)

// typeaheadLimit caps the AppView typeahead read; search is a convenience, not a browse.
const typeaheadLimit = 10

// maxTasteHints is the per-person title cap the badge shows (SPEC <discovery> People "Search").
const maxTasteHints = 2

// TypeaheadFetcher is the AppView-side dependency; *AppView satisfies it.
type TypeaheadFetcher interface {
	SearchActorsTypeahead(ctx context.Context, q string, limit int) ([]Actor, error)
}

// PresenceReader is the reader-network-side dependency, implemented later over
// sqlc. Both reads are structurally save-free (SPEC <discovery> People save
// privacy): presence ignores save-only signals, hints draw from subscribe/author rows only.
type PresenceReader interface {
	Presence(ctx context.Context, dids []string) (map[string]bool, error)
	TasteHints(ctx context.Context, did string, max int) ([]string, error)
}

// Result is one AppView hit badged with reader-network presence.
type Result struct {
	DID             string
	Handle          string
	DisplayName     string
	Avatar          string
	InReaderNetwork bool
	TasteHint       []string // <=2 titles, only for people present in the reader network.
}

// Searcher runs whole-network person search: AppView typeahead, badged and
// partitioned by reader-network presence (SPEC <discovery> People "Search").
type Searcher struct {
	typeahead TypeaheadFetcher
	presence  PresenceReader
}

func NewSearcher(typeahead TypeaheadFetcher, presence PresenceReader) *Searcher {
	return &Searcher{typeahead: typeahead, presence: presence}
}

// Search runs the AppView typeahead, then partitions present people first with
// AppView order preserved inside each group, hanging a taste hint on present
// results. Empty q is the caller's problem (the handler 400s); a typeahead
// error propagates (the handler 502s). A presence-read failure degrades every
// result to absent rather than erroring: a followable list still helps, and an
// absent badge never leaks who saves.
func (s *Searcher) Search(ctx context.Context, q string) ([]Result, error) {
	actors, err := s.typeahead.SearchActorsTypeahead(ctx, q, typeaheadLimit)
	if err != nil {
		return nil, err
	}

	dids := make([]string, len(actors))
	for i, a := range actors {
		dids[i] = a.DID
	}

	present, err := s.presence.Presence(ctx, dids)
	if err != nil {
		slog.Warn("personsearch: presence read failed, badging all absent", "err", err)
		present = nil
	}

	var here, absent []Result
	for _, a := range actors {
		r := Result{
			DID:             a.DID,
			Handle:          a.Handle,
			DisplayName:     a.DisplayName,
			Avatar:          a.Avatar,
			InReaderNetwork: present[a.DID],
		}
		if r.InReaderNetwork {
			r.TasteHint = s.hintsFor(ctx, a.DID)
			here = append(here, r)
		} else {
			absent = append(absent, r)
		}
	}
	return append(here, absent...), nil
}

// hintsFor reads a present person's taste hint, capped at maxTasteHints. A hint
// is a convenience, never a gate: a read failure yields no hint, not an error.
func (s *Searcher) hintsFor(ctx context.Context, did string) []string {
	hints, err := s.presence.TasteHints(ctx, did, maxTasteHints)
	if err != nil {
		slog.Warn("personsearch: taste hint read failed", "did", did, "err", err)
		return nil
	}
	if len(hints) > maxTasteHints {
		hints = hints[:maxTasteHints]
	}
	return hints
}
