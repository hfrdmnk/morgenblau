package discoveringest

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDecodeFrame_Commit(t *testing.T) {
	raw := `{"$type":"message","payload":{
		"$type":"network.bsky.jetstream.subscribeEvents#commit",
		"seq":4242,
		"did":"did:plc:aaaaaaaaaaaaaaaaaaaaaaaa",
		"time":"2026-08-01T10:00:00.000000Z",
		"rev":"3aaaaaaaaaaa2",
		"operation":"create",
		"collection":"blue.morgen.feed.save",
		"rkey":"3bbbbbbbbbbb2",
		"cid":"bafyreiaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"record":{"$type":"blue.morgen.feed.save","itemUrl":"https://news.example.com/a"}
	}}`

	ev, err := decodeFrame([]byte(raw))
	if err != nil {
		t.Fatalf("decodeFrame: %v", err)
	}
	if ev.Kind != kindCommit {
		t.Fatalf("Kind = %q, want %q", ev.Kind, kindCommit)
	}
	if ev.Seq != 4242 {
		t.Errorf("Seq = %d, want 4242", ev.Seq)
	}
	if ev.Commit == nil {
		t.Fatal("Commit is nil")
	}
	if ev.Commit.Action != actionCreate {
		t.Errorf("Action = %q, want %q", ev.Commit.Action, actionCreate)
	}
	if ev.Commit.Collection != "blue.morgen.feed.save" {
		t.Errorf("Collection = %q", ev.Commit.Collection)
	}
	if ev.Commit.Rkey != "3bbbbbbbbbbb2" {
		t.Errorf("Rkey = %q", ev.Commit.Rkey)
	}
	if ev.Commit.CID == "" {
		t.Error("CID is empty")
	}
	var value map[string]any
	if err := json.Unmarshal(ev.Commit.Record, &value); err != nil {
		t.Fatalf("record is not JSON: %v", err)
	}
	if value["itemUrl"] != "https://news.example.com/a" {
		t.Errorf("record itemUrl = %v", value["itemUrl"])
	}
}

func TestDecodeFrame_DeleteCarriesNoRecord(t *testing.T) {
	raw := `{"$type":"message","payload":{
		"$type":"network.bsky.jetstream.subscribeEvents#commit",
		"seq":7,"did":"did:plc:aaaaaaaaaaaaaaaaaaaaaaaa","time":"2026-08-01T10:00:00.000000Z",
		"rev":"3aaaaaaaaaaa2","operation":"delete",
		"collection":"blue.morgen.feed.save","rkey":"3bbbbbbbbbbb2"}}`

	ev, err := decodeFrame([]byte(raw))
	if err != nil {
		t.Fatalf("decodeFrame: %v", err)
	}
	if ev.Commit.Action != actionDelete {
		t.Fatalf("Action = %q", ev.Commit.Action)
	}
	if len(ev.Commit.Record) != 0 {
		t.Errorf("Record = %q, want empty", ev.Commit.Record)
	}
}

// An identity frame must never produce an account change: the two kinds arrive
// separately and a merged shape would let identity's zero values deactivate a repo.
func TestDecodeFrame_IdentityCarriesHandleOnly(t *testing.T) {
	raw := `{"$type":"message","payload":{
		"$type":"network.bsky.jetstream.subscribeEvents#identity",
		"seq":9,"did":"did:plc:aaaaaaaaaaaaaaaaaaaaaaaa","time":"2026-08-01T10:00:00.000000Z",
		"identity":{"did":"did:plc:aaaaaaaaaaaaaaaaaaaaaaaa","handle":"reader.example","seq":11,"time":"2026-08-01T09:59:00Z"}}}`

	ev, err := decodeFrame([]byte(raw))
	if err != nil {
		t.Fatalf("decodeFrame: %v", err)
	}
	if ev.Kind != kindIdentity {
		t.Fatalf("Kind = %q", ev.Kind)
	}
	if ev.Identity == nil || ev.Identity.Handle != "reader.example" {
		t.Fatalf("Identity = %+v", ev.Identity)
	}
	if ev.Account != nil {
		t.Error("identity frame produced an account change")
	}
}

