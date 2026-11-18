package mail

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"io"
	"math/big"
	"mime"
	"mime/multipart"
	"net"
	netmail "net/mail"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"
)

type received struct {
	user, pass string
	from       string
	to         []string
	data       []byte
}

type relay struct {
	mu   sync.Mutex
	msgs []received
	tls  bool
}

func (r *relay) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	return &relaySession{relay: r, conn: c}, nil
}

type relaySession struct {
	relay *relay
	conn  *gosmtp.Conn
	cur   received
}

func (s *relaySession) AuthMechanisms() []string { return []string{sasl.Plain} }

func (s *relaySession) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, user, pass string) error {
		if user != "relay-user" || pass != "relay-pass" {
			return errors.New("bad credentials")
		}
		s.cur.user, s.cur.pass = user, pass
		return nil
	}), nil
}

func (s *relaySession) Mail(from string, _ *gosmtp.MailOptions) error {
	if s.cur.user == "" {
		return gosmtp.ErrAuthRequired
	}
	s.cur.from = from
	return nil
}

func (s *relaySession) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	s.cur.to = append(s.cur.to, to)
	return nil
}

func (s *relaySession) Data(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.cur.data = data
	s.relay.mu.Lock()
	s.relay.msgs = append(s.relay.msgs, s.cur)
	_, isTLS := s.conn.TLSConnectionState()
	s.relay.tls = s.relay.tls || isTLS
	s.relay.mu.Unlock()
	return nil
}

func (s *relaySession) Reset()        {}
func (s *relaySession) Logout() error { return nil }

// startRelay runs a submission server that offers STARTTLS and AUTH PLAIN, like a provider's port 587.
func startRelay(t *testing.T) (*relay, Config, string) {
	t.Helper()
	cert, pool := selfSigned(t)
	r := &relay{}
	srv := gosmtp.NewServer(r)
	srv.Domain = "relay.example.com"
	srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.AllowInsecureAuth = false
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return r, Config{
		Host:     "relay.example.com",
		Port:     ln.Addr().(*net.TCPAddr).Port,
		Username: "relay-user",
		Password: "relay-pass",
		From:     netmail.Address{Name: "Morgenblau", Address: "feedback@notify.example.com"},
		TLS:      &tls.Config{RootCAs: pool, ServerName: "relay.example.com"},
	}, ln.Addr().String()
}

// senderTo dials the test relay whatever host the config names, so the sender treats it as a remote relay.
func senderTo(cfg Config, addr string) *Sender {
	s := NewSender(cfg)
	s.dial = func(ctx context.Context, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	}
	return s
}

func selfSigned(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "relay.example.com"},
		DNSNames:     []string{"relay.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

func TestSend_DeliversMultipartMessageOverSTARTTLSWithAuth(t *testing.T) {
	r, cfg, addr := startRelay(t)
	png := []byte("\x89PNG\r\n\x1a\nnot-really-a-png")
	msg := Message{
		To:      "owner@example.com",
		Subject: "Feedback from @reader.example: Grüße\r\nBcc: victim@example.com",
		Text:    "The digest skipped Tuesday.\n\nPage: /digest",
		Attachments: []Attachment{
			{Filename: "screen shot \"1\".png", ContentType: "image/png", Data: png},
		},
	}

	if err := senderTo(cfg, addr).Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if len(r.msgs) != 1 {
		t.Fatalf("relay got %d messages, want 1", len(r.msgs))
	}
	got := r.msgs[0]
	if !r.tls {
		t.Error("message was not sent over TLS")
	}
	if got.from != "feedback@notify.example.com" || len(got.to) != 1 || got.to[0] != "owner@example.com" {
		t.Errorf("envelope = %q -> %v, want feedback@notify.example.com -> [owner@example.com]", got.from, got.to)
	}

	parsed, err := netmail.ReadMessage(bytes.NewReader(got.data))
	if err != nil {
		t.Fatalf("parse message: %v", err)
	}
	if bcc := parsed.Header.Get("Bcc"); bcc != "" {
		t.Errorf("subject injected a Bcc header: %q", bcc)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
	if err != nil {
		t.Fatal(err)
	}
	if subject != "Feedback from @reader.example: Grüße Bcc: victim@example.com" {
		t.Errorf("subject = %q", subject)
	}
	from, err := parsed.Header.AddressList("From")
	if err != nil || len(from) != 1 || from[0].Address != "feedback@notify.example.com" || from[0].Name != "Morgenblau" {
		t.Errorf("From = %v (%v)", from, err)
	}
	if parsed.Header.Get("Message-Id") == "" || parsed.Header.Get("Date") == "" {
		t.Error("missing Message-Id or Date header")
	}

	mediaType, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("Content-Type = %q (%v), want multipart/mixed", mediaType, err)
	}
	parts := multipart.NewReader(parsed.Body, params["boundary"])

	text, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if ct := text.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("first part Content-Type = %q, want text/plain", ct)
	}
	body := decodePart(t, text)
	if string(body) != msg.Text {
		t.Errorf("text = %q, want %q", body, msg.Text)
	}

	att, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if att.FileName() != "screen shot \"1\".png" {
		t.Errorf("attachment filename = %q", att.FileName())
	}
	if ct := att.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("attachment Content-Type = %q, want image/png", ct)
	}
	data := decodePart(t, att)
	if !bytes.Equal(data, png) {
		t.Errorf("attachment bytes = %q, want %q", data, png)
	}
	if _, err := parts.NextPart(); err != io.EOF {
		t.Errorf("want exactly two parts, next part err = %v", err)
	}
}

