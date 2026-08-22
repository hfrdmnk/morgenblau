package discoveringest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"morgenblau/internal/backoff"
)

const (
	planPath    = "/xrpc/network.bsky.jetstream.planSnapshot"
	blockPath   = "/xrpc/network.bsky.jetstream.getBlock"
	segmentPath = "/xrpc/network.bsky.jetstream.getSegment"
)

const (
	// planTimeout bounds one planner call; the planner runs over resident metadata and never opens segment files.
	planTimeout = 2 * time.Minute
	// blockTimeout bounds one block download, a few hundred KiB compressed.
	blockTimeout = 2 * time.Minute
	// segmentTimeout bounds a whole sealed segment, which reaches 256 MB.
	segmentTimeout = 30 * time.Minute
	// maxMeteringWaits bounds how many times one request will sit out a 429 before the bootstrap gives up and the reconnect ladder takes over.
	maxMeteringWaits = 3
	meteringFallback = 30 * time.Second
	maxMeteringWait  = 5 * time.Minute
	// maxPlanBytes bounds a planner response; a full-network plan reaches a few MB.
	maxPlanBytes = 64 << 20
)

// Plan segment download modes.
const (
	modeSegment = "segment"
	modeBlocks  = "blocks"
)

type blockRange struct {
	First int64 `json:"first"`
	Last  int64 `json:"last"`
}

type planSegment struct {
	Name   string       `json:"name"`
	Index  int64        `json:"index"`
	MinSeq int64        `json:"minSeq"`
	MaxSeq int64        `json:"maxSeq"`
	Mode   string       `json:"mode"`
	Blocks []blockRange `json:"blocks"`
}

type planInput struct {
	Collections []string `json:"collections,omitempty"`
	AfterSeq    int64    `json:"afterSeq,omitempty"`
	BeforeSeq   int64    `json:"beforeSeq,omitempty"`
}

type planOutput struct {
	SealedTipSeq      int64         `json:"sealedTipSeq"`
	PlannedThroughSeq int64         `json:"plannedThroughSeq"`
	Segments          []planSegment `json:"segments"`
}

// replayClient talks to Jetstream's sealed-archive endpoints. The host is operator-configured rather than attacker-influenceable, so this deliberately skips safehttp's SSRF gate.
type replayClient struct {
	base   string
	http   *http.Client
	apiKey string
}

// plan asks which sealed segments or block ranges may hold matching rows. Filters are one-sided: the planner never omits a match but may over-select, so the caller still applies the exact predicate.
func (r *replayClient) plan(ctx context.Context, in planInput) (planOutput, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return planOutput{}, err
	}
	resp, err := r.do(ctx, planTimeout, func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.base+planPath, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
	if err != nil {
		return planOutput{}, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, planPath); err != nil {
		return planOutput{}, err
	}
	var out planOutput
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPlanBytes)).Decode(&out); err != nil {
		return planOutput{}, fmt.Errorf("discoveringest: decode plan: %w", err)
	}
	return out, nil
}

// block fetches one block as the raw zstd frame the archive stores it as.
func (r *replayClient) block(ctx context.Context, segment string, index int64) ([]byte, error) {
	q := url.Values{"segment": {segment}, "blockIndex": {strconv.FormatInt(index, 10)}}
	resp, err := r.get(ctx, blockTimeout, blockPath, q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, blockPath); err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBlockBytes))
}

// segment opens a whole sealed .jss file. The caller must close the body; it is streamed rather than buffered because a segment reaches 256 MB.
func (r *replayClient) segment(ctx context.Context, name string) (io.ReadCloser, error) {
	resp, err := r.get(ctx, segmentTimeout, segmentPath, url.Values{"name": {name}})
	if err != nil {
		return nil, err
	}
	if err := checkStatus(resp, segmentPath); err != nil {
		resp.Body.Close()
		return nil, err
	}
	return resp.Body, nil
}

func (r *replayClient) get(ctx context.Context, timeout time.Duration, path string, q url.Values) (*http.Response, error) {
	target := r.base + path + "?" + q.Encode()
	return r.do(ctx, timeout, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	})
}

// do issues one archive request, waiting out a metering pause rather than failing the whole bootstrap for it. The returned response's body is still open and its timeout outlives the call, so a segment stream survives being read.
func (r *replayClient) do(ctx context.Context, timeout time.Duration, build func(context.Context) (*http.Request, error)) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		req, err := build(reqCtx)
		if err != nil {
			cancel()
			return nil, err
		}
		if r.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+r.apiKey)
		}
		resp, err := r.http.Do(req)
		if err != nil {
			cancel()
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt >= maxMeteringWaits {
			resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
			return resp, nil
		}
		wait, ok := backoff.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		resp.Body.Close()
		cancel()
		if !ok || wait > maxMeteringWait {
			wait = meteringFallback
		}
		slog.Info("discoveringest: replay byte quota exhausted, waiting it out", "wait", wait)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// cancelOnClose keeps a request's deadline alive for as long as its body is being read, and releases it when the caller closes.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func checkStatus(resp *http.Response, path string) error {
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
	return fmt.Errorf("discoveringest: %s returned %s: %s", path, resp.Status, bytes.TrimSpace(body))
}

