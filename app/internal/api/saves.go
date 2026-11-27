package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"golang.org/x/net/html"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/lexicon"
	"morgenblau/internal/newsletter"
	"morgenblau/internal/session"
	"morgenblau/internal/tags"
)

const saveCollection = lexicon.Save

type SaveWire struct {
	URI         string      `json:"uri,omitempty"`
	CID         string      `json:"cid,omitempty"`
	Rkey        string      `json:"rkey"`
	ItemURL     string      `json:"itemUrl"`
	FeedURL     string      `json:"feedUrl,omitempty"`
	CreatedAt   string      `json:"createdAt"`
	Title       string      `json:"title,omitempty"`
	TargetURL   string      `json:"targetUrl,omitempty"`
	EntrySlug   string      `json:"entrySlug,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
	Source      *SourceMeta `json:"source,omitempty"`
	ContentType string      `json:"contentType,omitempty"`
	Minutes     int         `json:"minutes,omitempty"`
}

type SavesIndexReader interface {
	GetUserSave(ctx context.Context, arg db.GetUserSaveParams) (db.UserSave, error)
	GetUserSaveByItemURL(ctx context.Context, arg db.GetUserSaveByItemURLParams) (db.UserSave, error)
	ListUserSaves(ctx context.Context, did string) ([]db.ListUserSavesRow, error)
}

type SavesIndexWriter interface {
	UpsertUserSave(ctx context.Context, arg db.UpsertUserSaveParams) error
	DeleteUserSave(ctx context.Context, arg db.DeleteUserSaveParams) error
}

type newsletterSaveLister interface {
	ListSaves(context.Context, string) ([]newsletter.SaveItem, error)
}

type newsletterSaveWire struct {
	Kind      string     `json:"kind"`
	ID        string     `json:"id"`
	MessageID string     `json:"messageId"`
	CreatedAt string     `json:"createdAt"`
	Title     string     `json:"title,omitempty"`
	EntrySlug string     `json:"entrySlug"`
	Tags      []string   `json:"tags,omitempty"`
	Source    SourceMeta `json:"source"`
	Minutes   int        `json:"minutes,omitempty"`
}

type savesCreateRequest struct {
	ItemURL string `json:"itemUrl"`
	FeedURL string `json:"feedUrl"`
}

func SavesCreateHandler(reader SavesIndexReader, writer SavesIndexWriter, pds atprepo.Writer, disp RepairDispatcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body savesCreateRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		body.ItemURL = strings.TrimSpace(body.ItemURL)
		body.FeedURL = strings.TrimSpace(body.FeedURL)
		if body.ItemURL == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "itemUrl is required")
			return
		}
		didStr := sess.Data.AccountDID.String()

		if existing, err := reader.GetUserSaveByItemURL(r.Context(), db.GetUserSaveByItemURLParams{
			Did:     didStr,
			ItemUrl: body.ItemURL,
		}); err == nil {
			writeJSON(w, saveRowToWire(existing))
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("/api/saves: dedupe probe failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		record := map[string]any{
			"itemUrl":   body.ItemURL,
			"createdAt": now,
		}
		if body.FeedURL != "" {
			record["feedUrl"] = body.FeedURL
		}
		if err := lexicon.ValidateRecord(saveCollection, record); err != nil {
			slog.Warn("/api/saves: record failed lexicon validation", "err", err)
			writeError(w, http.StatusInternalServerError, codeInvalidRecord, "internal error")
			return
		}
		ref, ok := commitThenMirror(r.Context(), disp, sess, "/api/saves: Tier-1 upsert", func() (*atprepo.RecordRef, bool) {
			ref, err := pds.CreateRecord(r.Context(), sess, syntax.NSID(saveCollection), record)
			if err != nil {
				slog.Warn("/api/saves: PDS create failed", "err", err)
				writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
				return nil, false
			}
			return ref, true
		}, func(ref *atprepo.RecordRef) error {
			return writer.UpsertUserSave(r.Context(), db.UpsertUserSaveParams{
				Did:       didStr,
				Rkey:      atprepo.RkeyFromATURI(ref.URI),
				AtUri:     ref.URI,
				ItemUrl:   body.ItemURL,
				FeedUrl:   nilIfEmpty(body.FeedURL),
				CreatedAt: now,
				UpdatedAt: now,
			})
		})
		if !ok {
			return
		}
		rkey := atprepo.RkeyFromATURI(ref.URI)

		writeJSONStatus(w, http.StatusCreated, SaveWire{
			URI:       ref.URI,
			CID:       ref.CID,
			Rkey:      rkey,
			ItemURL:   body.ItemURL,
			FeedURL:   body.FeedURL,
			CreatedAt: now,
		})
	})
}

func SavesDeleteHandler(reader SavesIndexReader, writer SavesIndexWriter, pds atprepo.Writer, disp RepairDispatcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		rkey := r.PathValue("rkey")
		if rkey == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "rkey is required")
			return
		}
		didStr := sess.Data.AccountDID.String()
		_, err := reader.GetUserSave(r.Context(), db.GetUserSaveParams{Did: didStr, Rkey: rkey})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, codeNotFound, "not found")
				return
			}
			slog.Warn("/api/saves DELETE: load failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		if _, ok := commitThenMirror(r.Context(), disp, sess, "/api/saves DELETE: Tier-1 delete", func() (struct{}, bool) {
			if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(saveCollection), rkey); err != nil {
				slog.Warn("/api/saves DELETE: PDS delete failed", "err", err)
				writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
				return struct{}{}, false
			}
			return struct{}{}, true
		}, func(struct{}) error {
			return writer.DeleteUserSave(r.Context(), db.DeleteUserSaveParams{Did: didStr, Rkey: rkey})
		}); !ok {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type saveRecordRepo interface {
	atprepo.RecordGetter
	PutRecordIfCID(ctx context.Context, sess *session.Session, collection syntax.NSID, rkey string, record map[string]any, cid string) (*atprepo.RecordRef, error)
}

func SavesPatchHandler(reader SavesIndexReader, writer SavesIndexWriter, pds saveRecordRepo, disp RepairDispatcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body struct {
			Tags *[]string `json:"tags"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.Tags == nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "tags is required")
			return
		}
		didStr := sess.Data.AccountDID.String()
		rkey := r.PathValue("rkey")
		row, err := reader.GetUserSave(r.Context(), db.GetUserSaveParams{Did: didStr, Rkey: rkey})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, codeNotFound, "not found")
				return
			}
			slog.Warn("/api/saves PATCH: load failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		current, err := pds.GetRecord(r.Context(), sess, syntax.NSID(saveCollection), syntax.RecordKey(rkey))
		if err != nil {
			slog.Warn("/api/saves PATCH: PDS read failed", "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
			return
		}
		tagList := normalizeTags(*body.Tags)
		record := make(map[string]any, len(current.Value)+1)
		for key, value := range current.Value {
			record[key] = value
		}
		delete(record, "tags")
		if len(tagList) > 0 {
			record["tags"] = tagList
		}
		if err := lexicon.ValidateRecord(saveCollection, record); err != nil {
			slog.Warn("/api/saves PATCH: record failed lexicon validation", "err", err)
			writeError(w, http.StatusInternalServerError, codeInvalidRecord, "internal error")
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		ref, ok := commitThenMirror(r.Context(), disp, sess, "/api/saves PATCH: Tier-1 upsert", func() (*atprepo.RecordRef, bool) {
			ref, err := pds.PutRecordIfCID(r.Context(), sess, syntax.NSID(saveCollection), rkey, record, current.CID)
			if isSwapConflict(err) {
				writeError(w, http.StatusConflict, codeConflict, "This save changed elsewhere. Try again.")
				return nil, false
			}
			if err != nil {
				slog.Warn("/api/saves PATCH: PDS put failed", "err", err)
				writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
				return nil, false
			}
			return ref, true
		}, func(ref *atprepo.RecordRef) error {
			row.AtUri = ref.URI
			row.Tags = tags.Marshal(tagList)
			row.UpdatedAt = now
			return writer.UpsertUserSave(r.Context(), db.UpsertUserSaveParams{
				Did: row.Did, Rkey: row.Rkey, AtUri: row.AtUri, ItemUrl: row.ItemUrl, FeedUrl: row.FeedUrl,
				Tags: row.Tags, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			})
		})
		if !ok {
			return
		}
		wire := saveRowToWire(row)
		wire.CID = ref.CID
		writeJSON(w, wire)
	})
}