func TestDecodeFrame_AccountCarriesStatusOnly(t *testing.T) {
	raw := `{"$type":"message","payload":{
		"$type":"network.bsky.jetstream.subscribeEvents#account",
		"seq":12,"did":"did:plc:aaaaaaaaaaaaaaaaaaaaaaaa","time":"2026-08-01T10:00:00.000000Z",
		"account":{"did":"did:plc:aaaaaaaaaaaaaaaaaaaaaaaa","active":false,"status":"deleted","seq":13,"time":"2026-08-01T09:59:00Z"}}}`

	ev, err := decodeFrame([]byte(raw))
	if err != nil {
		t.Fatalf("decodeFrame: %v", err)
	}
	if ev.Kind != kindAccount {
		t.Fatalf("Kind = %q", ev.Kind)
	}
	if ev.Account == nil || ev.Account.Active || ev.Account.Status != "deleted" {
		t.Fatalf("Account = %+v", ev.Account)
	}
	if ev.Identity != nil {
		t.Error("account frame produced an identity change")
	}
}

func TestDecodeFrame_Sync(t *testing.T) {
	raw := `{"$type":"message","payload":{
		"$type":"network.bsky.jetstream.subscribeEvents#sync",
		"seq":14,"did":"did:plc:aaaaaaaaaaaaaaaaaaaaaaaa","time":"2026-08-01T10:00:00.000000Z",
		"sync":{"did":"did:plc:aaaaaaaaaaaaaaaaaaaaaaaa","rev":"3aaaaaaaaaaa2","seq":15,"time":"2026-08-01T09:59:00Z"}}}`

	ev, err := decodeFrame([]byte(raw))
	if err != nil {
		t.Fatalf("decodeFrame: %v", err)
	}
	if ev.Kind != kindSync || ev.Sync == nil {
		t.Fatalf("Kind = %q, Sync = %+v", ev.Kind, ev.Sync)
	}
	if ev.Seq != 14 {
		t.Errorf("Seq = %d, want 14", ev.Seq)
	}
}

// #info is seq-less, so it must never move the persisted cursor backwards to zero.
func TestDecodeFrame_InfoIsSeqless(t *testing.T) {
	raw := `{"$type":"message","payload":{
		"$type":"network.bsky.jetstream.subscribeEvents#info",
		"name":"OutdatedCursor","message":"resumed from 900"}}`

	ev, err := decodeFrame([]byte(raw))
	if err != nil {
		t.Fatalf("decodeFrame: %v", err)
	}
	if ev.Kind != kindInfo || ev.Info != "OutdatedCursor" {
		t.Fatalf("Kind = %q, Info = %q", ev.Kind, ev.Info)
	}
	if ev.Seq != 0 {
		t.Errorf("Seq = %d, want 0", ev.Seq)
	}
}

func TestDecodeFrame_ErrorFrameIsTerminal(t *testing.T) {
	raw := `{"$type":"error","error":"ConsumerTooSlow","message":"too far behind"}`

	_, err := decodeFrame([]byte(raw))
	var se *streamError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want *streamError", err)
	}
	if se.Name != "ConsumerTooSlow" {
		t.Errorf("Name = %q", se.Name)
	}
}

func TestDecodeFrame_UnknownPayloadTypeIsIgnorable(t *testing.T) {
	raw := `{"$type":"message","payload":{"$type":"network.bsky.jetstream.subscribeEvents#somethingNew","seq":3}}`

	ev, err := decodeFrame([]byte(raw))
	if err != nil {
		t.Fatalf("decodeFrame: %v", err)
	}
	if ev.Kind != "" {
		t.Errorf("Kind = %q, want empty for an unknown variant", ev.Kind)
	}
	if ev.Seq != 3 {
		t.Errorf("Seq = %d, want 3 so an unknown variant still advances the cursor", ev.Seq)
	}
}
