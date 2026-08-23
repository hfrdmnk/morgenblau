package discoveringest

import (
	"context"
	"iter"
	"log/slog"
	"strings"

	"github.com/bluesky-social/jetstream"
)

type sourceRequest struct {
	Host        string
	Collections []string
	AfterSeq    *uint64
	LiveCursor  *uint64
	APIKey      string
}

type sourceBatch struct {
	events []jetstream.Event
	cursor uint64
}

type eventSource interface {
	Events(context.Context) iter.Seq2[sourceBatch, error]
	Close() error
}

type sourceFactory func(sourceRequest) (eventSource, error)

type upstreamClient interface {
	Events(context.Context) iter.Seq2[*jetstream.Batch, error]
	Close() error
}

type officialSource struct {
	client upstreamClient
}

func newOfficialSource(req sourceRequest) (eventSource, error) {
	return subscribeOfficialSource(req, func(host string, opts ...jetstream.Option) (upstreamClient, error) {
		return jetstream.Subscribe(host, opts...)
	})
}

func subscribeOfficialSource(req sourceRequest, subscribe func(string, ...jetstream.Option) (upstreamClient, error)) (eventSource, error) {
	opts := []jetstream.Option{
		jetstream.WithCollections(req.Collections),
		jetstream.WithLogger(slog.Default()),
	}
	if req.AfterSeq != nil {
		opts = append(opts, jetstream.WithAfterSeq(*req.AfterSeq))
	}
	if req.LiveCursor != nil {
		opts = append(opts, jetstream.WithLiveCursor(*req.LiveCursor))
	}
	if req.APIKey != "" {
		opts = append(opts, jetstream.WithAPIKey(req.APIKey))
	}
	client, err := subscribe(normalizeSourceHost(req.Host), opts...)
	if err != nil {
		return nil, err
	}
	return &officialSource{client: client}, nil
}

func normalizeSourceHost(host string) string {
	if strings.HasPrefix(host, "wss://") {
		return "https://" + strings.TrimPrefix(host, "wss://")
	}
	if strings.HasPrefix(host, "ws://") {
		return "http://" + strings.TrimPrefix(host, "ws://")
	}
	return host
}

func (s *officialSource) Events(ctx context.Context) iter.Seq2[sourceBatch, error] {
	return func(yield func(sourceBatch, error) bool) {
		for batch, err := range s.client.Events(ctx) {
			if err != nil {
				if !yield(sourceBatch{}, err) {
					return
				}
				continue
			}
			if batch == nil {
				continue
			}
			if !yield(sourceBatch{events: batch.Events(), cursor: batch.LastCursor()}, nil) {
				return
			}
		}
	}
}

func (s *officialSource) Close() error { return s.client.Close() }
