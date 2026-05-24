// Package atprepo provides a thin authenticated wrapper around the
// com.atproto.repo.* XRPC endpoints (createRecord, putRecord, deleteRecord).
// Hides the JSON shape of each call behind typed Go methods.
package atprepo

import (
	"context"
	"strings"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

// RkeyFromATURI extracts the rkey segment from an at-uri like
// at://did:plc:alice/blue.morgen.feed.subscription/3la123.
func RkeyFromATURI(uri string) string {
	parts := strings.Split(uri, "/")
	if len(parts) != 5 || parts[0] != "at:" || parts[1] != "" || parts[2] == "" || parts[3] == "" || parts[4] == "" {
		return ""
	}
	return parts[4]
}

// RecordRef identifies a PDS-resident record by at-uri + CID.
type RecordRef struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

// Writer is the slice of PDS operations the subscription endpoints use.
// Production wires SessionWriter; handler tests inject a fake.
type Writer interface {
	CreateRecord(ctx context.Context, sess *oauth.ClientSession, collection syntax.NSID, record map[string]any) (*RecordRef, error)
	PutRecord(ctx context.Context, sess *oauth.ClientSession, collection syntax.NSID, rkey string, record map[string]any) (*RecordRef, error)
	DeleteRecord(ctx context.Context, sess *oauth.ClientSession, collection syntax.NSID, rkey string) error
}

// SessionWriter calls the session's authenticated APIClient.
type SessionWriter struct{}

type createRecordBody struct {
	Repo       string         `json:"repo"`
	Collection string         `json:"collection"`
	Record     map[string]any `json:"record"`
}

type putRecordBody struct {
	Repo       string         `json:"repo"`
	Collection string         `json:"collection"`
	Rkey       string         `json:"rkey"`
	Record     map[string]any `json:"record"`
}

type deleteRecordBody struct {
	Repo       string `json:"repo"`
	Collection string `json:"collection"`
	Rkey       string `json:"rkey"`
}

func (SessionWriter) CreateRecord(ctx context.Context, sess *oauth.ClientSession, collection syntax.NSID, record map[string]any) (*RecordRef, error) {
	body := createRecordBody{
		Repo:       sess.Data.AccountDID.String(),
		Collection: collection.String(),
		Record:     record,
	}
	var out RecordRef
	if err := sess.APIClient().Post(ctx, syntax.NSID("com.atproto.repo.createRecord"), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (SessionWriter) PutRecord(ctx context.Context, sess *oauth.ClientSession, collection syntax.NSID, rkey string, record map[string]any) (*RecordRef, error) {
	body := putRecordBody{
		Repo:       sess.Data.AccountDID.String(),
		Collection: collection.String(),
		Rkey:       rkey,
		Record:     record,
	}
	var out RecordRef
	if err := sess.APIClient().Post(ctx, syntax.NSID("com.atproto.repo.putRecord"), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (SessionWriter) DeleteRecord(ctx context.Context, sess *oauth.ClientSession, collection syntax.NSID, rkey string) error {
	body := deleteRecordBody{
		Repo:       sess.Data.AccountDID.String(),
		Collection: collection.String(),
		Rkey:       rkey,
	}
	return sess.APIClient().Post(ctx, syntax.NSID("com.atproto.repo.deleteRecord"), body, nil)
}
