package newsletter

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testWebhookSecret = "example-inbox-webhook-secret"

// PostShiba's docs call the sendgrid_parse body both URL-encoded and multipart, so both must store the same bytes.
func TestPostShibaWebhookStoresTheRawMessageForEachKnownRecipient(t *testing.T) {
	for _, encoding := range []string{"urlencoded", "multipart"} {
		t.Run(encoding, func(t *testing.T) {
			testWebhookStoresRawMessage(t, encoding)
		})
	}
}

func testWebhookStoresRawMessage(t *testing.T, encoding string) {
	service, reader, _ := newTestService(t)
	ctx := context.Background()
	alice, _ := service.CreateAddress(ctx, "did:plc:alice")
	bob, _ := service.CreateAddress(ctx, "did:plc:bob")
	raw := "From: Example Publication <issues@example.com>\r\nSubject: Issue 1\r\nContent-Type: text/plain\r\n\r\nBody with + & = %41 and\r\nCRLF lines\r\n"
	form := url.Values{
		"email":    {raw},
		"envelope": {`{"to":["` + strings.ToUpper(alice) + `","unknown@news.example","` + bob + `"],"from":"bounce@example.com"}`},
		"subject":  {"Issue 1"},
	}
	body, contentType := form.Encode(), "application/x-www-form-urlencoded"
	if encoding == "multipart" {
		body, contentType = multipartWebhookBody(t, form)
	}
	request := signedWebhook(body, testNow, testWebhookSecret)
	request.contentType = contentType

	status := postWebhook(t, service, request)
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", status)
	}
	rows, err := reader.Query(`SELECT did, recipient, envelope_from, raw_mime FROM newsletter_receipts ORDER BY did`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var did, recipient, envelopeFrom string
		var stored []byte
		if err := rows.Scan(&did, &recipient, &envelopeFrom, &stored); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(stored, []byte(raw)) {
			t.Fatalf("stored MIME = %q, want the email field byte for byte", stored)
		}
		got = append(got, did+" "+recipient+" "+envelopeFrom)
	}
	want := []string{"did:plc:alice " + alice + " bounce@example.com", "did:plc:bob " + bob + " bounce@example.com"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("receipts:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestPostShibaWebhookRefusesUnsignedStaleAndTamperedRequests(t *testing.T) {
	service, reader, _ := newTestService(t)
	address, _ := service.CreateAddress(context.Background(), "did:plc:alice")
	body := url.Values{"email": {"From: issues@example.com\r\n\r\nBody\r\n"}, "envelope": {`{"to":["` + address + `"],"from":"bounce@example.com"}`}}.Encode()
	now := testNow
	for _, tc := range []struct {
		name   string
		at     time.Time
		secret string
		mutate func(*webhookRequest)
	}{
		{"unsigned", now, testWebhookSecret, func(r *webhookRequest) { r.timestamp, r.signature = "", "" }},
		{"other secret", now, "another-secret", nil},
		{"signature without timestamp", now, testWebhookSecret, func(r *webhookRequest) { r.timestamp = "" }},
		{"signature for another timestamp", now, testWebhookSecret, func(r *webhookRequest) { r.timestamp = strconv.FormatInt(now.Unix()+1, 10) }},
		{"tampered body", now, testWebhookSecret, func(r *webhookRequest) { r.body += "&x=1" }},
		{"six minutes old", now.Add(-6 * time.Minute), testWebhookSecret, nil},
		{"six minutes ahead", now.Add(6 * time.Minute), testWebhookSecret, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := signedWebhook(body, tc.at, tc.secret)
			if tc.mutate != nil {
				tc.mutate(&request)
			}
			if status := postWebhook(t, service, request); status != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", status)
			}
		})
	}
	if got := countRows(t, reader, "newsletter_receipts"); got != 0 {
		t.Fatalf("receipts = %d, want none from refused requests", got)
	}
	if status := postWebhook(t, service, signedWebhook(body, now.Add(-4*time.Minute), testWebhookSecret)); status != http.StatusNoContent {
		t.Fatalf("four-minute-old delivery status = %d, want 204", status)
	}
}

