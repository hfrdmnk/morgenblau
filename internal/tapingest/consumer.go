package tapingest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/coder/websocket"

	"morgenblau/internal/backoff"
	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
)

// maxRecordBytes lifts coder/websocket's 32 KiB default read limit: a single atproto record can reach a couple of MiB, and one oversized frame would otherwise kill the connection.
const maxRecordBytes = 4 << 20

const defaultPingInterval = 30 * time.Second

// TapStore is the consumer's write surface over the record mirror; *db.Queries satisfies it.
type TapStore interface {
	UpsertTapRecord(ctx context.Context, arg db.UpsertTapRecordParams) error
	DeleteTapRecord(ctx context.Context, arg db.DeleteTapRecordParams) error
	MarkTapRepoDirty(ctx context.Context, arg db.MarkTapRepoDirtyParams) error
}

// Consumer holds the single websocket session to the tap sidecar and mirrors its record stream into SQLite.
// Exactly one Consumer may run per process: tap shards events across concurrent sockets, so a second one would silently split the stream.
type Consumer struct {
	channelURL   string
	httpClient   *http.Client
	runTx        func(ctx context.Context, fn func(TapStore) error) error
	policy       backoff.Policy
	pingInterval time.Duration
	now          func() time.Time

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewConsumer builds a consumer against tap's base URL (e.g. http://localhost:2480). Callers must chain WithTxRunner; without one every event fails and stays unacked.
func NewConsumer(tapURL string) *Consumer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Consumer{
		channelURL:   channelURL(tapURL),
		httpClient:   http.DefaultClient,
		policy:       backoff.Policy{Steps: backoff.Exponential(time.Second, 2, time.Minute)},
		pingInterval: defaultPingInterval,
		now:          time.Now,
		ctx:          ctx,
		cancel:       cancel,
		runTx: func(ctx context.Context, fn func(TapStore) error) error {
			return errors.New("tapingest: no transaction runner configured (call WithTxRunner)")
		},
	}
}

// WithTxRunner commits each event's mirror write in one transaction on the writer pool.
func (c *Consumer) WithTxRunner(w *sql.DB) *Consumer {
	c.runTx = func(ctx context.Context, fn func(TapStore) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return c
}

// channelURL maps tap's base URL onto its websocket endpoint; config may carry either the http or the ws form. An unparseable value passes through so Dial reports it once per attempt instead of failing silently at construction.
func channelURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	switch u.Scheme {
	case "http", "ws":
		u.Scheme = "ws"
	case "https", "wss":
		u.Scheme = "wss"
	default:
		return raw
	}
	u.Path = "/channel"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// Start launches the session goroutine. Not safe to call more than once.
func (c *Consumer) Start() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.loop()
	}()
}

// Shutdown cancels the session and waits for it to drain, or for ctx to expire.
func (c *Consumer) Shutdown(ctx context.Context) error {
	c.cancel()
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// loop keeps one session alive. Tap being down degrades discovery to stale signals; it must never take the app down with it.
func (c *Consumer) loop() {
	failures := 0
	for {
		if c.ctx.Err() != nil {
			return
		}
		if delay := c.policy.Delay(failures); delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-c.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		// Any event that arrived proves the connection worked, so the ladder restarts from the bottom.
		if c.session() {
			failures = 0
		} else {
			failures++
		}
	}
}

// session runs one connection to exhaustion and reports whether it delivered anything.
func (c *Consumer) session() bool {
	ctx, cancel := context.WithCancel(c.ctx)
	conn, _, err := websocket.Dial(ctx, c.channelURL, &websocket.DialOptions{HTTPClient: c.httpClient})
	if err != nil {
		cancel()
		if c.ctx.Err() == nil {
			slog.Warn("tapingest: tap dial failed", "url", c.channelURL, "err", err)
		}
		return false
	}
	conn.SetReadLimit(maxRecordBytes)

	var pings sync.WaitGroup
	pings.Add(1)
	go func() {
		defer pings.Done()
		c.pingLoop(ctx, conn)
	}()
	// Cancel before waiting, and close only once the ping goroutine is done touching the connection.
	defer func() {
		cancel()
		pings.Wait()
		conn.CloseNow()
	}()

	delivered := false
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			if c.ctx.Err() == nil {
				slog.Warn("tapingest: tap read failed", "err", err)
			}
			return delivered
		}
		delivered = true
		if kind != websocket.MessageText {
			slog.Debug("tapingest: ignoring non-text tap frame", "kind", kind.String())
			continue
		}
		c.handle(ctx, conn, data)
	}
}

