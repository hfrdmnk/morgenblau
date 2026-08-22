package discoveringest

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/bluesky-social/indigo/atproto/atdata"
	"github.com/klauspost/compress/zstd"
)

// mustCBOR builds a DAG-CBOR record payload the way the archive stores one.
func mustCBOR(t *testing.T, value map[string]any) []byte {
	t.Helper()
	b, err := atdata.MarshalCBOR(value)
	if err != nil {
		t.Fatalf("MarshalCBOR: %v", err)
	}
	return b
}

// buildBlock encodes rows into the columnar block layout and compresses it as one zstd frame, matching what getBlock serves.
func buildBlock(t *testing.T, rows []archiveRow) []byte {
	t.Helper()
	var fixed bytes.Buffer
	var colls, dids, rkeys, revs, payloads bytes.Buffer

	n := uint32(len(rows))
	_ = binary.Write(&fixed, binary.LittleEndian, n)
	for _, r := range rows {
		_ = binary.Write(&fixed, binary.LittleEndian, uint64(r.Seq))
	}
	for range rows {
		_ = binary.Write(&fixed, binary.LittleEndian, int64(1785849176574140))
	}
	for range rows {
		_ = binary.Write(&fixed, binary.LittleEndian, int64(0))
	}
	for _, r := range rows {
		fixed.WriteByte(r.Kind)
	}
	for _, r := range rows {
		fixed.WriteByte(uint8(len(r.Collection)))
	}
	for _, r := range rows {
		_ = binary.Write(&fixed, binary.LittleEndian, uint16(len(r.DID)))
	}
	for _, r := range rows {
		fixed.WriteByte(uint8(len(r.Rkey)))
	}
	for _, r := range rows {
		fixed.WriteByte(uint8(len(r.Rev)))
	}
	for _, r := range rows {
		_ = binary.Write(&fixed, binary.LittleEndian, uint32(len(r.Payload)))
	}
	for _, r := range rows {
		colls.WriteString(r.Collection)
		dids.WriteString(r.DID)
		rkeys.WriteString(r.Rkey)
		revs.WriteString(r.Rev)
		payloads.Write(r.Payload)
	}

	plain := bytes.Join([][]byte{fixed.Bytes(), colls.Bytes(), dids.Bytes(), rkeys.Bytes(), revs.Bytes(), payloads.Bytes()}, nil)
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatalf("zstd.NewWriter: %v", err)
	}
	defer enc.Close()
	return enc.EncodeAll(plain, nil)
}

// buildSegment wraps compressed blocks in a sealed .jss file: 256-byte header, length-prefixed blocks, then a footer we never read.
func buildSegment(t *testing.T, blocks [][]byte) []byte {
	t.Helper()
	var body bytes.Buffer
	for _, b := range blocks {
		_ = binary.Write(&body, binary.LittleEndian, uint64(len(b)))
		body.Write(b)
	}
	header := make([]byte, segmentHeaderBytes)
	copy(header, segmentMagic)
	binary.LittleEndian.PutUint64(header[4:], 0xfeedfacefeedface)
	binary.LittleEndian.PutUint32(header[14:], uint32(len(blocks)))
	binary.LittleEndian.PutUint64(header[offsetFooterOffset:], uint64(segmentHeaderBytes+body.Len()))

	out := append([]byte{}, header...)
	out = append(out, body.Bytes()...)
	return append(out, []byte("footer bytes the reader must not walk into")...)
}

func sampleRows(t *testing.T) []archiveRow {
	t.Helper()
	return []archiveRow{
		{
			Seq: 10, Kind: archiveKindCreate, Collection: "blue.morgen.feed.subscription",
			DID: testDID, Rkey: "3aaaaaaaaaaa2", Rev: "3rrrrrrrrrrr2",
			Payload: mustCBOR(t, map[string]any{
				"$type":     "blue.morgen.feed.subscription",
				"title":     "Example Publication",
				"createdAt": "2026-03-01T00:00:00Z",
			}),
		},
		{
			Seq: 11, Kind: archiveKindDelete, Collection: "blue.morgen.feed.save",
			DID: testDID, Rkey: "3bbbbbbbbbbb2", Rev: "3rrrrrrrrrrr3",
		},
		{
			Seq: 12, Kind: archiveKindCreate, Collection: "app.bsky.feed.post",
			DID: otherDID, Rkey: "3ccccccccccc2", Rev: "3rrrrrrrrrrr4",
			Payload: mustCBOR(t, map[string]any{"$type": "app.bsky.feed.post", "text": "off-network"}),
		},
	}
}

