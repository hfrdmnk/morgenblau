package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/cache/profiles"
	"morgenblau/internal/mail"
	"morgenblau/internal/middleware/ratelimit"
)

const feedbackReader = "did:plc:feedbackreader"

type fakeFeedbackSender struct {
	sent []mail.Message
	err  error
}

func (f *fakeFeedbackSender) Send(_ context.Context, m mail.Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

type fakeFeedbackProfiles struct{ err error }

func (f fakeFeedbackProfiles) Get(_ context.Context, did syntax.DID) (profiles.Profile, error) {
	if f.err != nil {
		return profiles.Profile{}, f.err
	}
	name := "Example Reader"
	return profiles.Profile{DID: did.String(), Handle: "reader.example", DisplayName: &name}, nil
}

type feedbackImage struct {
	name string
	data []byte
}

var (
	pngBytes  = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	jpegBytes = append([]byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00"), bytes.Repeat([]byte{0}, 64)...)
)

func feedbackRequest(t *testing.T, message string, context [][2]string, images ...feedbackImage) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("message", message)
	if context != nil {
		raw, _ := json.Marshal(context)
		_ = w.WriteField("context", string(raw))
	}
	for _, img := range images {
		part, err := w.CreateFormFile("images", img.name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(img.data)
	}
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("User-Agent", "Mozilla/5.0 (Example Browser)")
	return withSession(req, feedbackReader, "sid-1")
}

func newFeedbackHandler(sender FeedbackSender) http.Handler {
	return FeedbackHandler(sender, "owner@example.com", fakeFeedbackProfiles{}, ratelimit.New(100, time.Hour, nil))
}

func TestFeedback_MailsTheMessageWithAccountBrowserContextAndImages(t *testing.T) {
	sender := &fakeFeedbackSender{}
	rr := httptest.NewRecorder()
	newFeedbackHandler(sender).ServeHTTP(rr, feedbackRequest(t,
		"  The digest skipped Tuesday.\nIt showed Monday twice.  ",
		[][2]string{{"Page", "https://app.example.com/?date=2026-11-10"}, {"Viewport", "390 × 844 @3x"}},
		feedbackImage{"screenshot.png", pngBytes},
		feedbackImage{"photo.jpg", jpegBytes},
	))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s; want 204", rr.Code, rr.Body)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sender.sent))
	}
	m := sender.sent[0]
	if m.To != "owner@example.com" {
		t.Errorf("To = %q", m.To)
	}
	if m.Subject != "Feedback from @reader.example: The digest skipped Tuesday." {
		t.Errorf("Subject = %q", m.Subject)
	}
	for _, want := range []string{
		"The digest skipped Tuesday.\nIt showed Monday twice.\n",
		"@reader.example",
		"Example Reader",
		feedbackReader,
		"https://bsky.app/profile/" + feedbackReader,
		"https://app.example.com/?date=2026-11-10",
		"390 × 844 @3x",
		"Mozilla/5.0 (Example Browser)",
	} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("Text is missing %q:\n%s", want, m.Text)
		}
	}
	if !strings.HasPrefix(m.Text, "The digest skipped Tuesday.") {
		t.Errorf("Text should open with the trimmed message:\n%s", m.Text)
	}
	if len(m.Attachments) != 2 {
		t.Fatalf("attachments = %d, want 2", len(m.Attachments))
	}
	if a := m.Attachments[0]; a.Filename != "screenshot.png" || a.ContentType != "image/png" || !bytes.Equal(a.Data, pngBytes) {
		t.Errorf("first attachment = %s %s (%d bytes)", a.Filename, a.ContentType, len(a.Data))
	}
	if a := m.Attachments[1]; a.Filename != "photo.jpg" || a.ContentType != "image/jpeg" {
		t.Errorf("second attachment = %s %s", a.Filename, a.ContentType)
	}
}

func TestFeedback_SendsWithTheDIDWhenTheProfileIsUnavailable(t *testing.T) {
	sender := &fakeFeedbackSender{}
	rr := httptest.NewRecorder()
	FeedbackHandler(sender, "owner@example.com", fakeFeedbackProfiles{err: errors.New("pds down")}, ratelimit.New(100, time.Hour, nil)).
		ServeHTTP(rr, feedbackRequest(t, "Hello", nil))

	if rr.Code != http.StatusNoContent || len(sender.sent) != 1 {
		t.Fatalf("status = %d, sent = %d; want 204 and one message", rr.Code, len(sender.sent))
	}
	if got := sender.sent[0].Subject; got != "Feedback from "+feedbackReader+": Hello" {
		t.Errorf("Subject = %q", got)
	}
}

func TestFeedback_RejectsWhatItCannotSend(t *testing.T) {
	sixImages := make([]feedbackImage, 6)
	for i := range sixImages {
		sixImages[i] = feedbackImage{"shot.png", pngBytes}
	}
	cases := map[string]struct {
		req  *http.Request
		code int
	}{
		"blank message":    {feedbackRequest(t, "  \n ", nil), http.StatusBadRequest},
		"message too long": {feedbackRequest(t, strings.Repeat("a", maxFeedbackMessageRunes+1), nil), http.StatusBadRequest},
		"not an image":     {feedbackRequest(t, "Hi", nil, feedbackImage{"notes.png", []byte("plain text pretending")}), http.StatusBadRequest},
		"too many images":  {feedbackRequest(t, "Hi", nil, sixImages...), http.StatusBadRequest},
		"images too large": {feedbackRequest(t, "Hi", nil,
			feedbackImage{"a.png", append(append([]byte{}, pngBytes...), make([]byte, maxFeedbackImagesTotalBytes/2)...)},
			feedbackImage{"b.png", append(append([]byte{}, pngBytes...), make([]byte, maxFeedbackImagesTotalBytes/2)...)},
		), http.StatusRequestEntityTooLarge},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sender := &fakeFeedbackSender{}
			rr := httptest.NewRecorder()
			newFeedbackHandler(sender).ServeHTTP(rr, tc.req)
			if rr.Code != tc.code {
				t.Errorf("status = %d, body = %s; want %d", rr.Code, rr.Body, tc.code)
			}
			if len(sender.sent) != 0 {
				t.Errorf("sent %d messages, want none", len(sender.sent))
			}
		})
	}
}

func TestFeedback_ReportsAFailedSendAndAnUnconfiguredServer(t *testing.T) {
	rr := httptest.NewRecorder()
	newFeedbackHandler(&fakeFeedbackSender{err: errors.New("relay refused")}).ServeHTTP(rr, feedbackRequest(t, "Hi", nil))
	if rr.Code != http.StatusBadGateway {
		t.Errorf("failed send: status = %d, want 502", rr.Code)
	}

	rr = httptest.NewRecorder()
	newFeedbackHandler(nil).ServeHTTP(rr, feedbackRequest(t, "Hi", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("unconfigured: status = %d, want 503", rr.Code)
	}
}

func TestFeedback_LimitsEachReader(t *testing.T) {
	sender := &fakeFeedbackSender{}
	h := FeedbackHandler(sender, "owner@example.com", fakeFeedbackProfiles{}, ratelimit.New(2, time.Hour, nil))
	codes := []int{}
	for range 3 {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, feedbackRequest(t, "Hi", nil))
		codes = append(codes, rr.Code)
	}
	if codes[0] != http.StatusNoContent || codes[1] != http.StatusNoContent || codes[2] != http.StatusTooManyRequests {
		t.Errorf("codes = %v, want [204 204 429]", codes)
	}
	if len(sender.sent) != 2 {
		t.Errorf("sent %d, want 2", len(sender.sent))
	}
}