// bootstrap fills the mirror from the sealed archive and returns the seq the live tail must resume from. It is resumable: progress is persisted per plan segment, and every write it makes is an idempotent upsert.
func (c *Consumer) bootstrap(ctx context.Context, pos position) (int64, error) {
	archive := c.archive()
	decoder, err := newArchiveDecoder()
	if err != nil {
		return 0, err
	}
	defer decoder.Close()

	live, after, tip := pos.Seq, pos.BootstrapThrough, pos.BootstrapTip
	if tip == 0 {
		// The first page pins the tip for the whole backfill, so the range never floats as new data seals behind it.
		after = pos.Seq
		page, err := archive.plan(ctx, planInput{Collections: Collections, AfterSeq: after})
		if err != nil {
			return 0, err
		}
		tip = page.SealedTipSeq
		if tip <= after {
			slog.Info("discoveringest: sealed archive holds nothing new, tailing from the cursor", "seq", after)
			return after, nil
		}
		slog.Info("discoveringest: bootstrapping from the replay archive", "after", after, "tip", tip, "segments", len(page.Segments))
		after, err = c.applyPlan(ctx, archive, decoder, page, live, after, tip)
		if err != nil {
			return 0, err
		}
	}

	for after < tip {
		page, err := archive.plan(ctx, planInput{Collections: Collections, AfterSeq: after, BeforeSeq: tip})
		if err != nil {
			return 0, err
		}
		next, err := c.applyPlan(ctx, archive, decoder, page, live, after, tip)
		if err != nil {
			return 0, err
		}
		if next <= after {
			return 0, fmt.Errorf("discoveringest: replay plan made no progress past seq %d", after)
		}
		after = next
	}

	// The live cursor is inclusive, so the tip is replayed once more and folded away by the idempotent upserts.
	if err := c.persistPosition(ctx, position{Seq: tip, Known: true}); err != nil {
		return 0, err
	}
	slog.Info("discoveringest: replay bootstrap complete, cutting over to the live tail", "seq", tip)
	return tip, nil
}

// applyPlan downloads and folds one plan page, persisting progress after each segment so an interrupted page resumes near where it stopped rather than re-downloading the whole plan.
func (c *Consumer) applyPlan(ctx context.Context, archive *replayClient, decoder *archiveDecoder, page planOutput, live, after, tip int64) (int64, error) {
	for _, seg := range page.Segments {
		if err := c.applySegment(ctx, archive, decoder, seg, after, tip); err != nil {
			return 0, err
		}
		if seg.MaxSeq > after && seg.MaxSeq < page.PlannedThroughSeq {
			if err := c.persistPosition(ctx, position{Seq: live, BootstrapTip: tip, BootstrapThrough: seg.MaxSeq, Known: true}); err != nil {
				return 0, err
			}
		}
	}
	through := max(page.PlannedThroughSeq, after)
	if err := c.persistPosition(ctx, position{Seq: live, BootstrapTip: tip, BootstrapThrough: through, Known: true}); err != nil {
		return 0, err
	}
	return through, nil
}

// applySegment folds one planned segment through the same apply path the live tail uses.
func (c *Consumer) applySegment(ctx context.Context, archive *replayClient, decoder *archiveDecoder, seg planSegment, after, tip int64) error {
	// The planner has no false negatives but plenty of false positives, so the exact seq and collection predicate runs here.
	fold := func(rows []archiveRow) error {
		for _, row := range rows {
			if row.Seq <= after || row.Seq > tip {
				continue
			}
			if isCommitKind(row.Kind) && !tracked(row.Collection) {
				continue
			}
			ev, ok := row.toEvent()
			if !ok {
				continue
			}
			if err := c.applyEvent(ctx, ev); err != nil {
				return err
			}
		}
		return nil
	}

	switch seg.Mode {
	case modeBlocks:
		for _, br := range seg.Blocks {
			for idx := br.First; idx <= br.Last; idx++ {
				if err := ctx.Err(); err != nil {
					return err
				}
				frame, err := archive.block(ctx, seg.Name, idx)
				if err != nil {
					return err
				}
				rows, err := decoder.block(frame)
				if err != nil {
					return err
				}
				if err := fold(rows); err != nil {
					return err
				}
			}
		}
		return nil
	case modeSegment:
		body, err := archive.segment(ctx, seg.Name)
		if err != nil {
			return err
		}
		defer body.Close()
		return decoder.segment(body, fold)
	default:
		return fmt.Errorf("discoveringest: unknown plan mode %q for segment %s", seg.Mode, seg.Name)
	}
}

func isCommitKind(kind uint8) bool {
	return kind == archiveKindCreate || kind == archiveKindUpdate || kind == archiveKindDelete
}
