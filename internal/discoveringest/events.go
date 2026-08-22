// Package discoveringest tails Jetstream for the reader network's records,
// bootstraps missing history from its Replay archive, and rebuilds discover's
// trending signals from the local mirror.
// SPEC <discovery> Global/Trending acquisition.
package discoveringest

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Event kinds on the v2 wire, named after the lexicon's $type fragments.
const (
	kindCommit   = "commit"
	kindIdentity = "identity"
	kindAccount  = "account"
	kindSync     = "sync"
	kindInfo     = "info"
)

// Commit operations. The archive spells the same three as its kind column.
const (
	actionCreate = "create"
	actionUpdate = "update"
	actionDelete = "delete"
)

// eventTypePrefix qualifies every payload variant of network.bsky.jetstream.subscribeEvents.
const eventTypePrefix = "network.bsky.jetstream.subscribeEvents#"

// commitOp is one record mutation, produced identically by a live #commit frame and by an archive commit row.
type commitOp struct {
	DID        string
	Collection string
	Rkey       string
	CID        string
	Action     string
	Record     json.RawMessage
}

// identityChange is a handle change. It deliberately carries no hosting status: identity and account arrive as separate events, and a merged shape would let identity's zero values read as a deactivation.
type identityChange struct {
	DID    string
	Handle string
}

// accountChange is a hosting-status change. It deliberately carries no handle, for the same reason identityChange carries no status.
type accountChange struct {
	DID    string
	Active bool
	Status string
}

// syncMarker says a repo diverged and its mirror must be re-derived from the PDS.
type syncMarker struct {
	DID string
}

// event is one decoded stream event. Kind names which payload field is set; an unrecognized variant leaves all of them nil but still carries Seq.
type event struct {
	Seq      int64
	Kind     string
	Commit   *commitOp
	Identity *identityChange
	Account  *accountChange
	Sync     *syncMarker
	Info     string
}

// streamError is a terminal error frame. The server closes the connection right after sending one, so the session ends and the reconnect ladder takes over.
type streamError struct {
	Name    string
	Message string
}

func (e *streamError) Error() string {
	return fmt.Sprintf("discoveringest: jetstream closed the stream: %s: %s", e.Name, e.Message)
}

type frame struct {
	Type    string          `json:"$type"`
	Payload json.RawMessage `json:"payload"`
	Error   string          `json:"error"`
	Message string          `json:"message"`
}

type commitPayload struct {
	Seq        int64           `json:"seq"`
	DID        string          `json:"did"`
	Operation  string          `json:"operation"`
	Collection string          `json:"collection"`
	Rkey       string          `json:"rkey"`
	CID        string          `json:"cid"`
	Record     json.RawMessage `json:"record"`
}

type identityPayload struct {
	Seq      int64  `json:"seq"`
	DID      string `json:"did"`
	Identity struct {
		Handle string `json:"handle"`
	} `json:"identity"`
}

type accountPayload struct {
	Seq     int64  `json:"seq"`
	DID     string `json:"did"`
	Account struct {
		Active bool   `json:"active"`
		Status string `json:"status"`
	} `json:"account"`
}

type syncPayload struct {
	Seq int64  `json:"seq"`
	DID string `json:"did"`
}

type infoPayload struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

// decodeFrame turns one websocket text frame into an event. An error frame returns *streamError; an unknown payload variant returns a kind-less event so its seq still advances the cursor.
func decodeFrame(data []byte) (event, error) {
	var f frame
	if err := json.Unmarshal(data, &f); err != nil {
		return event{}, err
	}
	switch f.Type {
	case "error":
		return event{}, &streamError{Name: f.Error, Message: f.Message}
	case "message":
	default:
		return event{}, fmt.Errorf("discoveringest: unknown frame type %q", f.Type)
	}

	var variant struct {
		Type string `json:"$type"`
		Seq  int64  `json:"seq"`
	}
	if err := json.Unmarshal(f.Payload, &variant); err != nil {
		return event{}, err
	}

	switch strings.TrimPrefix(variant.Type, eventTypePrefix) {
	case kindCommit:
		var p commitPayload
		if err := json.Unmarshal(f.Payload, &p); err != nil {
			return event{}, err
		}
		op := commitOp{DID: p.DID, Collection: p.Collection, Rkey: p.Rkey, CID: p.CID, Action: p.Operation}
		if p.Operation != actionDelete {
			op.Record = p.Record
		}
		return event{Seq: p.Seq, Kind: kindCommit, Commit: &op}, nil
	case kindIdentity:
		var p identityPayload
		if err := json.Unmarshal(f.Payload, &p); err != nil {
			return event{}, err
		}
		return event{Seq: p.Seq, Kind: kindIdentity, Identity: &identityChange{DID: p.DID, Handle: p.Identity.Handle}}, nil
	case kindAccount:
		var p accountPayload
		if err := json.Unmarshal(f.Payload, &p); err != nil {
			return event{}, err
		}
		return event{Seq: p.Seq, Kind: kindAccount, Account: &accountChange{DID: p.DID, Active: p.Account.Active, Status: p.Account.Status}}, nil
	case kindSync:
		var p syncPayload
		if err := json.Unmarshal(f.Payload, &p); err != nil {
			return event{}, err
		}
		return event{Seq: p.Seq, Kind: kindSync, Sync: &syncMarker{DID: p.DID}}, nil
	case kindInfo:
		var p infoPayload
		if err := json.Unmarshal(f.Payload, &p); err != nil {
			return event{}, err
		}
		return event{Kind: kindInfo, Info: p.Name}, nil
	default:
		return event{Seq: variant.Seq}, nil
	}
}

// knownAction guards the mirror write against an operation the stream may add later.
func knownAction(action string) bool {
	return action == actionCreate || action == actionUpdate || action == actionDelete
}
