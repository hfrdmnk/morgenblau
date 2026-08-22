package discoveringest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"morgenblau/internal/backoff"
	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
)

// subscribePath is the v2 stream's lexicon-canonical XRPC path. The legacy /subscribe wire never emits #sync, which a folding consumer needs to repair a diverged repo.
const subscribePath = "/xrpc/network.bsky.jetstream.subscribeEvents"

// statusDeleted is the only account status that purges a repo's records; every other inactive status retains them for reactivation.
const statusDeleted = "deleted"

// maxRecordBytes lifts coder/websocket's 32 KiB default read limit: a single atproto record can reach a couple of MiB, and one oversized frame would otherwise kill the connection.
const maxRecordBytes = 4 << 20

const (
	defaultPingInterval = 30 * time.Second
	// The cursor is a restart hint, not a correctness boundary: mirror writes are idempotent upserts, so redelivery after a crash costs at most this much replay.
	defaultCursorFlushEvents   = 500
	defaultCursorFlushInterval = 5 * time.Second
)

// Config is the operator-configured Jetstream instance. APIKey is optional and only the metered Replay HTTP endpoints require one.
type Config struct {
	URL    string
	APIKey string
}

// CursorReader reads the persisted stream position; wire it to the reader pool.
type CursorReader interface {
	GetDiscoverIngestCursor(ctx context.Context) (db.GetDiscoverIngestCursorRow, error)
}

// RecordFetcher re-reads one repo's tracked collections from its PDS. Only a #sync divergence marker needs it, and it must never run inside a transaction.
type RecordFetcher interface {
	FetchRepoRecords(ctx context.Context, did string) ([]MirrorRecord, error)
}

// position is the persisted stream position. The bootstrap fields are set only while a Replay backfill is unfinished, which is what tells a restart apart from a first run.
type position struct {
	Seq              int64
	BootstrapTip     int64
	BootstrapThrough int64
	Known            bool
}

func (p position) needsBootstrap() bool { return !p.Known || p.BootstrapTip > 0 }

func (p position) params(stamp string) db.UpsertDiscoverIngestCursorParams {
	out := db.UpsertDiscoverIngestCursorParams{Seq: p.Seq, UpdatedAt: stamp}
	if p.BootstrapTip > 0 {
		tip, through := p.BootstrapTip, p.BootstrapThrough
		out.BootstrapTipSeq = &tip
		out.BootstrapThroughSeq = &through
	}
	return out
}

// Consumer holds the single Jetstream session that mirrors the reader network's records into SQLite, bootstrapping from the Replay archive whenever it has no usable cursor.
// Exactly one Consumer may run per process: two would double-apply and fight over the shared cursor row.
type Consumer struct {
	cfg          Config
	httpClient   *http.Client
	cursors      CursorReader
	records      RecordFetcher
	runTx        func(ctx context.Context, fn func(MirrorStore) error) error
	policy       backoff.Policy
	pingInterval time.Duration
	now          func() time.Time

	cursorFlushEvents   int
	cursorFlushInterval time.Duration

	// Only the session goroutine touches the fields below.
	rebootstrap bool
	lastSeq     int64
	pending     int
	flushedAt   time.Time

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewConsumer builds a consumer against a Jetstream instance's base URL. Callers must chain WithTxRunner; without one every event fails and the stream never advances.
func NewConsumer(cfg Config, cursors CursorReader, records RecordFetcher) *Consumer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Consumer{
		cfg: cfg,
		// No client timeout: coder/websocket turns one into a deadline on the whole session, which would kill the tail seconds after it connects. The Replay client applies its own per-request deadlines.
		httpClient:          &http.Client{},
		cursors:             cursors,
		records:             records,
		policy:              backoff.Policy{Steps: backoff.Exponential(time.Second, 2, time.Minute)},
		pingInterval:        defaultPingInterval,
		now:                 time.Now,
		cursorFlushEvents:   defaultCursorFlushEvents,
		cursorFlushInterval: defaultCursorFlushInterval,
		ctx:                 ctx,
		cancel:              cancel,
		runTx: func(ctx context.Context, fn func(MirrorStore) error) error {
			return errors.New("discoveringest: no transaction runner configured (call WithTxRunner)")
		},
	}
}

// WithTxRunner commits each event's mirror write in one transaction on the writer pool.
func (c *Consumer) WithTxRunner(w *sql.DB) *Consumer {
	c.runTx = func(ctx context.Context, fn func(MirrorStore) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return c
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

// loop keeps one session alive. Jetstream being unreachable degrades discovery to stale signals; it must never take the app down with it.
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
		if c.cycle() {
			failures = 0
		} else {
			failures++
		}
	}
}

// cycle runs one bootstrap-then-tail pass and reports whether it made progress. Bootstrap is entirely internal: wiring never learns the Replay archive exists.
func (c *Consumer) cycle() bool {
	pos, err := c.position(c.ctx)
	if err != nil {
		slog.Warn("discoveringest: stream position read failed", "err", err)
		return false
	}
	if pos.needsBootstrap() || c.rebootstrap {
		seq, err := c.bootstrap(c.ctx, pos)
		if err != nil {
			if c.ctx.Err() == nil {
				slog.Warn("discoveringest: replay bootstrap failed", "err", err)
			}
			return false
		}
		c.rebootstrap = false
		pos = position{Seq: seq, Known: true}
	}
	return c.tail(pos.Seq)
}

