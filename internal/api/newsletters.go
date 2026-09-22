package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"morgenblau/internal/newsletter"
)

type newsletterAddressService interface {
	Address(context.Context, string) (string, error)
	CreateAddress(context.Context, string) (string, error)
}

type newsletterSourceService interface {
	ListSources(context.Context, string) (newsletter.SourceGroups, error)
	GetSource(context.Context, string, string) (newsletter.Source, error)
	PatchSource(context.Context, string, string, newsletter.SourcePatch) (newsletter.Source, error)
	StopSource(context.Context, string, string) (newsletter.Source, error)
	EnableSource(context.Context, string, string) (newsletter.Source, error)
	ListSourceMessages(context.Context, string, string) ([]newsletter.Message, error)
}

type newsletterMessageService interface {
	AllowRemoteImages(context.Context, string, string) (newsletter.Message, error)
	MoveMessage(context.Context, string, string, newsletter.MoveTarget) (newsletter.Source, error)
}

type newsletterSaveService interface {
	SaveMessage(context.Context, string, string) (newsletter.Save, error)
	DeleteSave(context.Context, string, string) error
}

type newsletterAssetService interface {
	GetInlineAsset(context.Context, string, string) (newsletter.InlineAsset, error)
}

type newsletterSourceWire struct {
	ID             string                  `json:"id"`
	Kind           string                  `json:"kind"`
	Title          string                  `json:"title"`
	SenderName     *string                 `json:"senderName"`
	SenderAddress  string                  `json:"senderAddress"`
	Primary        bool                    `json:"primary"`
	Tags           []string                `json:"tags"`
	Status         newsletter.SourceStatus `json:"status"`
	LastReceivedAt *time.Time              `json:"lastReceivedAt"`
	Frequency      string                  `json:"frequency"`
	IssueCount     int64                   `json:"issueCount"`
	SavedByYou     int64                   `json:"savedByYou"`
}

type newsletterSourcesWire struct {
	Active  []newsletterSourceWire `json:"active"`
	Stopped []newsletterSourceWire `json:"stopped"`
}

type NewsletterMessageMeta struct {
	MessageID              string  `json:"messageId"`
	SenderName             *string `json:"senderName"`
	SenderAddress          string  `json:"senderAddress"`
	SentAt                 *string `json:"sentAt"`
	HasBlockedRemoteImages bool    `json:"hasBlockedRemoteImages"`
	RemoteImagesAllowed    bool    `json:"remoteImagesAllowed"`
}

func NewsletterAddressHandler(service newsletterAddressService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateResponse(w)
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		address, err := service.Address(r.Context(), sess.Data.AccountDID.String())
		if errors.Is(err, newsletter.ErrNotFound) {
			writeJSON(w, map[string]string{})
			return
		}
		if err != nil {
			writeNewsletterError(w, err)
			return
		}
		writeJSON(w, map[string]string{"address": address})
	})
}

func NewsletterAddressCreateHandler(service newsletterAddressService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateResponse(w)
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		address, err := service.CreateAddress(r.Context(), sess.Data.AccountDID.String())
		if err != nil {
			writeNewsletterError(w, err)
			return
		}
		writeJSON(w, map[string]string{"address": address})
	})
}

func NewslettersListHandler(service newsletterSourceService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateResponse(w)
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		groups, err := service.ListSources(r.Context(), sess.Data.AccountDID.String())
		if err != nil {
			writeNewsletterError(w, err)
			return
		}
		writeJSON(w, newsletterSourcesWire{
			Active:  newsletterSourcesToWire(groups.Active),
			Stopped: newsletterSourcesToWire(groups.Stopped),
		})
	})
}

func NewsletterGetHandler(service newsletterSourceService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateResponse(w)
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		source, err := service.GetSource(r.Context(), sess.Data.AccountDID.String(), r.PathValue("id"))
		if err != nil {
			writeNewsletterError(w, err)
			return
		}
		writeJSON(w, newsletterSourceToWire(source, time.Now()))
	})
}

func NewsletterPatchHandler(service newsletterSourceService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateResponse(w)
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var patch newsletter.SourcePatch
		if !decodeJSON(w, r, &patch) {
			return
		}
		source, err := service.PatchSource(r.Context(), sess.Data.AccountDID.String(), r.PathValue("id"), patch)
		if err != nil {
			writeNewsletterError(w, err)
			return
		}
		writeJSON(w, newsletterSourceToWire(source, time.Now()))
	})
}

func NewsletterStopHandler(service newsletterSourceService) http.Handler {
	return newsletterSourceActionHandler(service.StopSource)
}

func NewsletterEnableHandler(service newsletterSourceService) http.Handler {
	return newsletterSourceActionHandler(service.EnableSource)
}

func newsletterSourceActionHandler(action func(context.Context, string, string) (newsletter.Source, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateResponse(w)
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		source, err := action(r.Context(), sess.Data.AccountDID.String(), r.PathValue("id"))
		if err != nil {
			writeNewsletterError(w, err)
			return
		}
		writeJSON(w, newsletterSourceToWire(source, time.Now()))
	})
}

func NewsletterEntriesHandler(service newsletterSourceService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateResponse(w)
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		messages, err := service.ListSourceMessages(r.Context(), sess.Data.AccountDID.String(), r.PathValue("id"))
		if err != nil {
			writeNewsletterError(w, err)
			return
		}
		entries := make([]EntryWire, 0, len(messages))
		for _, message := range messages {
			entries = append(entries, newsletterMessageToWire(message))
		}
		writeJSON(w, entries)
	})
}