type saveTagsReader interface {
	ListUserSaveTags(ctx context.Context, did string) ([]*string, error)
}

type newsletterSaveTagLister interface {
	ListSaveTags(context.Context, string) ([]string, error)
}

func SavesTagsHandler(reader saveTagsReader, privateListers ...newsletterSaveTagLister) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		did := sess.Data.AccountDID.String()
		rows, err := reader.ListUserSaveTags(r.Context(), did)
		if err != nil {
			slog.Warn("/api/saves/tags: list failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		lists := make([][]string, 0, len(rows)+1)
		for _, row := range rows {
			lists = append(lists, tags.Unmarshal(row))
		}
		if len(privateListers) > 0 && privateListers[0] != nil {
			privateResponse(w)
			private, err := privateListers[0].ListSaveTags(r.Context(), did)
			if err != nil {
				writeNewsletterError(w, err)
				return
			}
			lists = append(lists, private)
		}
		writeJSON(w, tagsResponse{Tags: distinctTags(lists...)})
	})
}

func SavesListHandler(reader SavesIndexReader, privateListers ...newsletterSaveLister) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		rows, err := reader.ListUserSaves(r.Context(), sess.Data.AccountDID.String())
		if err != nil {
			slog.Warn("/api/saves GET: list failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		type datedSave struct {
			created time.Time
			value   any
		}
		out := make([]datedSave, 0, len(rows))
		for _, row := range rows {
			created, _ := time.Parse(time.RFC3339, row.CreatedAt)
			out = append(out, datedSave{created: created, value: saveListRowToWire(row)})
		}
		if len(privateListers) > 0 && privateListers[0] != nil {
			privateResponse(w)
			items, err := privateListers[0].ListSaves(r.Context(), sess.Data.AccountDID.String())
			if err != nil {
				writeNewsletterError(w, err)
				return
			}
			for _, item := range items {
				sourceTitle := item.SourceTitle
				out = append(out, datedSave{created: item.CreatedAt, value: newsletterSaveWire{
					Kind: "newsletter", ID: item.ID, MessageID: item.MessageID, CreatedAt: item.CreatedAt.Format(time.RFC3339),
					Title: item.Title, EntrySlug: item.EntrySlug, Tags: item.Tags,
					Source:  SourceMeta{Kind: "newsletter", ID: item.SourceID, Title: &sourceTitle},
					Minutes: readingMinutes(item.Text),
				}})
			}
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].created.After(out[j].created) })
		values := make([]any, 0, len(out))
		for _, item := range out {
			values = append(values, item.value)
		}
		writeJSON(w, values)
	})
}

