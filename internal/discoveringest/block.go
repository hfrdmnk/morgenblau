package discoveringest

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/bluesky-social/indigo/atproto/atdata"
	"github.com/klauspost/compress/zstd"
)

// Archive kind discriminators, from the segment block's kind column.
const (
	archiveKindCreate   uint8 = 1
	archiveKindUpdate   uint8 = 2
	archiveKindDelete   uint8 = 3
	archiveKindIdentity uint8 = 4
	archiveKindAccount  uint8 = 5
	archiveKindSync     uint8 = 6
)

const (
	segmentHeaderBytes = 256
	// offsetFooterOffset locates the end of the block region inside the fixed header; blocks run from segmentHeaderBytes up to it.
	offsetFooterOffset = 58
	// maxBlockBytes bounds one compressed block before we allocate for it, so a corrupt or hostile length prefix cannot exhaust memory.
	maxBlockBytes = 64 << 20
	// maxDecodedBytes bounds one decompressed block; the operator default is 4096 events, far below this.
	maxDecodedBytes = 512 << 20
)

var segmentMagic = []byte("jss0")

var errShortBlock = errors.New("discoveringest: archive block is truncated")

// archiveRow is one event as the archive stores it. The archive has no CID column, so records mirrored from a bootstrap carry no CID.
type archiveRow struct {
	Seq        int64
	Kind       uint8
	Collection string
	DID        string
	Rkey       string
	Rev        string
	Payload    []byte
}

// archiveDecoder turns Replay archive bytes into rows. One decoder is reused across a whole bootstrap, so it is not safe for concurrent use.
type archiveDecoder struct {
	zstd *zstd.Decoder
}

func newArchiveDecoder() (*archiveDecoder, error) {
	z, err := zstd.NewReader(nil, zstd.WithDecoderMaxMemory(maxDecodedBytes), zstd.WithDecoderConcurrency(1))
	if err != nil {
		return nil, err
	}
	return &archiveDecoder{zstd: z}, nil
}

func (d *archiveDecoder) Close() { d.zstd.Close() }

// block decompresses one zstd frame as served by getBlock and splits its columnar payload into rows.
func (d *archiveDecoder) block(frame []byte) ([]archiveRow, error) {
	plain, err := d.zstd.DecodeAll(frame, nil)
	if err != nil {
		return nil, fmt.Errorf("discoveringest: decompress archive block: %w", err)
	}
	return decodeColumns(plain)
}

// segment walks a .jss file's block region, handing each block's rows to fn. Blocks are streamed rather than buffered: a sealed segment reaches 256 MB.
func (d *archiveDecoder) segment(r io.Reader, fn func([]archiveRow) error) error {
	header := make([]byte, segmentHeaderBytes)
	if _, err := io.ReadFull(r, header); err != nil {
		return fmt.Errorf("discoveringest: read segment header: %w", err)
	}
	if string(header[:4]) != string(segmentMagic) {
		return fmt.Errorf("discoveringest: segment magic %q is not %q", header[:4], segmentMagic)
	}

	// A zero checksum means the header was never finalized, so footer_offset is meaningless and the blocks run to EOF.
	remaining := int64(-1)
	if binary.LittleEndian.Uint64(header[4:]) != 0 {
		footer := binary.LittleEndian.Uint64(header[offsetFooterOffset:])
		if footer < segmentHeaderBytes {
			return fmt.Errorf("discoveringest: segment footer offset %d precedes the header", footer)
		}
		remaining = int64(footer) - segmentHeaderBytes
	}

	var length [8]byte
	for remaining != 0 {
		if _, err := io.ReadFull(r, length[:]); err != nil {
			if errors.Is(err, io.EOF) && remaining < 0 {
				return nil
			}
			return fmt.Errorf("discoveringest: read block length: %w", err)
		}
		size := binary.LittleEndian.Uint64(length[:])
		if size == 0 || size > maxBlockBytes {
			return fmt.Errorf("discoveringest: implausible block length %d", size)
		}
		frame := make([]byte, size)
		if _, err := io.ReadFull(r, frame); err != nil {
			return fmt.Errorf("discoveringest: read block body: %w", err)
		}
		rows, err := d.block(frame)
		if err != nil {
			return err
		}
		if err := fn(rows); err != nil {
			return err
		}
		if remaining > 0 {
			remaining -= int64(len(length)) + int64(size)
			if remaining < 0 {
				return errors.New("discoveringest: block region overran the segment footer")
			}
		}
	}
	return nil
}

