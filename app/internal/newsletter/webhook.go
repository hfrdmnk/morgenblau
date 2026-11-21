package newsletter

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	webhookMaxClockSkew = 5 * time.Minute
	// A body of several megabytes needs longer than the server-wide read and write timeouts.
	webhookTransferTimeout = 2 * time.Minute
	// Each request buffers its whole body before the signature can be checked, so few may do it at once.
	webhookConcurrentRequests = 2
)

type webhookEnvelope struct {
	To   []string `json:"to"`
	From string   `json:"from"`
}

// NewPostShibaHandler receives the sendgrid_parse webhook of a PostShiba catch-all inbox: the raw message in "email" and the SMTP envelope in "envelope".
func NewPostShibaHandler(service *Service, secret string, maxMessageBytes int64) http.Handler {
	slots := make(chan struct{}, webhookConcurrentRequests)
	// The form percent-encodes the raw message and may repeat its parts in text, html and attachment fields.
	maxBodyBytes := 4 * maxMessageBytes
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		case <-r.Context().Done():
			return
		}
		controller := http.NewResponseController(w)
		deadline := time.Now().Add(webhookTransferTimeout)
		if err := controller.SetReadDeadline(deadline); err != nil {
			slog.Error("newsletter webhook read deadline", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := controller.SetWriteDeadline(deadline); err != nil {
			slog.Error("newsletter webhook write deadline", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !validWebhookSignature(secret, r.Header.Get("X-Capsule-Timestamp"), r.Header.Get("X-Capsule-Signature"), body, service.now()) {
			slog.Warn("newsletter webhook rejected", "reason", "signature", "timestamp", r.Header.Get("X-Capsule-Timestamp"), "has_signature", r.Header.Get("X-Capsule-Signature") != "", "bytes", len(body))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		form, err := webhookForm(r.Header.Get("Content-Type"), body)
		if errors.Is(err, errUnsupportedWebhookType) {
			slog.Warn("newsletter webhook rejected", "reason", "content type", "content_type", r.Header.Get("Content-Type"))
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		if err != nil {
			slog.Warn("newsletter webhook rejected", "reason", "form", "content_type", r.Header.Get("Content-Type"), "err", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		raw := []byte(form.Get("email"))
		var envelope webhookEnvelope
		if len(raw) == 0 || json.Unmarshal([]byte(form.Get("envelope")), &envelope) != nil || len(envelope.To) == 0 {
			// Field names and the envelope's keys show the payload's shape without logging reader addresses.
			slog.Warn("newsletter webhook rejected", "reason", "fields", "fields", slices.Sorted(maps.Keys(form)), "email_bytes", len(raw), "envelope_keys", jsonKeys(form.Get("envelope")))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if int64(len(raw)) > maxMessageBytes {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		recipients := make([]deliveryRecipient, 0, len(envelope.To))
		for _, address := range envelope.To {
			recipient, err := service.resolveRecipient(r.Context(), address)
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalid) {
				continue
			}
			if err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			recipients = append(recipients, recipient)
		}
		// A catch-all inbox accepts every address, so mail for unknown ones is dropped here instead of refused in SMTP.
		if len(recipients) == 0 {
			slog.Info("newsletter webhook dropped a message with no known recipient", "recipients", len(envelope.To))
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err := service.acceptReceipts(r.Context(), strings.TrimSpace(envelope.From), recipients, raw); err != nil {
			slog.Error("newsletter webhook receipt", "err", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func jsonKeys(value string) []string {
	var object map[string]json.RawMessage
	if json.Unmarshal([]byte(value), &object) != nil {
		return nil
	}
	return slices.Sorted(maps.Keys(object))
}

var errUnsupportedWebhookType = errors.New("unsupported webhook content type")

// webhookForm reads the text fields of either body PostShiba's docs describe for sendgrid_parse: URL-encoded, or multipart like SendGrid's own.
func webhookForm(contentType string, body []byte) (url.Values, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, errUnsupportedWebhookType
	}
	switch mediaType {
	case "application/x-www-form-urlencoded":
		return url.ParseQuery(string(body))
	case "multipart/form-data":
		form := url.Values{}
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				return form, nil
			}
			if err != nil {
				return nil, err
			}
			// Attachments are already inside the raw message in "email".
			if part.FileName() != "" {
				continue
			}
			value, err := io.ReadAll(part)
			if err != nil {
				return nil, err
			}
			form.Add(part.FormName(), string(value))
		}
	default:
		return nil, errUnsupportedWebhookType
	}
}

// validWebhookSignature checks PostShiba's HMAC-SHA256 of "{timestamp}.{body}"; every retry is signed afresh, so an old timestamp is a replay.
func validWebhookSignature(secret, timestamp, signature string, body []byte, now time.Time) bool {
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if skew := now.Sub(time.Unix(seconds, 0)); skew > webhookMaxClockSkew || skew < -webhookMaxClockSkew {
		return false
	}
	given, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return hmac.Equal(given, mac.Sum(nil))
}
