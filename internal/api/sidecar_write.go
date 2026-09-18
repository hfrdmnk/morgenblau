package api

import (
	"context"
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
	ExistenceOp         string // slog op logged if the existence write fails

	Sidecar           map[string]any
	SidecarCollection syntax.NSID
	SidecarRkey       string // existing rkey: puts here instead of creating
	SidecarOp         string // slog op logged if the sidecar write fails
}

// sidecarWriteResult is what writeSidecarPair actually wrote, for the caller to fold into its Tier-1 mirror.
type sidecarWriteResult struct {
	ExistenceRef *atprepo.RecordRef
	SidecarRkey  string // set whether created or put; empty if spec.Sidecar was nil
}

// writeSidecarPair runs the existence-then-sidecar PDS sequence, writing the 502 itself on failure; existence goes first so a sidecar failure leaves an adoptable bare record (SPEC <sync-architecture>).
func writeSidecarPair(ctx context.Context, w http.ResponseWriter, sess *oauth.ClientSession, pds atprepo.Writer, spec sidecarWriteSpec) (sidecarWriteResult, bool) {
	var out sidecarWriteResult
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