// decodeColumns splits one decompressed block into rows. Column order is fixed by the segment format; every length is validated against the remaining bytes before it is used to slice.
func decodeColumns(b []byte) ([]archiveRow, error) {
	if len(b) < 4 {
		return nil, errShortBlock
	}
	count := int(binary.LittleEndian.Uint32(b))
	pos := 4

	fixed := func(width int) ([]byte, error) {
		end := pos + width*count
		if end > len(b) || end < pos {
			return nil, errShortBlock
		}
		out := b[pos:end]
		pos = end
		return out, nil
	}

	seq, err := fixed(8)
	if err != nil {
		return nil, err
	}
	if _, err := fixed(8); err != nil { // witnessed_at: crawl time, never a freshness input
		return nil, err
	}
	if _, err := fixed(8); err != nil { // indexed_at: display time, same
		return nil, err
	}
	kind, err := fixed(1)
	if err != nil {
		return nil, err
	}
	collLen, err := fixed(1)
	if err != nil {
		return nil, err
	}
	didLen, err := fixed(2)
	if err != nil {
		return nil, err
	}
	rkeyLen, err := fixed(1)
	if err != nil {
		return nil, err
	}
	revLen, err := fixed(1)
	if err != nil {
		return nil, err
	}
	eventLen, err := fixed(4)
	if err != nil {
		return nil, err
	}

	take := func(n int) ([]byte, error) {
		end := pos + n
		if end > len(b) || end < pos {
			return nil, errShortBlock
		}
		out := b[pos:end]
		pos = end
		return out, nil
	}

	rows := make([]archiveRow, count)
	for i := range rows {
		rows[i].Seq = int64(binary.LittleEndian.Uint64(seq[i*8:]))
		rows[i].Kind = kind[i]
	}
	for i := range rows {
		s, err := take(int(collLen[i]))
		if err != nil {
			return nil, err
		}
		rows[i].Collection = string(s)
	}
	for i := range rows {
		s, err := take(int(binary.LittleEndian.Uint16(didLen[i*2:])))
		if err != nil {
			return nil, err
		}
		rows[i].DID = string(s)
	}
	for i := range rows {
		s, err := take(int(rkeyLen[i]))
		if err != nil {
			return nil, err
		}
		rows[i].Rkey = string(s)
	}
	for i := range rows {
		s, err := take(int(revLen[i]))
		if err != nil {
			return nil, err
		}
		rows[i].Rev = string(s)
	}
	for i := range rows {
		p, err := take(int(binary.LittleEndian.Uint32(eventLen[i*4:])))
		if err != nil {
			return nil, err
		}
		rows[i].Payload = p
	}
	return rows, nil
}

// toEvent maps an archive row onto the live stream's event shape so both feed one apply path. A marker whose payload will not decode is dropped rather than guessed at: a wrong guess purges a live repo.
func (r archiveRow) toEvent() (event, bool) {
	switch r.Kind {
	case archiveKindCreate, archiveKindUpdate, archiveKindDelete:
		op := commitOp{DID: r.DID, Collection: r.Collection, Rkey: r.Rkey, Action: archiveAction(r.Kind)}
		if r.Kind != archiveKindDelete {
			record, err := cborToJSON(r.Payload)
			if err != nil {
				return event{}, false
			}
			op.Record = record
		}
		return event{Seq: r.Seq, Kind: kindCommit, Commit: &op}, true
	case archiveKindIdentity:
		value, err := atdata.UnmarshalCBOR(r.Payload)
		if err != nil {
			return event{}, false
		}
		handle, _ := value["handle"].(string)
		return event{Seq: r.Seq, Kind: kindIdentity, Identity: &identityChange{DID: r.DID, Handle: handle}}, true
	case archiveKindAccount:
		value, err := atdata.UnmarshalCBOR(r.Payload)
		if err != nil {
			return event{}, false
		}
		active, ok := value["active"].(bool)
		if !ok {
			return event{}, false
		}
		status, _ := value["status"].(string)
		return event{Seq: r.Seq, Kind: kindAccount, Account: &accountChange{DID: r.DID, Active: active, Status: status}}, true
	case archiveKindSync:
		return event{Seq: r.Seq, Kind: kindSync, Sync: &syncMarker{DID: r.DID}}, true
	default:
		return event{Seq: r.Seq}, false
	}
}

func archiveAction(kind uint8) string {
	switch kind {
	case archiveKindUpdate:
		return actionUpdate
	case archiveKindDelete:
		return actionDelete
	default:
		return actionCreate
	}
}

// cborToJSON re-encodes a record's DAG-CBOR as atproto JSON, the form the mirror and every decoder downstream expect.
func cborToJSON(payload []byte) (json.RawMessage, error) {
	value, err := atdata.UnmarshalCBOR(payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}
