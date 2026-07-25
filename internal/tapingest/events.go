// Package tapingest mirrors a tap sidecar's firehose stream into the local
// record mirror and rebuilds discover's trending signals from it.
// SPEC <discovery> Global/Trending acquisition.
package tapingest

import "encoding/json"

// Envelope types tap emits; anything else is acked and ignored so a newer tap can add event kinds without stalling us.
const (
	eventTypeRecord   = "record"
	eventTypeIdentity = "identity"
)

// Record actions tap emits. Backfill sends only create and update.
const (
	actionCreate = "create"
	actionUpdate = "update"
	actionDelete = "delete"
)

// Envelope is one tap websocket message; the payload field matching Type is the one that is set.
type Envelope struct {
	ID       uint64         `json:"id"`
	Type     string         `json:"type"`
	Record   *RecordEvent   `json:"record,omitempty"`
	Identity *IdentityEvent `json:"identity,omitempty"`
}

// RecordEvent is one repo record change. Record is absent on deletes and whenever tap's own decode failed, so its absence is not an error.
type RecordEvent struct {
	Live       bool            `json:"live"`
	DID        string          `json:"did"`
	Rev        string          `json:"rev"`
	Collection string          `json:"collection"`
	Rkey       string          `json:"rkey"`
	Action     string          `json:"action"`
	Record     json.RawMessage `json:"record,omitempty"`
	CID        string          `json:"cid"`
}

// IdentityEvent is emitted only when a handle actually changes; tap never sends a baseline identity for a tracked repo.
type IdentityEvent struct {
	DID      string `json:"did"`
	Handle   string `json:"handle"`
	IsActive bool   `json:"is_active"`
	Status   string `json:"status"`
}

// ackMessage acknowledges one envelope. A live event is a per-DID barrier: until it is acked, that repo's stream stalls.
type ackMessage struct {
	Type string `json:"type"`
	ID   uint64 `json:"id"`
}

// parseEvent decodes one envelope. A partial decode still yields the id it managed to read, so the caller can ack an event it will never be able to process; id 0 means even that failed.
func parseEvent(data []byte) (Envelope, error) {
	var env Envelope
	err := json.Unmarshal(data, &env)
	if err != nil {
		var idOnly struct {
			ID uint64 `json:"id"`
		}
		if json.Unmarshal(data, &idOnly) == nil {
			env.ID = idOnly.ID
		}
	}
	return env, err
}

func knownAction(action string) bool {
	return action == actionCreate || action == actionUpdate || action == actionDelete
}
