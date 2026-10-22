package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/session"
)

func (p *fakePDS) GetLatestCommit(context.Context, *session.Session) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return fmt.Sprintf("head-%d-%d", p.creates, p.puts), nil
}

func (p *fakePDS) CreateRecordIfCommit(_ context.Context, sess *session.Session, collection syntax.NSID, record map[string]any, head string) (*atprepo.RecordRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if head != fmt.Sprintf("head-%d-%d", p.creates, p.puts) {
		return nil, &atclient.APIError{StatusCode: 400, Name: "InvalidSwap"}
	}
	if err := p.createErr[collection.String()]; err != nil {
		return nil, err
	}
	p.creates++
	p.rkeySeq++
	p.lastRec = record
	rkey := fmt.Sprintf("3la%d", p.rkeySeq)
	p.created = append(p.created, pdsWrite{collection: collection.String(), rkey: rkey, record: record})
	p.storeListed(sess.Data.AccountDID.String(), collection.String(), rkey, record)
	return &atprepo.RecordRef{URI: "at://" + sess.Data.AccountDID.String() + "/" + collection.String() + "/" + rkey, CID: "bafyreiabc"}, nil
}

func (p *fakePDS) PutRecordIfCID(_ context.Context, sess *session.Session, collection syntax.NSID, rkey string, record map[string]any, cid string) (*atprepo.RecordRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.putErr != nil {
		return nil, p.putErr
	}
	uri := "at://" + sess.Data.AccountDID.String() + "/" + collection.String() + "/" + rkey
	for i, old := range p.listed[collection.String()] {
		if old.URI != uri {
			continue
		}
		if old.CID != cid {
			return nil, &atclient.APIError{StatusCode: 400, Name: "InvalidSwap"}
		}
		p.puts++
		p.lastPut = record
		p.lastPutRkey = rkey
		newCID := fmt.Sprintf("record-%d", p.puts)
		p.listed[collection.String()][i] = atprepo.ListedRecord{URI: uri, CID: newCID, Value: record}
		return &atprepo.RecordRef{URI: uri, CID: newCID}, nil
	}
	return nil, &atclient.APIError{StatusCode: 400, Name: "InvalidSwap"}
}

type racingImportPDS struct {
	fakePDS
	beforeWrite  func()
	loseResponse bool
}

func (p *racingImportPDS) interleave() {
	if p.beforeWrite != nil {
		f := p.beforeWrite
		p.beforeWrite = nil
		f()
	}
}

func (p *racingImportPDS) CreateRecord(ctx context.Context, sess *session.Session, c syntax.NSID, r map[string]any) (*atprepo.RecordRef, error) {
	p.interleave()
	return p.fakePDS.CreateRecord(ctx, sess, c, r)
}

func (p *racingImportPDS) PutRecord(ctx context.Context, sess *session.Session, c syntax.NSID, key string, r map[string]any) (*atprepo.RecordRef, error) {
	p.interleave()
	return p.fakePDS.PutRecord(ctx, sess, c, key, r)
}

func (p *racingImportPDS) CreateRecordIfCommit(ctx context.Context, sess *session.Session, c syntax.NSID, r map[string]any, head string) (*atprepo.RecordRef, error) {
	p.interleave()
	ref, err := p.fakePDS.CreateRecordIfCommit(ctx, sess, c, r, head)
	if err == nil && p.loseResponse {
		return nil, errors.New("response lost after commit")
	}
	return ref, err
}

func (p *racingImportPDS) PutRecordIfCID(ctx context.Context, sess *session.Session, c syntax.NSID, key string, r map[string]any, cid string) (*atprepo.RecordRef, error) {
	p.interleave()
	return p.fakePDS.PutRecordIfCID(ctx, sess, c, key, r, cid)
}

func TestImportConcurrentMetadataChangeIsMerged(t *testing.T) {
	old := importRecord(importSource{FeedURL: "https://example.com/feed", Title: "Old", Tags: []string{"Old"}}, "2026-01-01T00:00:00Z")
	pds := &racingImportPDS{fakePDS: fakePDS{listed: map[string][]atprepo.ListedRecord{subscriptionCollection: {{URI: "at://did:plc:alice/" + subscriptionCollection + "/old", CID: "old-cid", Value: old}}}}}
	pds.beforeWrite = func() {
		fresh := maps.Clone(old)
		fresh["title"], fresh["primary"], fresh["tags"] = "Newer title", true, []string{"Concurrent"}
		pds.listed[subscriptionCollection][0].Value = fresh
		pds.listed[subscriptionCollection][0].CID = "newer-cid"
	}
	idx := newFakeIndex()
	w := importRequest(t, SubscriptionsImportHandler(idx, pds, &fakeDispatcher{}), map[string]any{"sources": []importSource{{FeedURL: "https://example.com/feed", Tags: []string{"Imported"}}}})
	if w.Code != 200 || pds.lastPut["title"] != "Newer title" || pds.lastPut["primary"] != true || !reflect.DeepEqual(recordTags(pds.lastPut), []string{"Concurrent", "Imported"}) {
		t.Fatalf("concurrent metadata overwritten: %s record=%+v", w.Body.String(), pds.lastPut)
	}
}

