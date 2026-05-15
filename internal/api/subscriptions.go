package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/middleware/auth"
)

const subscriptionCollection = "app.skyreader.feed.subscription"

// Lister fetches records from a PDS. Abstracted so handler tests don't need
// a real PDS — the production implementation is PDSLister below.
type Lister interface {
	ListRecords(ctx context.Context, did syntax.DID, collection string, sess *oauth.ClientSession) ([]map[string]any, error)
}

// SubscriptionsHandler reads app.skyreader.feed.subscription records from
// the session-bound user's PDS and passes them through verbatim. No local
// mirror in this slice — derived view per SPEC <atproto-lexicons>.
func SubscriptionsHandler(lister Lister) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			slog.Error("/api/subscriptions: no session in context (middleware bypassed?)")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		records, err := lister.ListRecords(r.Context(), sess.Data.AccountDID, subscriptionCollection, sess)
		if err != nil {
			slog.Warn("/api/subscriptions: PDS call failed", "did", sess.Data.AccountDID, "err", err)
			http.Error(w, "upstream PDS error", http.StatusBadGateway)
			return
		}
		if records == nil {
			records = []map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(records)
	})
}

// PDSLister is the production Lister, wired to indigo's atclient.
type PDSLister struct{}

// listRecordsResp matches the relevant subset of com.atproto.repo.listRecords.
type listRecordsResp struct {
	Records []map[string]any `json:"records"`
	Cursor  string           `json:"cursor"`
}

func (PDSLister) ListRecords(ctx context.Context, did syntax.DID, collection string, sess *oauth.ClientSession) ([]map[string]any, error) {
	client := sess.APIClient()
	var out listRecordsResp
	params := map[string]any{
		"repo":       did.String(),
		"collection": collection,
	}
	if err := client.Get(ctx, syntax.NSID("com.atproto.repo.listRecords"), params, &out); err != nil {
		return nil, err
	}
	return out.Records, nil
}

// Compile-time interface checks.
var (
	_ Lister                       = PDSLister{}
	_ atclient.AuthMethod          = (*oauth.ClientSession)(nil)
)