func TestPostShibaWebhookAcknowledgesMailForUnknownAddressesWithoutStoringIt(t *testing.T) {
	service, reader, _ := newTestService(t)
	form := url.Values{"email": {"From: issues@example.com\r\n\r\nBody\r\n"}, "envelope": {`{"to":["unknown@news.example","someone@other.example"],"from":"bounce@example.com"}`}}
	if status := postWebhook(t, service, signedWebhook(form.Encode(), testNow, testWebhookSecret)); status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 so PostShiba stops retrying", status)
	}
	if got := countRows(t, reader, "newsletter_receipts"); got != 0 {
		t.Fatalf("receipts = %d, want 0", got)
	}
}

func TestPostShibaWebhookRejectsMalformedAndOversizedMessages(t *testing.T) {
	service, reader, _ := newTestService(t)
	address, _ := service.CreateAddress(context.Background(), "did:plc:alice")
	envelope := `{"to":["` + address + `"],"from":"bounce@example.com"}`
	for _, tc := range []struct {
		name   string
		form   url.Values
		status int
	}{
		{"no raw message", url.Values{"envelope": {envelope}, "text": {"Body"}}, http.StatusBadRequest},
		{"no envelope", url.Values{"email": {"From: issues@example.com\r\n\r\nBody\r\n"}}, http.StatusBadRequest},
		{"message over the limit", url.Values{"email": {strings.Repeat("a", webhookTestMaxBytes+1)}, "envelope": {envelope}}, http.StatusRequestEntityTooLarge},
		{"body over the limit", url.Values{"email": {"From: issues@example.com\r\n\r\nBody\r\n"}, "envelope": {envelope}, "html": {strings.Repeat("a", 4*webhookTestMaxBytes)}}, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if status := postWebhook(t, service, signedWebhook(tc.form.Encode(), testNow, testWebhookSecret)); status != tc.status {
				t.Fatalf("status = %d, want %d", status, tc.status)
			}
		})
	}
	if got := countRows(t, reader, "newsletter_receipts"); got != 0 {
		t.Fatalf("receipts = %d, want 0", got)
	}
	exact := url.Values{"email": {strings.Repeat("a", webhookTestMaxBytes)}, "envelope": {envelope}}
	if status := postWebhook(t, service, signedWebhook(exact.Encode(), testNow, testWebhookSecret)); status != http.StatusNoContent {
		t.Fatalf("message at the limit status = %d, want 204", status)
	}
}

func TestPostShibaWebhookAsksForARetryWhenTheReceiptCannotCommit(t *testing.T) {
	service, _, writer := newTestService(t)
	address, _ := service.CreateAddress(context.Background(), "did:plc:alice")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	form := url.Values{"email": {"From: issues@example.com\r\n\r\nBody\r\n"}, "envelope": {`{"to":["` + address + `"],"from":"bounce@example.com"}`}}
	if status := postWebhook(t, service, signedWebhook(form.Encode(), testNow, testWebhookSecret)); status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 so PostShiba retries", status)
	}
}

const webhookTestMaxBytes = 4 << 10

type webhookRequest struct {
	body, timestamp, signature, contentType string
}

// multipartWebhookBody writes the fields the way SendGrid's Inbound Parse does, with a file part ahead of them.
func multipartWebhookBody(t *testing.T, form url.Values) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	file, err := w.CreateFormFile("attachment1", "issue.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("\x89PNG\r\n\x1a\n"))
	for _, name := range []string{"email", "envelope", "subject"} {
		if err := w.WriteField(name, form.Get(name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String(), w.FormDataContentType()
}

func signedWebhook(body string, at time.Time, secret string) webhookRequest {
	timestamp := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + body))
	return webhookRequest{body: body, timestamp: timestamp, signature: "sha256=" + hex.EncodeToString(mac.Sum(nil))}
}

func postWebhook(t *testing.T, service *Service, request webhookRequest) int {
	t.Helper()
	server := httptest.NewServer(NewPostShibaHandler(service, testWebhookSecret, webhookTestMaxBytes))
	defer server.Close()
	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(request.body))
	if err != nil {
		t.Fatal(err)
	}
	contentType := request.contentType
	if contentType == "" {
		contentType = "application/x-www-form-urlencoded"
	}
	req.Header.Set("Content-Type", contentType)
	if request.timestamp != "" {
		req.Header.Set("X-Capsule-Timestamp", request.timestamp)
	}
	if request.signature != "" {
		req.Header.Set("X-Capsule-Signature", request.signature)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}
