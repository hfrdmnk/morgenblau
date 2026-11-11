package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"morgenblau/internal/database/db"
	"morgenblau/internal/youtube"
)

type fakeTranscripts struct {
	mu   sync.Mutex
	rows map[int64]db.GetVideoTranscriptRow
}

func (f *fakeTranscripts) GetVideoTranscript(_ context.Context, entryID int64) (db.GetVideoTranscriptRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[entryID]
	if !ok {
		return db.GetVideoTranscriptRow{}, sql.ErrNoRows
	}
	return row, nil
}

func (f *fakeTranscripts) UpsertVideoTranscript(_ context.Context, arg db.UpsertVideoTranscriptParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rows == nil {
		f.rows = map[int64]db.GetVideoTranscriptRow{}
	}
	f.rows[arg.EntryID] = db.GetVideoTranscriptRow{VideoID: arg.VideoID, Status: arg.Status, Transcript: arg.Transcript, CheckedAt: arg.CheckedAt}
	return nil
}

type fakeTranscriptFetcher struct {
	calls   atomic.Int32
	release chan struct{}
	result  youtube.Transcript
	err     error
}

func (f *fakeTranscriptFetcher) Fetch(_ context.Context, videoID string) (youtube.Transcript, error) {
	f.calls.Add(1)
	if videoID != "dQw4w9WgXcQ" {
		return youtube.Transcript{}, fmt.Errorf("fetched video %q, want the entry's own", videoID)
	}
	if f.release != nil {
		<-f.release
	}
	return f.result, f.err
}

var sampleTranscript = youtube.Transcript{Language: "en", Kind: youtube.KindManual, Cues: []youtube.Cue{
	{Start: 4220, Duration: 1180, Text: "This is a 3."},
	{Start: 6060, Duration: 4653, Text: "It is written at 28x28."},
}}

func videoEntryReader(publishedAt time.Time) *fakeEntryReader {
	entry := entryFixture()
	entry.Url = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
	entry.ContentType = "video"
	entry.PublishedAt = publishedAt.UTC().Format(time.RFC3339)
	return &fakeEntryReader{entry: entry, subOK: true, subscription: subscriptionFixture(nil), feed: feedFixture()}
}

func openTranscript(t *testing.T, reader EntryReader, store *fakeTranscripts, fetcher TranscriptFetcher) (*httptest.ResponseRecorder, transcriptWire) {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("POST /api/entries/{slug}/transcript", EntryTranscriptHandler(reader, store, store, fetcher))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, withSession(httptest.NewRequest(http.MethodPost, "/api/entries/abc1234567/transcript", nil), "did:plc:alice", "sid-1"))
	var got transcriptWire
	if rr.Code == http.StatusOK {
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatalf("body %s: %v", rr.Body, err)
		}
		if rr.Header().Get("Cache-Control") != "private, no-store" {
			t.Errorf("Cache-Control = %q, want private, no-store", rr.Header().Get("Cache-Control"))
		}
	}
	return rr, got
}

func TestEntryTranscript_FetchesOnceThenServesTheStoredTranscript(t *testing.T) {
	reader := videoEntryReader(time.Now().Add(-30 * 24 * time.Hour))
	store := &fakeTranscripts{}
	fetcher := &fakeTranscriptFetcher{result: sampleTranscript}

	for range 2 {
		rr, got := openTranscript(t, reader, store, fetcher)
		if rr.Code != http.StatusOK || got.Status != "ready" {
			t.Fatalf("status = %d %q, body = %s", rr.Code, got.Status, rr.Body)
		}
		if got.Language != "en" || got.Kind != youtube.KindManual || len(got.Cues) != 2 || got.Cues[1] != sampleTranscript.Cues[1] {
			t.Fatalf("transcript = %+v", got)
		}
	}
	if got := fetcher.calls.Load(); got != 1 {
		t.Fatalf("YouTube fetches = %d, want 1: a stored transcript is served without asking again", got)
	}
}

func TestEntryTranscript_NoTranscriptIsRecheckedOnlyAfterItsWindow(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name      string
		published time.Time
		checked   time.Time
		wantFetch bool
	}{
		{"new video inside six hours", now.Add(-5 * time.Hour), now.Add(-5 * time.Hour), false},
		{"new video after six hours", now.Add(-8 * time.Hour), now.Add(-7 * time.Hour), true},
		{"old video after six hours", now.Add(-30 * 24 * time.Hour), now.Add(-7 * time.Hour), false},
		{"old video inside a week", now.Add(-30 * 24 * time.Hour), now.Add(-6 * 24 * time.Hour), false},
		{"old video after a week", now.Add(-30 * 24 * time.Hour), now.Add(-8 * 24 * time.Hour), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reader := videoEntryReader(tc.published)
			store := &fakeTranscripts{rows: map[int64]db.GetVideoTranscriptRow{
				reader.entry.ID: {VideoID: "dQw4w9WgXcQ", Status: "none", CheckedAt: tc.checked.UTC().Format(time.RFC3339)},
			}}
			fetcher := &fakeTranscriptFetcher{err: youtube.ErrNoTranscript}

			rr, got := openTranscript(t, reader, store, fetcher)
			if rr.Code != http.StatusOK || got.Status != "none" {
				t.Fatalf("status = %d %q, body = %s", rr.Code, got.Status, rr.Body)
			}
			if fetched := fetcher.calls.Load() == 1; fetched != tc.wantFetch {
				t.Fatalf("fetched = %v, want %v", fetched, tc.wantFetch)
			}
		})
	}
}

