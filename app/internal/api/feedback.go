package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"morgenblau/internal/mail"
	"morgenblau/internal/middleware/ratelimit"
)

const (
	maxFeedbackMessageRunes = 5000
	maxFeedbackImages       = 5
	// maxFeedbackImageBytes caps all images together, so the encoded mail stays under common relay limits.
	maxFeedbackImageBytes = 12 << 20
	maxFeedbackContext    = 40
	maxFeedbackValueRunes = 500
)

// The formats every mail client previews inline, by sniffed type; anything else is refused.
var feedbackImageExtensions = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}

// FeedbackSender is the slice of *mail.Sender the feedback route uses.
type FeedbackSender interface {
	Send(ctx context.Context, m mail.Message) error
}

// invalidFeedback is a refusal the reader can act on; its text is the response message.
type invalidFeedback string

func (e invalidFeedback) Error() string { return string(e) }

var errFeedbackImagesTooLarge = errors.New("feedback images exceed the size cap")

type feedback struct {
	message string
	context [][2]string
	images  []mail.Attachment
}

// FeedbackHandler mails a reader's message, images and browser details to the owner; a nil sender means FEEDBACK_EMAIL is unset.
func FeedbackHandler(sender FeedbackSender, to string, profiles ProfileSource, limiter *ratelimit.Limiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		if sender == nil {
			writeError(w, http.StatusServiceUnavailable, codeUnavailable, "Feedback isn’t set up on this server.")
			return
		}
		did := sess.Data.AccountDID
		if _, ok := limiter.Allow(did.String()); !ok {
			writeError(w, http.StatusTooManyRequests, codeRateLimited, "You’ve sent a lot of feedback this hour. Try again later.")
			return
		}

		fb, err := readFeedback(r)
		if err != nil {
			var maxErr *http.MaxBytesError
			var invalid invalidFeedback
			switch {
			case errors.As(err, &maxErr) || errors.Is(err, errFeedbackImagesTooLarge):
				writeError(w, http.StatusRequestEntityTooLarge, codePayloadTooLarge, fmt.Sprintf("Images can add up to %d MB.", maxFeedbackImageBytes>>20))
			case errors.As(err, &invalid):
				writeError(w, http.StatusBadRequest, codeInvalidRequest, invalid.Error())
			default:
				writeError(w, http.StatusBadRequest, codeInvalidRequest, "Couldn’t read your feedback.")
			}
			return
		}

		var account [][2]string
		from := did.String()
		if p, err := profiles.Get(r.Context(), did); err != nil {
			slog.Warn("feedback: profile unavailable", "did", did, "err", err)
		} else {
			from = "@" + p.Handle
			account = append(account, [2]string{"Handle", from})
			if p.DisplayName != nil && *p.DisplayName != "" {
				account = append(account, [2]string{"Name", *p.DisplayName})
			}
		}
		account = append(account,
			[2]string{"DID", did.String()},
			[2]string{"Profile", "https://bsky.app/profile/" + did.String()},
		)
		browser := append(fb.context, [2]string{"User agent", r.UserAgent()})
		server := [][2]string{{"Received", time.Now().UTC().Format("2006-01-02 15:04:05 MST")}}

		var text strings.Builder
		text.WriteString(fb.message + "\n")
		for _, section := range []struct {
			title string
			rows  [][2]string
		}{{"Account", account}, {"Browser", browser}, {"Server", server}} {
			text.WriteString("\n" + section.title + "\n")
			for _, row := range section.rows {
				text.WriteString(row[0] + ": " + row[1] + "\n")
			}
		}

		err = sender.Send(r.Context(), mail.Message{
			To:          to,
			Subject:     "Feedback from " + from + ": " + truncateRunes(strings.SplitN(fb.message, "\n", 2)[0], 60),
			Text:        text.String(),
			Attachments: fb.images,
		})
		if err != nil {
			slog.Error("feedback: send failed", "did", did, "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "Couldn’t send your feedback. Try again in a moment.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func readFeedback(r *http.Request) (feedback, error) {
	var fb feedback
	parts, err := r.MultipartReader()
	if err != nil {
		return fb, invalidFeedback("Send feedback as multipart form data.")
	}
	imageBytes := 0
	for {
		part, err := parts.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fb, err
		}
		switch part.FormName() {
		case "message":
			raw, err := io.ReadAll(io.LimitReader(part, maxFeedbackMessageRunes*utf8.UTFMax+1))
			if err != nil {
				return fb, err
			}
			fb.message = strings.TrimSpace(strings.ReplaceAll(string(raw), "\r\n", "\n"))
		case "context":
			if err := json.NewDecoder(io.LimitReader(part, 64<<10)).Decode(&fb.context); err != nil {
				return fb, invalidFeedback("The browser details are malformed.")
			}
		case "images":
			if len(fb.images) == maxFeedbackImages {
				return fb, invalidFeedback(fmt.Sprintf("Attach up to %d images.", maxFeedbackImages))
			}
			data, err := io.ReadAll(io.LimitReader(part, int64(maxFeedbackImageBytes-imageBytes+1)))
			if err != nil {
				return fb, err
			}
			imageBytes += len(data)
			if imageBytes > maxFeedbackImageBytes {
				return fb, errFeedbackImagesTooLarge
			}
			contentType := http.DetectContentType(data)
			ext, ok := feedbackImageExtensions[contentType]
			if !ok {
				return fb, invalidFeedback("Attach images as PNG, JPEG, GIF or WebP.")
			}
			name := part.FileName()
			if name == "" {
				name = "image-" + strconv.Itoa(len(fb.images)+1) + ext
			}
			fb.images = append(fb.images, mail.Attachment{Filename: name, ContentType: contentType, Data: data})
		}
	}

	if fb.message == "" {
		return fb, invalidFeedback("Write a message.")
	}
	if utf8.RuneCountInString(fb.message) > maxFeedbackMessageRunes {
		return fb, invalidFeedback(fmt.Sprintf("Keep your message under %d characters.", maxFeedbackMessageRunes))
	}
	if len(fb.context) > maxFeedbackContext {
		fb.context = fb.context[:maxFeedbackContext]
	}
	for i, row := range fb.context {
		fb.context[i] = [2]string{truncateRunes(oneLine(row[0]), 40), truncateRunes(oneLine(row[1]), maxFeedbackValueRunes)}
	}
	return fb, nil
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