func (c *Consumer) position(ctx context.Context) (position, error) {
	row, err := c.cursors.GetDiscoverIngestCursor(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return position{}, nil
	}
	if err != nil {
		return position{}, err
	}
	pos := position{Seq: row.Seq, Known: true}
	if row.BootstrapTipSeq != nil {
		pos.BootstrapTip = *row.BootstrapTipSeq
	}
	if row.BootstrapThroughSeq != nil {
		pos.BootstrapThrough = *row.BootstrapThroughSeq
	}
	return pos, nil
}

// tail runs one live connection to exhaustion and reports whether it delivered anything.
func (c *Consumer) tail(seq int64) bool {
	ctx, cancel := context.WithCancel(c.ctx)
	target := c.subscribeURL(seq)
	conn, resp, err := websocket.Dial(ctx, target, &websocket.DialOptions{
		HTTPClient:      c.httpClient,
		HTTPHeader:      c.authHeader(),
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		cancel()
		if cursorTooOld(resp) {
			// Re-entering the plan loop is the only way to cover the (cursor, floor] gap the server just refused to replay.
			slog.Warn("discoveringest: jetstream cursor aged out, re-entering the replay archive", "seq", seq)
			c.rebootstrap = true
		} else if c.ctx.Err() == nil {
			slog.Warn("discoveringest: jetstream dial failed", "url", target, "err", err)
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
		c.flushCursor(c.ctx)
	}()

	c.lastSeq = seq
	delivered := false
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			if c.ctx.Err() == nil {
				slog.Warn("discoveringest: jetstream read failed", "err", err)
			}
			return delivered
		}
		delivered = true
		if kind != websocket.MessageText {
			continue
		}
		ev, err := decodeFrame(data)
		if err != nil {
			var terminal *streamError
			if errors.As(err, &terminal) {
				slog.Warn("discoveringest: jetstream ended the stream", "name", terminal.Name, "message", terminal.Message)
				return delivered
			}
			slog.Warn("discoveringest: malformed jetstream frame", "err", err)
			continue
		}
		if err := c.applyEvent(ctx, ev); err != nil {
			// There is no per-event ack, so the only way not to lose the event is to drop the connection and let the redial resume from the last persisted cursor.
			if c.ctx.Err() == nil {
				slog.Warn("discoveringest: mirror write failed, resuming from the persisted cursor", "seq", ev.Seq, "kind", ev.Kind, "err", err)
			}
			return delivered
		}
		c.advance(ctx, ev.Seq)
	}
}

// advance records progress and flushes it on the event-count or time pace, whichever trips first. A #info frame is seq-less, and treating its zero as a position would rewind the stream to the archive floor.
func (c *Consumer) advance(ctx context.Context, seq int64) {
	if seq <= 0 {
		return
	}
	c.lastSeq = seq
	c.pending++
	if c.pending < c.cursorFlushEvents && c.now().Sub(c.flushedAt) < c.cursorFlushInterval {
		return
	}
	c.flushCursor(ctx)
}

func (c *Consumer) flushCursor(ctx context.Context) {
	if c.pending == 0 || c.lastSeq <= 0 {
		return
	}
	if err := c.persistPosition(ctx, position{Seq: c.lastSeq, Known: true}); err != nil {
		if ctx.Err() == nil {
			slog.Warn("discoveringest: cursor write failed", "seq", c.lastSeq, "err", err)
		}
		return
	}
	c.pending = 0
	c.flushedAt = c.now()
}

// pingLoop supplies a liveness probe so a silently dead connection is noticed instead of blocking Read forever.
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
					slog.Warn("discoveringest: jetstream ping failed, dropping the connection", "err", err)
					conn.CloseNow()
				}
				return
			}
		}
	}
}

// subscribeURL builds the v2 subscribe URL. kinds is deliberately omitted: a collections filter constrains commits only, and the account/sync markers it lets through are what purge a dead repo.
func (c *Consumer) subscribeURL(seq int64) string {
	u, err := url.Parse(c.cfg.URL)
	if err != nil {
		return c.cfg.URL
	}
	switch u.Scheme {
	case "http", "ws":
		u.Scheme = "ws"
	case "https", "wss":
		u.Scheme = "wss"
	default:
		return c.cfg.URL
	}
	q := url.Values{}
	for _, coll := range Collections {
		q.Add("collections", coll)
	}
	if seq > 0 {
		q.Set("cursor", strconv.FormatInt(seq, 10))
	}
	u.Path = subscribePath
	u.RawQuery = q.Encode()
	u.Fragment = ""
	return u.String()
}

func (c *Consumer) authHeader() http.Header {
	if c.cfg.APIKey == "" {
		return nil
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+c.cfg.APIKey)
	return h
}

// archive builds the Replay client on demand so it always picks up the consumer's current HTTP client.
func (c *Consumer) archive() *replayClient {
	return &replayClient{base: strings.TrimSuffix(c.cfg.URL, "/"), http: c.httpClient, apiKey: c.cfg.APIKey}
}

// cursorTooOld reads the pre-upgrade XRPC error envelope. coder/websocket buffers the first KiB of a failed handshake body, so the envelope is still readable here.
func cursorTooOld(resp *http.Response) bool {
	if resp == nil || resp.StatusCode != http.StatusBadRequest || resp.Body == nil {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	resp.Body.Close()
	if err != nil {
		return false
	}
	var envelope struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return false
	}
	return envelope.Error == "CursorTooOld"
}
