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
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	webhookMaxClockSkew       = 5 * time.Minute
	webhookTransferTimeout    = 2 * time.Minute
	webhookConcurrentRequests = 2
)

type webhookEnvelope struct {
	To   recipientList `json:"rcpt_to"`
	From string        `json:"mail_from"`
}

// PostShiba's inbound payload shows envelope.rcpt_to as one address, not a list.
// https://www.postshiba.com/learn/inbound-mail
type recipientList []string

func (l *recipientList) UnmarshalJSON(data []byte) error {
	var one string
	if json.Unmarshal(data, &one) == nil {
		*l = nil
		if one != "" {
			*l = recipientList{one}
		}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*l = many
	return nil
}

func NewPostShibaHandler(service *Service, secret string, maxMessageBytes int64) http.Handler {
	slots := make(chan struct{}, webhookConcurrentRequests)
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
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		form, err := webhookForm(r.Header.Get("Content-Type"), body)
		if errors.Is(err, errUnsupportedWebhookType) {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		raw := []byte(form.Get("email"))
		var envelope webhookEnvelope
		if len(raw) == 0 || json.Unmarshal([]byte(form.Get("envelope")), &envelope) != nil || len(envelope.To) == 0 {
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

var errUnsupportedWebhookType = errors.New("unsupported webhook content type")

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
			// A sendgrid_parse webhook puts the raw MIME in "email" and sends its files as attachment1, attachment2 and so on.
			// https://www.postshiba.com/docs/dashboard/receiving/attachments
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

// PostShiba signs "{timestamp}.{body}" with HMAC-SHA256 and signs every retry with a fresh timestamp.
// https://www.postshiba.com/docs/webhooks/verify-webhooks-requests
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