func TestEntryTranscript_AStoredTranscriptOfAnotherVideoIsFetchedAgain(t *testing.T) {
	reader := videoEntryReader(time.Now().Add(-30 * 24 * time.Hour))
	old := `{"language":"de","kind":"manual","cues":[{"start":0,"duration":1000,"text":"Ein anderes Video."}]}`
	store := &fakeTranscripts{rows: map[int64]db.GetVideoTranscriptRow{
		reader.entry.ID: {VideoID: "aaaaaaaaaaa", Status: "ready", Transcript: &old, CheckedAt: time.Now().UTC().Format(time.RFC3339)},
	}}
	fetcher := &fakeTranscriptFetcher{result: sampleTranscript}

	rr, got := openTranscript(t, reader, store, fetcher)
	if rr.Code != http.StatusOK || got.Language != "en" || fetcher.calls.Load() != 1 {
		t.Fatalf("status = %d, language %q, fetches %d: the feed now links another video, so its transcript must be fetched", rr.Code, got.Language, fetcher.calls.Load())
	}
	if row := store.rows[reader.entry.ID]; row.VideoID != "dQw4w9WgXcQ" || row.Status != "ready" {
		t.Fatalf("stored %+v, want the entry's own video", row)
	}
}

func TestEntryTranscript_StoresNoTranscriptSoTheNextOpenSkipsYouTube(t *testing.T) {
	reader := videoEntryReader(time.Now().Add(-30 * 24 * time.Hour))
	store := &fakeTranscripts{}
	fetcher := &fakeTranscriptFetcher{err: fmt.Errorf("%w: no captions", youtube.ErrNoTranscript)}

	for range 2 {
		if rr, got := openTranscript(t, reader, store, fetcher); rr.Code != http.StatusOK || got.Status != "none" {
			t.Fatalf("status = %d %q, body = %s", rr.Code, got.Status, rr.Body)
		}
	}
	if got := fetcher.calls.Load(); got != 1 {
		t.Fatalf("YouTube fetches = %d, want 1", got)
	}
}

func TestEntryTranscript_FailedFetchAnswers502AndStoresNothing(t *testing.T) {
	for name, err := range map[string]error{
		"blocked":   fmt.Errorf("%w: bot check", youtube.ErrBlocked),
		"transient": fmt.Errorf("youtube: HTTP 503 from /youtubei/v1/player"),
	} {
		t.Run(name, func(t *testing.T) {
			reader := videoEntryReader(time.Now())
			store := &fakeTranscripts{}
			rr, _ := openTranscript(t, reader, store, &fakeTranscriptFetcher{err: err})
			if rr.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, want 502", rr.Code)
			}
			if len(store.rows) != 0 {
				t.Fatalf("stored %+v after a failed fetch; the next open must try again", store.rows)
			}
		})
	}
}

func TestEntryTranscript_NothingIsFetchedForAReaderWhoDoesNotFollowTheFeed(t *testing.T) {
	reader := videoEntryReader(time.Now())
	reader.subOK = false
	fetcher := &fakeTranscriptFetcher{result: sampleTranscript}
	rr, _ := openTranscript(t, reader, &fakeTranscripts{}, fetcher)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
	if fetcher.calls.Load() != 0 {
		t.Fatal("fetched a transcript for a reader who does not follow the feed")
	}
}

func TestEntryTranscript_AnEntryThatIsNotAYouTubeVideoHasNone(t *testing.T) {
	reader := videoEntryReader(time.Now())
	reader.entry.Url = "https://video.example.com/watch?v=dQw4w9WgXcQ"
	fetcher := &fakeTranscriptFetcher{result: sampleTranscript}
	rr, got := openTranscript(t, reader, &fakeTranscripts{}, fetcher)
	if rr.Code != http.StatusOK || got.Status != "none" {
		t.Fatalf("status = %d %q", rr.Code, got.Status)
	}
	if fetcher.calls.Load() != 0 {
		t.Fatal("asked YouTube about a URL that is not a YouTube video")
	}
}

func TestEntryTranscript_ConcurrentOpensShareOneFetch(t *testing.T) {
	reader := videoEntryReader(time.Now())
	store := &fakeTranscripts{}
	fetcher := &fakeTranscriptFetcher{result: sampleTranscript, release: make(chan struct{})}
	mux := http.NewServeMux()
	mux.Handle("POST /api/entries/{slug}/transcript", EntryTranscriptHandler(reader, store, store, fetcher))

	var wg sync.WaitGroup
	codes := make(chan int, 10)
	for range 10 {
		wg.Go(func() {
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, withSession(httptest.NewRequest(http.MethodPost, "/api/entries/abc1234567/transcript", nil), "did:plc:alice", "sid-1"))
			codes <- rr.Code
		})
	}
	// Let every request reach the shared fetch before YouTube answers.
	for fetcher.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	close(fetcher.release)
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != http.StatusOK {
			t.Errorf("status = %d, want 200", code)
		}
	}
	if got := fetcher.calls.Load(); got != 1 {
		t.Fatalf("YouTube fetches = %d, want 1", got)
	}
}