func TestImportCompetingCreateDoesNotDuplicate(t *testing.T) {
	pds := &racingImportPDS{}
	pds.beforeWrite = func() {
		pds.creates++
		pds.storeListed("did:plc:alice", subscriptionCollection, "other-session", importRecord(importSource{FeedURL: "https://example.com/feed", Title: "Other session", Tags: []string{"Concurrent"}}, "2026-01-01T00:00:00Z"))
	}
	w := importRequest(t, SubscriptionsImportHandler(newFakeIndex(), pds, &fakeDispatcher{}), map[string]any{"sources": []importSource{{FeedURL: "https://example.com/feed", Tags: []string{"Imported"}}}})
	if w.Code != 200 || pds.creates != 1 || pds.lastPut["title"] != "Other session" || !reflect.DeepEqual(recordTags(pds.lastPut), []string{"Concurrent", "Imported"}) {
		t.Fatalf("concurrent create duplicated or metadata lost: %s creates=%d put=%+v", w.Body.String(), pds.creates, pds.lastPut)
	}
}

func TestImportCommitThenLostResponseRecoversWithoutDuplicate(t *testing.T) {
	pds := &racingImportPDS{loseResponse: true}
	idx, disp := newFakeIndex(), &fakeDispatcher{}
	h := SubscriptionsImportHandler(idx, pds, disp)
	body := map[string]any{"sources": []importSource{{FeedURL: "https://example.com/feed"}}}
	w := importRequest(t, h, body)
	var first importResult
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Failures) != 1 || len(idx.upsertedFeeds) != 0 {
		t.Fatalf("uncertain write mirrored: %+v", first)
	}
	w = importRequest(t, h, body)
	var second importResult
	if err := json.Unmarshal(w.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Failures) != 0 || second.Unchanged != 1 || pds.creates != 1 || len(disp.dispatched) != 1 {
		t.Fatalf("retry recovery: %+v creates=%d dispatch=%v", second, pds.creates, disp.dispatched)
	}
}

type simultaneousImportPDS struct {
	fakePDS
	bothListed chan struct{}
}