func saveListRowToWire(row db.ListUserSavesRow) SaveWire {
	feedURL := derefStr(row.FeedUrl)
	if feedURL == "" {
		feedURL = derefStr(row.EntryFeedUrl)
	}
	wire := SaveWire{
		URI:       row.AtUri,
		Rkey:      row.Rkey,
		ItemURL:   row.ItemUrl,
		FeedURL:   feedURL,
		CreatedAt: row.CreatedAt,
		Title:     derefStr(row.EntryTitle),
		TargetURL: derefStr(row.EntryUrl),
		EntrySlug: derefStr(row.EntrySlug),
		Tags:      tags.Unmarshal(row.Tags),
	}
	if row.EntrySlug == nil {
		return wire
	}
	source := buildSourceMeta(derefStr(row.EntryFeedUrl), displayTitle(row.SubscriptionTitle, row.CatalogTitle), row.FeedSiteUrl, row.FeedIconUrl)
	source.Rkey = derefStr(row.SubscriptionRkey)
	wire.Source = &source
	wire.ContentType = derefStr(row.EntryContentType)
	body := derefStr(row.EntryExtractedBody)
	if body == "" {
		body = derefStr(row.EntryContentHtml)
	}
	if wire.ContentType != "video" {
		wire.Minutes = readingMinutes(htmlText(body))
	}
	return wire
}

const wordsPerMinute = 220

var blockElements = map[string]bool{
	"p": true, "div": true, "li": true, "br": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"blockquote": true, "figcaption": true, "tr": true, "td": true, "th": true, "pre": true, "section": true, "article": true,
}

func readingMinutes(text string) int {
	return (len(strings.Fields(text)) + wordsPerMinute - 1) / wordsPerMinute
}

func htmlText(body string) string {
	var b strings.Builder
	skip := 0
	z := html.NewTokenizer(strings.NewReader(body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.TextToken:
			if skip == 0 {
				b.WriteString(z.Token().Data)
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			switch tag := string(name); {
			case tag == "script" || tag == "style":
				skip++
			case blockElements[tag]:
				b.WriteByte(' ')
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch tag := string(name); {
			case (tag == "script" || tag == "style") && skip > 0:
				skip--
			case blockElements[tag]:
				b.WriteByte(' ')
			}
		}
	}
}

func saveRowToWire(row db.UserSave) SaveWire {
	feedURL := ""
	if row.FeedUrl != nil {
		feedURL = *row.FeedUrl
	}
	return SaveWire{
		URI:       row.AtUri,
		Rkey:      row.Rkey,
		ItemURL:   row.ItemUrl,
		FeedURL:   feedURL,
		CreatedAt: row.CreatedAt,
		Tags:      tags.Unmarshal(row.Tags),
	}
}
