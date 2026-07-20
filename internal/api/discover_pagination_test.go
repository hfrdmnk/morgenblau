package api

import (
	"bytes"
	"testing"
	"time"

	"morgenblau/internal/discoverrank"
)

func TestDiscoverCursorRoundTripRejectsWrongKind(t *testing.T) {
	now := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	cursor, err := newDiscoverCursor("sources", now, bytes.NewReader([]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	if err != nil {
		t.Fatalf("new cursor: %v", err)
	}
	cursor.Personal.After = &discoverrank.Position{Band: 4, Shuffle: 12, Key: "https://source.example/feed"}

	encoded, err := encodeDiscoverCursor(cursor)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	decoded, err := decodeDiscoverCursor(encoded, "sources")
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if decoded.Kind != "sources" || decoded.Seed != 0x0807060504030201 || decoded.RankedAt != now.UnixNano() {
		t.Fatalf("decoded cursor = %+v", decoded)
	}
	if decoded.Personal.After == nil || *decoded.Personal.After != *cursor.Personal.After {
		t.Fatalf("decoded personal position = %+v, want %+v", decoded.Personal.After, cursor.Personal.After)
	}
	if _, err := decodeDiscoverCursor(encoded, "people"); err == nil {
		t.Fatal("wrong-kind cursor decoded successfully")
	}
}

func TestBalanceDiscoverPagesUsesFourFromEachPool(t *testing.T) {
	cursor := discoverCursor{Version: 1, Kind: "sources", Seed: 1, RankedAt: 1}
	personal := rankedStringPage("personal", 8, true)
	trending := rankedStringPage("trending", 8, true)

	page := balanceDiscoverPages(cursor, personal, trending)
	if len(page.Personal) != 4 || len(page.Trending) != 4 {
		t.Fatalf("balanced page = %d personal + %d trending, want 4 + 4", len(page.Personal), len(page.Trending))
	}
	if page.Cursor.Personal.After == nil || page.Cursor.Personal.After.Key != "personal-3" {
		t.Fatalf("personal cursor = %+v, want personal-3", page.Cursor.Personal.After)
	}
	if page.Cursor.Trending.After == nil || page.Cursor.Trending.After.Key != "trending-3" {
		t.Fatalf("trending cursor = %+v, want trending-3", page.Cursor.Trending.After)
	}
	if !page.HasMore {
		t.Fatal("HasMore = false, want true")
	}
}

func TestBalanceDiscoverPagesFillsFromTheOtherPool(t *testing.T) {
	cursor := discoverCursor{Version: 1, Kind: "people", Seed: 1, RankedAt: 1}
	personal := rankedStringPage("personal", 1, false)
	trending := rankedStringPage("trending", 8, false)

	page := balanceDiscoverPages(cursor, personal, trending)
	if len(page.Personal) != 1 || len(page.Trending) != 7 {
		t.Fatalf("filled page = %d personal + %d trending, want 1 + 7", len(page.Personal), len(page.Trending))
	}
	if !page.Cursor.Personal.Done {
		t.Fatal("personal pool not marked done")
	}
	if page.Cursor.Trending.Done {
		t.Fatal("trending pool marked done before its eighth item")
	}
	if !page.HasMore {
		t.Fatal("HasMore = false, want the remaining trending item")
	}
}

func TestBalanceDiscoverPagesMarksBothPoolsExhausted(t *testing.T) {
	cursor := discoverCursor{Version: 1, Kind: "sources", Seed: 1, RankedAt: 1}
	page := balanceDiscoverPages(
		cursor,
		rankedStringPage("personal", 2, false),
		rankedStringPage("trending", 3, false),
	)
	if len(page.Personal)+len(page.Trending) != 5 {
		t.Fatalf("page size = %d, want 5", len(page.Personal)+len(page.Trending))
	}
	if page.HasMore || !page.Cursor.Personal.Done || !page.Cursor.Trending.Done {
		t.Fatalf("page cursor = %+v, HasMore = %v, want both pools exhausted", page.Cursor, page.HasMore)
	}
}

func rankedStringPage(prefix string, count int, hasMore bool) discoverrank.Page[string] {
	items := make([]discoverrank.Ranked[string], count)
	for i := range items {
		key := prefix + "-" + string(rune('0'+i))
		items[i] = discoverrank.Ranked[string]{
			Value:    key,
			Position: discoverrank.Position{Band: 1, Shuffle: uint64(i), Key: key},
		}
	}
	return discoverrank.Page[string]{Items: items, HasMore: hasMore}
}