func decodePart(t *testing.T, p *multipart.Part) []byte {
	t.Helper()
	if enc := p.Header.Get("Content-Transfer-Encoding"); enc != "base64" {
		t.Fatalf("Content-Transfer-Encoding = %q, want base64", enc)
	}
	data, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, p))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSend_RefusesARelayWithoutTLS(t *testing.T) {
	r := &relay{}
	srv := gosmtp.NewServer(r)
	srv.AllowInsecureAuth = true
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	cfg := Config{Host: "relay.example.com", Port: ln.Addr().(*net.TCPAddr).Port, Username: "relay-user", Password: "relay-pass", From: netmail.Address{Address: "feedback@notify.example.com"}}
	if err := senderTo(cfg, ln.Addr().String()).Send(context.Background(), Message{To: "owner@example.com", Subject: "Hi", Text: "Hello"}); err == nil {
		t.Fatal("Send succeeded without TLS, want an error")
	}
	if len(r.msgs) != 0 {
		t.Errorf("relay received %d messages over plaintext", len(r.msgs))
	}
}

func TestLoadFeedbackConfig(t *testing.T) {
	full := map[string]string{
		"FEEDBACK_EMAIL":     "owner@example.com",
		"MAIL_FROM":          "Morgenblau <feedback@notify.example.com>",
		"MAIL_SMTP_HOST":     "smtp.example.com",
		"MAIL_SMTP_USERNAME": "relay-user",
		"MAIL_SMTP_PASSWORD": "relay-pass",
	}
	with := func(overrides map[string]string) func(string) string {
		env := map[string]string{}
		for k, v := range full {
			env[k] = v
		}
		for k, v := range overrides {
			env[k] = v
		}
		return func(k string) string { return env[k] }
	}

	cfg, to, err := LoadFeedbackConfig(with(nil))
	if err != nil {
		t.Fatal(err)
	}
	if to != "owner@example.com" || cfg.Host != "smtp.example.com" || cfg.Port != 587 || cfg.From.Address != "feedback@notify.example.com" || cfg.From.Name != "Morgenblau" {
		t.Errorf("cfg = %+v, to = %q", cfg, to)
	}

	cfg, _, err = LoadFeedbackConfig(with(map[string]string{"MAIL_SMTP_PORT": "465"}))
	if err != nil || cfg.Port != 465 {
		t.Errorf("port 465: cfg.Port = %d, err = %v", cfg.Port, err)
	}

	cfg, to, err = LoadFeedbackConfig(func(string) string { return "" })
	if err != nil || to != "" || cfg.Host != "" {
		t.Errorf("unset: cfg = %+v, to = %q, err = %v; want disabled without error", cfg, to, err)
	}

	for name, overrides := range map[string]map[string]string{
		"no host":           {"MAIL_SMTP_HOST": ""},
		"no from":           {"MAIL_FROM": ""},
		"bad recipient":     {"FEEDBACK_EMAIL": "not an address"},
		"bad from":          {"MAIL_FROM": "nope"},
		"bad port":          {"MAIL_SMTP_PORT": "smtp"},
		"user without pass": {"MAIL_SMTP_PASSWORD": ""},
	} {
		if _, _, err := LoadFeedbackConfig(with(overrides)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