func NewsletterRemoteImagesHandler(service newsletterMessageService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateResponse(w)
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		message, err := service.AllowRemoteImages(r.Context(), sess.Data.AccountDID.String(), r.PathValue("id"))
		if err != nil {
			writeNewsletterError(w, err)
			return
		}
		writeJSON(w, map[string]any{
			"body": message.BodyHTML, "remoteImagesAllowed": message.RemoteImagesAllowed,
			"hasBlockedRemoteImages": message.HasBlockedRemoteImages,
		})
	})
}

func NewsletterMoveHandler(service newsletterMessageService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateResponse(w)
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body struct {
			SourceID       string `json:"sourceId"`
			NewSourceTitle string `json:"newSourceTitle"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		body.SourceID = strings.TrimSpace(body.SourceID)
		body.NewSourceTitle = strings.TrimSpace(body.NewSourceTitle)
		if (body.SourceID == "") == (body.NewSourceTitle == "") {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "provide exactly one destination")
			return
		}
		source, err := service.MoveMessage(r.Context(), sess.Data.AccountDID.String(), r.PathValue("id"), newsletter.MoveTarget{SourceID: body.SourceID, NewSourceTitle: body.NewSourceTitle})
		if err != nil {
			writeNewsletterError(w, err)
			return
		}
		writeJSON(w, map[string]any{"source": newsletterSourceToWire(source, time.Now())})
	})
}

func NewsletterSaveCreateHandler(service newsletterSaveService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateResponse(w)
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body struct {
			MessageID string `json:"messageId"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		body.MessageID = strings.TrimSpace(body.MessageID)
		if body.MessageID == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "messageId is required")
			return
		}
		save, err := service.SaveMessage(r.Context(), sess.Data.AccountDID.String(), body.MessageID)
		if err != nil {
			writeNewsletterError(w, err)
			return
		}
		writeJSONStatus(w, http.StatusCreated, map[string]string{"id": save.ID})
	})
}

func NewsletterSaveDeleteHandler(service newsletterSaveService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateResponse(w)
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		if err := service.DeleteSave(r.Context(), sess.Data.AccountDID.String(), r.PathValue("id")); err != nil {
			writeNewsletterError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func NewsletterAssetHandler(service newsletterAssetService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateResponse(w)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		asset, err := service.GetInlineAsset(r.Context(), sess.Data.AccountDID.String(), r.PathValue("token"))
		if err != nil {
			writeNewsletterError(w, err)
			return
		}
		w.Header().Set("Content-Type", asset.MediaType)
		if asset.ETag != "" {
			w.Header().Set("ETag", `"`+strings.Trim(asset.ETag, `"`)+`"`)
		}
		_, _ = w.Write(asset.Data)
	})
}

func newsletterSourcesToWire(sources []newsletter.Source) []newsletterSourceWire {
	out := make([]newsletterSourceWire, 0, len(sources))
	now := time.Now()
	for _, source := range sources {
		out = append(out, newsletterSourceToWire(source, now))
	}
	return out
}

func newsletterSourceToWire(source newsletter.Source, now time.Time) newsletterSourceWire {
	first := ""
	if source.FirstReceivedAt != nil {
		first = source.FirstReceivedAt.Format(time.RFC3339)
	}
	tags := source.Tags
	if tags == nil {
		tags = []string{}
	}
	return newsletterSourceWire{
		ID: source.ID, Kind: "newsletter", Title: source.Title, SenderName: source.SenderName,
		SenderAddress: source.SenderAddress, Primary: source.Primary, Tags: tags, Status: source.Status,
		LastReceivedAt: source.LastReceivedAt,
		Frequency:      frequencyBucket(first, source.Count7d, source.Count28d, source.Count56d, source.Count84d, now),
		IssueCount:     source.IssueCount, SavedByYou: source.SavedCount,
	}
}

func newsletterMessageToWire(message newsletter.Message) EntryWire {
	body := message.BodyHTML
	title := message.Title
	sourceTitle := message.SourceTitle
	var sentAt *string
	if message.SentAt != nil {
		formatted := message.SentAt.Format(time.RFC3339)
		sentAt = &formatted
	}
	var saved *SavedState
	if message.SaveID != nil {
		saved = &SavedState{Kind: "newsletter", ID: *message.SaveID}
	}
	return EntryWire{
		ID: message.ID, EntrySlug: message.EntrySlug, Title: &title,
		ContentType: "newsletter", PublishedAt: message.ReceivedAt.Format(time.RFC3339),
		Source: SourceMeta{Kind: "newsletter", ID: message.SourceID, Title: &sourceTitle}, Body: &body,
		Newsletter: &NewsletterMessageMeta{
			MessageID: message.ID, SenderName: message.SenderName, SenderAddress: message.SenderAddress,
			SentAt: sentAt, HasBlockedRemoteImages: message.HasBlockedRemoteImages, RemoteImagesAllowed: message.RemoteImagesAllowed,
		},
		SavedState: saved,
	}
}

func privateResponse(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
}

func writeNewsletterError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, newsletter.ErrNotFound):
		writeError(w, http.StatusNotFound, codeNotFound, "not found")
	case errors.Is(err, newsletter.ErrInvalid):
		writeError(w, http.StatusBadRequest, codeInvalidRequest, "invalid request")
	case errors.Is(err, newsletter.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, "unavailable", "newsletter ingestion is unavailable")
	default:
		slog.Warn("newsletter API", "err", err)
		writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
	}
}