func (p *simultaneousImportPDS) ListRecords(ctx context.Context, _ *session.Session, collection syntax.NSID) ([]atprepo.ListedRecord, error) {
	p.mu.Lock()
	p.listCalls++
	call := p.listCalls
	var records []atprepo.ListedRecord
	for _, record := range p.listed[collection.String()] {
		record.Value = maps.Clone(record.Value)
		records = append(records, record)
	}
	if call == 2 {
		close(p.bothListed)
	}
	p.mu.Unlock()
	if call <= 2 {
		select {
		case <-p.bothListed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return records, nil
}

func TestImportTwoSessionsCreateOneRecord(t *testing.T) {
	pds := &simultaneousImportPDS{bothListed: make(chan struct{})}
	h := SubscriptionsImportHandler(newFakeIndex(), pds, &fakeDispatcher{})
	results := make(chan *httptest.ResponseRecorder, 2)
	for _, tag := range []string{"First", "Second"} {
		go func() {
			data, _ := json.Marshal(map[string]any{"sources": []importSource{{FeedURL: "https://example.com/feed", Tags: []string{tag}}}})
			r := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions/import", bytes.NewReader(data)), "did:plc:alice", tag)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			results <- w
		}()
	}
	for range 2 {
		w := <-results
		var result importResult
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || len(result.Failures) != 0 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	if pds.creates != 1 || pds.puts != 1 || !sameTagSet(recordTags(pds.listed[subscriptionCollection][0].Value), []string{"First", "Second"}) {
		t.Fatalf("duplicate or lost tags: creates=%d puts=%d records=%+v", pds.creates, pds.puts, pds.listed)
	}
}

type delayedImportPDS struct {
	fakePDS
	delayed     func() error
	beforeRetry bool
}

func (p *delayedImportPDS) CreateRecordIfCommit(ctx context.Context, sess *session.Session, c syntax.NSID, r map[string]any, head string) (*atprepo.RecordRef, error) {
	if p.delayed == nil {
		p.delayed = func() error {
			_, err := p.fakePDS.CreateRecordIfCommit(context.Background(), sess, c, r, head)
			return err
		}
		return nil, errors.New("timed out before commit")
	}
	if p.beforeRetry {
		p.beforeRetry = false
		if err := p.delayed(); err != nil {
			return nil, err
		}
	}
	return p.fakePDS.CreateRecordIfCommit(ctx, sess, c, r, head)
}

func TestImportDelayedCreateCannotDuplicateRetry(t *testing.T) {
	for _, oldWins := range []bool{true, false} {
		t.Run(fmt.Sprintf("oldWins=%v", oldWins), func(t *testing.T) {
			pds := &delayedImportPDS{beforeRetry: oldWins}
			disp := &fakeDispatcher{}
			h := SubscriptionsImportHandler(newFakeIndex(), pds, disp)
			body := map[string]any{"sources": []importSource{{FeedURL: "https://example.com/feed"}}}
			first := importRequest(t, h, body)
			var result importResult
			if err := json.Unmarshal(first.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Failures) != 1 || pds.creates != 0 {
				t.Fatalf("first request: %+v", result)
			}
			retry := importRequest(t, h, body)
			if err := json.Unmarshal(retry.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Failures) != 0 {
				t.Fatal(retry.Body.String())
			}
			if !oldWins && !isImportConflict(pds.delayed()) {
				t.Fatal("late commit was not rejected")
			}
			if pds.creates != 1 || len(disp.dispatched) != 1 {
				t.Fatalf("creates=%d dispatch=%v", pds.creates, disp.dispatched)
			}
		})
	}
}

func TestImportConflictRetriesAreBounded(t *testing.T) {
	record := importRecord(importSource{FeedURL: "https://example.com/feed"}, "2026-01-01T00:00:00Z")
	pds := &fakePDS{putErr: &atclient.APIError{StatusCode: 400, Name: "InvalidSwap"}, listed: map[string][]atprepo.ListedRecord{subscriptionCollection: {{URI: "at://did:plc:alice/" + subscriptionCollection + "/old", CID: "old-cid", Value: record}}}}
	idx := newFakeIndex()
	w := importRequest(t, SubscriptionsImportHandler(idx, pds, &fakeDispatcher{}), map[string]any{"sources": []importSource{{FeedURL: "https://example.com/feed", Tags: []string{"New"}}}})
	var result importResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Failures) != 1 || pds.listCalls != 3 || len(idx.upsertedFeeds) != 0 {
		t.Fatalf("unbounded/unsafe retry: %+v calls=%d", result, pds.listCalls)
	}
}

type supersededImportIndex struct {
	*fakeIndex
	newer db.UpsertUserSubscriptionParams
}

func (idx supersededImportIndex) MirrorImportedSubscription(ctx context.Context, baseline *db.UserSubscription, feed db.UpsertFeedParams, row db.UpsertUserSubscriptionParams) error {
	if err := idx.UpsertUserSubscription(ctx, idx.newer); err != nil {
		return err
	}
	return idx.fakeIndex.MirrorImportedSubscription(ctx, baseline, feed, row)
}

func TestImportDoesNotOverwriteNewerLocalMirror(t *testing.T) {
	index := newFakeIndex()
	first, second := `["First"]`, `["First","Second"]`
	row := db.UpsertUserSubscriptionParams{Did: "did:plc:alice", Rkey: "old", AtUri: "at://did:plc:alice/" + subscriptionCollection + "/old", FeedUrl: "https://example.com/feed", Tags: &first, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
	if err := index.UpsertUserSubscription(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	row.Tags = &second
	pds := &fakePDS{listed: map[string][]atprepo.ListedRecord{subscriptionCollection: {{URI: row.AtUri, CID: "cid", Value: importRecord(importSource{FeedURL: row.FeedUrl, Tags: []string{"First"}}, row.CreatedAt)}}}}
	disp := &fakeDispatcher{}
	w := importRequest(t, SubscriptionsImportHandler(supersededImportIndex{index, row}, pds, disp), map[string]any{"sources": []importSource{{FeedURL: row.FeedUrl}}})
	got, err := index.GetUserSubscriptionByFeedURL(context.Background(), db.GetUserSubscriptionByFeedURLParams{Did: row.Did, FeedUrl: row.FeedUrl})
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || got.Tags == nil || *got.Tags != second || disp.manualSync != 1 {
		t.Fatalf("stale mirror erased newer tags: %s tags=%v repairs=%d", w.Body.String(), got.Tags, disp.manualSync)
	}
}

func (idx *fakeIndex) SnapshotImportedSubscription(ctx context.Context, did, feedURL string) (*db.UserSubscription, error) {
	row, err := idx.GetUserSubscriptionByFeedURL(ctx, db.GetUserSubscriptionByFeedURLParams{Did: did, FeedUrl: feedURL})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (idx *fakeIndex) MirrorImportedSubscription(_ context.Context, baseline *db.UserSubscription, feed db.UpsertFeedParams, row db.UpsertUserSubscriptionParams) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	var current *db.UserSubscription
	if found, ok := idx.rows[row.Did][row.FeedUrl]; ok {
		current = &found
	}
	if !reflect.DeepEqual(current, baseline) {
		return errors.New("subscription changed during PDS import")
	}
	idx.upsertedFeeds = append(idx.upsertedFeeds, feed.FeedUrl)
	idx.feedParams = append(idx.feedParams, feed)
	if idx.rows[row.Did] == nil {
		idx.rows[row.Did] = map[string]db.UserSubscription{}
	}
	idx.rows[row.Did][row.FeedUrl] = db.UserSubscription{
		Did: row.Did, Rkey: row.Rkey, AtUri: row.AtUri, FeedUrl: row.FeedUrl, Kind: "rss",
		SidecarRkey: row.SidecarRkey, Title: row.Title, IsPrimary: row.IsPrimary, Tags: row.Tags, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	return nil
}

func (idx upsertErrIndex) MirrorImportedSubscription(context.Context, *db.UserSubscription, db.UpsertFeedParams, db.UpsertUserSubscriptionParams) error {
	return errors.New("local mirror failed")
}