func TestArchiveDecoder_BlockRoundTrip(t *testing.T) {
	dec := newTestDecoder(t)
	rows, err := dec.block(buildBlock(t, sampleRows(t)))
	if err != nil {
		t.Fatalf("block: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0].Seq != 10 || rows[0].Collection != "blue.morgen.feed.subscription" || rows[0].DID != testDID {
		t.Errorf("row 0 = %+v", rows[0])
	}
	if rows[0].Rkey != "3aaaaaaaaaaa2" || rows[0].Rev != "3rrrrrrrrrrr2" {
		t.Errorf("row 0 keys = %q %q", rows[0].Rkey, rows[0].Rev)
	}
	if rows[1].Kind != archiveKindDelete || len(rows[1].Payload) != 0 {
		t.Errorf("row 1 = %+v", rows[1])
	}
	if rows[2].DID != otherDID {
		t.Errorf("row 2 did = %q", rows[2].DID)
	}
}

func TestArchiveDecoder_SegmentStopsAtTheFooter(t *testing.T) {
	dec := newTestDecoder(t)
	seg := buildSegment(t, [][]byte{
		buildBlock(t, sampleRows(t)[:2]),
		buildBlock(t, sampleRows(t)[2:]),
	})

	var seen []int64
	err := dec.segment(bytes.NewReader(seg), func(rows []archiveRow) error {
		for _, r := range rows {
			seen = append(seen, r.Seq)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("segment: %v", err)
	}
	want := []int64{10, 11, 12}
	if len(seen) != len(want) {
		t.Fatalf("seq = %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("seq = %v, want %v", seen, want)
		}
	}
}

func TestArchiveRow_CommitBecomesJSONRecord(t *testing.T) {
	dec := newTestDecoder(t)
	rows, err := dec.block(buildBlock(t, sampleRows(t)))
	if err != nil {
		t.Fatalf("block: %v", err)
	}

	ev, ok := rows[0].toEvent()
	if !ok {
		t.Fatal("create row did not convert")
	}
	if ev.Kind != kindCommit || ev.Commit.Action != actionCreate {
		t.Fatalf("event = %+v", ev)
	}
	var value map[string]any
	if err := json.Unmarshal(ev.Commit.Record, &value); err != nil {
		t.Fatalf("record is not JSON: %v", err)
	}
	if value["title"] != "Example Publication" {
		t.Errorf("title = %v", value["title"])
	}
	// The archive has no CID column, so mirrored rows from a bootstrap carry none.
	if ev.Commit.CID != "" {
		t.Errorf("CID = %q, want empty", ev.Commit.CID)
	}
}

func TestArchiveRow_AccountDeletionConverts(t *testing.T) {
	row := archiveRow{
		Seq: 20, Kind: archiveKindAccount, DID: testDID,
		Payload: mustCBOR(t, map[string]any{"did": testDID, "active": false, "status": "deleted"}),
	}
	ev, ok := row.toEvent()
	if !ok {
		t.Fatal("account row did not convert")
	}
	if ev.Account == nil || ev.Account.Active || ev.Account.Status != "deleted" {
		t.Fatalf("account = %+v", ev.Account)
	}
	if ev.Identity != nil {
		t.Error("account row produced an identity change")
	}
}

// A marker whose payload will not decode must be dropped, never guessed at: a wrong guess purges a live repo.
func TestArchiveRow_UndecodableAccountIsDropped(t *testing.T) {
	row := archiveRow{Seq: 21, Kind: archiveKindAccount, DID: testDID, Payload: []byte{0xff, 0xff}}
	if _, ok := row.toEvent(); ok {
		t.Fatal("undecodable account row converted")
	}
}

func newTestDecoder(t *testing.T) *archiveDecoder {
	t.Helper()
	d, err := newArchiveDecoder()
	if err != nil {
		t.Fatalf("newArchiveDecoder: %v", err)
	}
	t.Cleanup(d.Close)
	return d
}
