package tapingest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/discoverbatch"
)

const (
	// seedChunkSize bounds one /repos/add body, so a rejected request costs a chunk of the enumeration rather than all of it.
	seedChunkSize = 500
	// defaultTapTimeout bounds one /repos/add round trip; tap answers from local state, so a slow reply means it is wedged.
	defaultTapTimeout = 30 * time.Second
)

// SeedReader lists the repos tap has already been told to track; wire it to the reader pool.
type SeedReader interface {
	ListTapSeededDids(ctx context.Context) ([]string, error)
}

// SeedWriter records the repos one accepted chunk registered.
type SeedWriter interface {
	InsertTapSeededDid(ctx context.Context, arg db.InsertTapSeededDidParams) error
}

// NewTapClient deliberately skips safehttp: its SSRF guard blocks loopback, and tap is an operator-configured local sidecar rather than an attacker-influenced URL.
func NewTapClient() *http.Client {
	return &http.Client{Timeout: defaultTapTimeout}
}

// Seeder keeps tap's membership in step with the reader network: each run enumerates the relay and hands tap the repos it has not been told about yet.
type Seeder struct {
	addURL        string
	tapClient     *http.Client
	relayEndpoint string
	relayClient   *http.Client
	collections   []string
	reader        SeedReader
	runTx         func(ctx context.Context, fn func(SeedWriter) error) error
	chunkSize     int
	now           func() time.Time
}

// NewSeeder builds a Seeder. relayClient must be the SSRF-safe client; tapClient must not be (see NewTapClient). Callers must chain WithTxRunner; without one no repo is ever marked seeded, so every run re-posts the whole network.
func NewSeeder(tapURL string, tapClient *http.Client, relayHost string, relayClient *http.Client, reader SeedReader) *Seeder {
	collections := append(append([]string{}, discoverbatch.EnumerationCollections...), discoverbatch.FollowEnumerationCollections...)
	return &Seeder{
		addURL:        strings.TrimSuffix(tapURL, "/") + "/repos/add",
		tapClient:     tapClient,
		relayEndpoint: discoverbatch.NormalizeRelayHost(relayHost),
		relayClient:   relayClient,
		collections:   collections,
		reader:        reader,
		chunkSize:     seedChunkSize,
		now:           time.Now,
		runTx: func(ctx context.Context, fn func(SeedWriter) error) error {
			return errors.New("tapingest: no transaction runner configured (call WithTxRunner)")
		},
	}
}

// WithTxRunner commits one chunk's seeded marks in a single transaction on the writer pool.
func (s *Seeder) WithTxRunner(w *sql.DB) *Seeder {
	s.runTx = func(ctx context.Context, fn func(SeedWriter) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return s
}

// Run registers every enumerated repo tap does not already track and reports how many were added.
// A tap or bookkeeping failure abandons the rest of the run: /repos/add is idempotent and only accepted chunks are marked, so the next tick re-posts exactly what is still missing.
func (s *Seeder) Run(ctx context.Context) (int, error) {
	dids, err := discoverbatch.EnumerateAll(ctx, s.relayClient, s.relayEndpoint, s.collections)
	if err != nil {
		return 0, err
	}
	seeded, err := s.reader.ListTapSeededDids(ctx)
	if err != nil {
		return 0, fmt.Errorf("tapingest: read seeded repos: %w", err)
	}

	pending := unseeded(dids, seeded)
	added := 0
	for len(pending) > 0 {
		chunk := pending[:min(s.chunkSize, len(pending))]
		if err := s.addRepos(ctx, chunk); err != nil {
			if ctx.Err() != nil {
				return added, nil
			}
			slog.Warn("tapingest: tap repo registration failed, leaving the rest for the next run", "chunk", len(chunk), "pending", len(pending), "err", err)
			return added, nil
		}
		if err := s.markSeeded(ctx, chunk); err != nil {
			// Tap now tracks these repos but we failed to record it; stopping keeps the retry cheap, since re-posting a tracked repo is a no-op.
			slog.Warn("tapingest: seeded-state write failed, leaving the rest for the next run", "chunk", len(chunk), "err", err)
			return added, nil
		}
		added += len(chunk)
		pending = pending[len(chunk):]
	}
	return added, nil
}

// addRepos hands one chunk to tap. Tap treats an already-tracked DID as a no-op, so it never re-triggers a backfill.
func (s *Seeder) addRepos(ctx context.Context, dids []string) error {
	body, err := json.Marshal(reposAddRequest{DIDs: dids})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.addURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.tapClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Tap answers with an empty body; draining it anyway keeps the connection alive for the next chunk.
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("tapingest: tap %s returned %s", s.addURL, resp.Status)
	}
	return nil
}

func (s *Seeder) markSeeded(ctx context.Context, dids []string) error {
	stamp := s.now().UTC().Format(time.RFC3339)
	return s.runTx(ctx, func(w SeedWriter) error {
		for _, did := range dids {
			if err := w.InsertTapSeededDid(ctx, db.InsertTapSeededDidParams{Did: did, SeededAt: stamp}); err != nil {
				return err
			}
		}
		return nil
	})
}

type reposAddRequest struct {
	DIDs []string `json:"dids"`
}

// unseeded keeps enumeration order so an abandoned run resumes where it stopped instead of reshuffling what is still pending.
func unseeded(dids, seeded []string) []string {
	known := make(map[string]struct{}, len(seeded))
	for _, d := range seeded {
		known[d] = struct{}{}
	}
	out := make([]string, 0, len(dids))
	for _, d := range dids {
		if _, ok := known[d]; ok {
			continue
		}
		out = append(out, d)
	}
	return out
}
