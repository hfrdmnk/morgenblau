package atprepo

import (
	"context"
	"fmt"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"morgenblau/internal/session"
)

func (SessionWriter) GetLatestCommit(ctx context.Context, sess *session.Session) (string, error) {
	var out struct {
		CID string `json:"cid"`
	}
	err := sess.APIClient().Get(ctx, syntax.NSID("com.atproto.sync.getLatestCommit"), map[string]any{"did": sess.Data.AccountDID.String()}, &out)
	if err != nil {
		return "", err
	}
	if out.CID == "" {
		return "", fmt.Errorf("PDS returned an empty repository head")
	}
	return out.CID, nil
}

func (SessionWriter) CreateRecordIfCommit(ctx context.Context, sess *session.Session, collection syntax.NSID, record map[string]any, commit string) (*RecordRef, error) {
	if commit == "" {
		return nil, fmt.Errorf("conditional create requires a repository head")
	}
	body := struct {
		createRecordBody
		SwapCommit string `json:"swapCommit"`
	}{createRecordBody{Repo: sess.Data.AccountDID.String(), Collection: collection.String(), Record: record}, commit}
	var out RecordRef
	if err := sess.APIClient().Post(ctx, syntax.NSID("com.atproto.repo.createRecord"), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (SessionWriter) PutRecordIfCID(ctx context.Context, sess *session.Session, collection syntax.NSID, rkey string, record map[string]any, cid string) (*RecordRef, error) {
	if cid == "" {
		return nil, fmt.Errorf("conditional update requires a record CID")
	}
	body := struct {
		putRecordBody
		SwapRecord string `json:"swapRecord"`
	}{putRecordBody{Repo: sess.Data.AccountDID.String(), Collection: collection.String(), Rkey: rkey, Record: record}, cid}
	var out RecordRef
	if err := sess.APIClient().Post(ctx, syntax.NSID("com.atproto.repo.putRecord"), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
