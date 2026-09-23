package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
)

// sweepLister is the PDS surface sweepDuplicates needs: list to find matches, delete to remove them.
type sweepLister interface {
	atprepo.Lister
	DeleteRecord(ctx context.Context, sess *oauth.ClientSession, collection syntax.NSID, rkey string) error
}

// sweepDuplicates deletes every record in collection where field(rec) == want, writing the 502 itself on the first failure; a surviving duplicate would resurrect the deleted state on the next reconcile.
func sweepDuplicates(
	ctx context.Context, w http.ResponseWriter, sess *oauth.ClientSession, pds sweepLister,
	op string, collection syntax.NSID, field func(atprepo.ListedRecord) string, want string,
) bool {
	records, err := pds.ListRecords(ctx, sess, collection)
	if err != nil {
		slog.Warn(op+": list failed", "err", err)
		writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
		return false
	}
	for _, rec := range records {
		if field(rec) != want {
			continue
		}
		if err := pds.DeleteRecord(ctx, sess, collection, atprepo.RkeyFromATURI(rec.URI)); err != nil {
			slog.Warn(op+": delete failed", "uri", rec.URI, "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
			return false
		}
	}
	return true
}

// stringField extracts one string field from a listed record's raw value, for use as sweepDuplicates' match key.
func stringField(key string) func(atprepo.ListedRecord) string {
	return func(rec atprepo.ListedRecord) string {
		s, _ := rec.Value[key].(string)
		return s
	}
}

// sidecarWriteSpec describes one PDS mutation: optional existence record, optional lazy sidecar; a non-empty SidecarRkey puts instead of creating, so one spec covers every caller without mode flags.
type sidecarWriteSpec struct {
	Existence           map[string]any
	ExistenceCollection syntax.NSID
	ExistenceRkey       syntax.RecordKey
	ExistenceOp         string // slog op logged if the existence write fails

	Sidecar           map[string]any
	SidecarCollection syntax.NSID
	SidecarRkey       string // existing rkey: puts here instead of creating
	SidecarCreateRkey syntax.RecordKey
	SidecarOp         string // slog op logged if the sidecar write fails
}

// sidecarWriteResult is what writeSidecarPair actually wrote, for the caller to fold into its Tier-1 mirror.
type sidecarWriteResult struct {
	ExistenceRef *atprepo.RecordRef
	SidecarRkey  string // set whether created or put; empty if spec.Sidecar was nil
}

// writeSidecarPair creates records with known rkeys in one PDS commit.
func writeSidecarPair(ctx context.Context, w http.ResponseWriter, sess *oauth.ClientSession, pds atprepo.Writer, spec sidecarWriteSpec) (sidecarWriteResult, bool) {
	var out sidecarWriteResult
	if spec.Existence != nil && spec.Sidecar != nil && spec.SidecarRkey == "" && (spec.ExistenceRkey == "" || spec.SidecarCreateRkey == "") {
		slog.Warn(spec.SidecarOp, "err", "atomic record creation requires client-chosen rkeys")
		writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
		return out, false
	}
	var writes []atprepo.RecordWrite
	if spec.Existence != nil && spec.ExistenceRkey != "" {
		writes = append(writes, atprepo.RecordWrite{Collection: spec.ExistenceCollection, Rkey: spec.ExistenceRkey, Record: spec.Existence})
	}
	if spec.Sidecar != nil && spec.SidecarRkey == "" && spec.SidecarCreateRkey != "" {
		writes = append(writes, atprepo.RecordWrite{Collection: spec.SidecarCollection, Rkey: spec.SidecarCreateRkey, Record: spec.Sidecar})
	}
	if len(writes) > 0 {
		atomic, ok := pds.(atprepo.AtomicWriter)
		if !ok {
			slog.Warn(spec.SidecarOp, "err", "PDS writer does not support applyWrites")
			writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
			return out, false
		}
		refs, err := atomic.ApplyWrites(ctx, sess, writes)
		if err != nil {
			if confirmed, ok := confirmAtomicWrites(ctx, sess, pds, writes); ok {
				refs = confirmed
			} else {
				slog.Warn(spec.SidecarOp, "err", err)
				writeError(w, http.StatusBadGateway, codeUpstreamError, "could not confirm whether the PDS write committed")
				return out, false
			}
		}
		refs = completeAtomicRefs(sess, writes, refs)
		for i, write := range writes {
			if spec.Existence != nil && write.Collection == spec.ExistenceCollection && write.Rkey == spec.ExistenceRkey {
				out.ExistenceRef = refs[i]
			}
			if spec.Sidecar != nil && write.Collection == spec.SidecarCollection && write.Rkey == spec.SidecarCreateRkey {
				out.SidecarRkey = write.Rkey.String()
			}
		}
		return out, true
	}
	if spec.Existence != nil {
		ref, err := pds.CreateRecord(ctx, sess, spec.ExistenceCollection, spec.Existence)
		if err != nil {
			slog.Warn(spec.ExistenceOp, "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
			return out, false
		}
		out.ExistenceRef = ref
	}
	if spec.Sidecar != nil {
		if spec.SidecarRkey == "" {
			ref, err := pds.CreateRecord(ctx, sess, spec.SidecarCollection, spec.Sidecar)
			if err != nil {
				slog.Warn(spec.SidecarOp, "err", err)
				writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
				return out, false
			}
			out.SidecarRkey = atprepo.RkeyFromATURI(ref.URI)
		} else {
			if _, err := pds.PutRecord(ctx, sess, spec.SidecarCollection, spec.SidecarRkey, spec.Sidecar); err != nil {
				slog.Warn(spec.SidecarOp, "err", err)
				writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
				return out, false
			}
			out.SidecarRkey = spec.SidecarRkey
		}
	}
	return out, true
}

func completeAtomicRefs(sess *oauth.ClientSession, writes []atprepo.RecordWrite, refs []*atprepo.RecordRef) []*atprepo.RecordRef {
	out := make([]*atprepo.RecordRef, len(writes))
	for i, write := range writes {
		uri := "at://" + sess.Data.AccountDID.String() + "/" + write.Collection.String() + "/" + write.Rkey.String()
		out[i] = &atprepo.RecordRef{URI: uri}
		if i < len(refs) && refs[i] != nil && refs[i].URI == uri {
			out[i].CID = refs[i].CID
		}
	}
	return out
}

func confirmAtomicWrites(ctx context.Context, sess *oauth.ClientSession, pds atprepo.Writer, writes []atprepo.RecordWrite) ([]*atprepo.RecordRef, bool) {
	getter, ok := pds.(atprepo.RecordGetter)
	if !ok {
		return nil, false
	}
	refs := make([]*atprepo.RecordRef, len(writes))
	for i, write := range writes {
		record, err := getter.GetRecord(ctx, sess, write.Collection, write.Rkey)
		expectedURI := "at://" + sess.Data.AccountDID.String() + "/" + write.Collection.String() + "/" + write.Rkey.String()
		if err != nil || record == nil || record.URI != expectedURI || !sameRecordValue(record.Value, write.Record) {
			return nil, false
		}
		refs[i] = &atprepo.RecordRef{URI: record.URI, CID: record.CID}
	}
	return refs, true
}

func sameRecordValue(got, want map[string]any) bool {
	if _, expected := want["$type"]; !expected {
		normalizedGot := make(map[string]any, len(got))
		for key, value := range got {
			if key != "$type" {
				normalizedGot[key] = value
			}
		}
		got = normalizedGot
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		return false
	}
	wantJSON, err := json.Marshal(want)
	return err == nil && string(gotJSON) == string(wantJSON)
}