// pingLoop supplies the liveness probe tap never sends, so a silently dead connection is noticed instead of blocking Read forever.
func (c *Consumer) pingLoop(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, c.pingInterval)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				if ctx.Err() == nil {
					slog.Warn("tapingest: tap ping failed, dropping the connection", "err", err)
					conn.CloseNow()
				}
				return
			}
		}
	}
}

// handle mirrors one envelope and acks it. An event is acked once it can never be processed any better, or once its write has committed; anything else is left for tap to redeliver.
func (c *Consumer) handle(ctx context.Context, conn *websocket.Conn, data []byte) {
	env, err := parseEvent(data)
	if err != nil {
		// Redelivery cannot make malformed JSON parse, so acking it is the only way to unblock that DID's stream.
		slog.Warn("tapingest: malformed tap envelope", "id", env.ID, "err", err)
		c.ack(ctx, conn, env.ID)
		return
	}

	switch env.Type {
	case eventTypeRecord:
		ev := env.Record
		if ev == nil || ev.DID == "" || ev.Collection == "" || ev.Rkey == "" || !knownAction(ev.Action) {
			slog.Warn("tapingest: skipping unusable record event", "id", env.ID)
			break
		}
		if err := c.apply(ctx, *ev); err != nil {
			slog.Warn("tapingest: mirror write failed, leaving the event unacked", "did", ev.DID, "collection", ev.Collection, "err", err)
			return
		}
	case eventTypeIdentity:
		slog.Debug("tapingest: identity event", "id", env.ID)
	default:
		slog.Debug("tapingest: ignoring unknown tap event type", "type", env.Type, "id", env.ID)
	}
	c.ack(ctx, conn, env.ID)
}

// apply writes one record change and its dirty mark in a single transaction, so the rebuild worker never sees a mirror change without the mark that schedules it.
func (c *Consumer) apply(ctx context.Context, ev RecordEvent) error {
	var payload string
	if ev.Action != actionDelete && len(ev.Record) > 0 && !bytes.Equal(ev.Record, []byte("null")) {
		compacted, err := compactJSON(ev.Record)
		if err != nil {
			return err
		}
		payload = compacted
	}
	stamp := c.now().UTC().Format(time.RFC3339)

	return c.runTx(ctx, func(s TapStore) error {
		switch {
		case ev.Action == actionDelete:
			if err := s.DeleteTapRecord(ctx, db.DeleteTapRecordParams{
				Did:        ev.DID,
				Collection: ev.Collection,
				Rkey:       ev.Rkey,
			}); err != nil {
				return err
			}
		case payload != "":
			if err := s.UpsertTapRecord(ctx, db.UpsertTapRecordParams{
				Did:        ev.DID,
				Collection: ev.Collection,
				Rkey:       ev.Rkey,
				Cid:        ev.CID,
				Record:     payload,
				IndexedAt:  stamp,
			}); err != nil {
				return err
			}
		}
		// A body-less create means tap could not decode the record; the repo still gets marked so the rebuild reconciles from whatever is mirrored.
		return s.MarkTapRepoDirty(ctx, db.MarkTapRepoDirtyParams{Did: ev.DID, MarkedAt: stamp})
	})
}

// ack releases the per-DID barrier. Id 0 means the envelope was too broken to read an id from, so there is nothing to release.
func (c *Consumer) ack(ctx context.Context, conn *websocket.Conn, id uint64) {
	if id == 0 {
		return
	}
	payload, err := json.Marshal(ackMessage{Type: "ack", ID: id})
	if err != nil {
		slog.Warn("tapingest: ack encode failed", "id", id, "err", err)
		return
	}
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil && ctx.Err() == nil {
		slog.Warn("tapingest: ack write failed", "id", id, "err", err)
	}
}

// compactJSON strips transport whitespace without reordering keys, keeping the mirrored bytes byte-comparable to what the PDS serves.
func compactJSON(raw json.RawMessage) (string, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return "", err
	}
	return buf.String(), nil
}
