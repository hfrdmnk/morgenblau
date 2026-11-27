package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net"
	netmail "net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"
)

const implicitTLSPort = 465

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     netmail.Address
	TLS      *tls.Config
}

type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

type Message struct {
	To          string
	Subject     string
	Text        string
	Attachments []Attachment
}

type Sender struct {
	cfg  Config
	dial func(ctx context.Context, addr string) (net.Conn, error)
}

func NewSender(cfg Config) *Sender {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	return &Sender{cfg: cfg, dial: func(ctx context.Context, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", addr)
	}}
}

func LoadFeedbackConfig(getenv func(string) string) (Config, string, error) {
	env := func(name string) string { return strings.TrimSpace(getenv(name)) }
	to := env("FEEDBACK_EMAIL")
	if to == "" {
		return Config{}, "", nil
	}
	if _, err := netmail.ParseAddress(to); err != nil {
		return Config{}, "", fmt.Errorf("FEEDBACK_EMAIL: %w", err)
	}
	cfg := Config{Host: env("MAIL_SMTP_HOST"), Port: 587, Username: env("MAIL_SMTP_USERNAME"), Password: getenv("MAIL_SMTP_PASSWORD")}
	if cfg.Host == "" {
		return Config{}, "", errors.New("MAIL_SMTP_HOST is required when FEEDBACK_EMAIL is set")
	}
	if raw := env("MAIL_SMTP_PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port <= 0 || port > 65535 {
			return Config{}, "", fmt.Errorf("MAIL_SMTP_PORT: %q is not a port", raw)
		}
		cfg.Port = port
	}
	if (cfg.Username == "") != (cfg.Password == "") {
		return Config{}, "", errors.New("MAIL_SMTP_USERNAME and MAIL_SMTP_PASSWORD must be set together")
	}
	from := env("MAIL_FROM")
	if from == "" {
		return Config{}, "", errors.New("MAIL_FROM is required when FEEDBACK_EMAIL is set")
	}
	addr, err := netmail.ParseAddress(from)
	if err != nil {
		return Config{}, "", fmt.Errorf("MAIL_FROM: %w", err)
	}
	cfg.From = *addr
	return cfg, to, nil
}

func (s *Sender) Send(ctx context.Context, m Message) error {
	raw, err := compose(s.cfg.From, m)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	conn, err := s.dial(ctx, addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	tlsConfig := s.cfg.TLS
	if tlsConfig == nil {
		tlsConfig = &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}
	}

	var c *gosmtp.Client
	switch {
	case s.cfg.Port == implicitTLSPort:
		c = gosmtp.NewClient(tls.Client(conn, tlsConfig))
	case isLoopback(s.cfg.Host):
		c = gosmtp.NewClient(conn)
	default:
		if c, err = gosmtp.NewClientStartTLS(conn, tlsConfig); err != nil {
			return fmt.Errorf("starttls with %s: %w", addr, err)
		}
	}
	defer func() { _ = c.Close() }()

	if s.cfg.Username != "" {
		if err := c.Auth(sasl.NewPlainClient("", s.cfg.Username, s.cfg.Password)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.SendMail(s.cfg.From.Address, []string{m.To}, bytes.NewReader(raw)); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return c.Quit()
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func compose(from netmail.Address, m Message) ([]byte, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	domain := from.Address[strings.LastIndex(from.Address, "@")+1:]

	header := []string{
		"From: " + from.String(),
		"To: " + (&netmail.Address{Address: m.To}).String(),
		"Subject: " + mime.QEncoding.Encode("utf-8", oneLine(m.Subject)),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"Message-ID: <" + hex.EncodeToString(id) + "@" + domain + ">",
		"MIME-Version: 1.0",
		"Content-Type: " + mime.FormatMediaType("multipart/mixed", map[string]string{"boundary": w.Boundary()}),
	}
	buf.WriteString(strings.Join(header, "\r\n") + "\r\n\r\n")

	text, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"text/plain; charset=utf-8"},
		"Content-Transfer-Encoding": {"base64"},
	})
	if err != nil {
		return nil, err
	}
	writeBase64(text, []byte(m.Text))

	for _, a := range m.Attachments {
		part, err := w.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {a.ContentType},
			"Content-Transfer-Encoding": {"base64"},
			"Content-Disposition":       {mime.FormatMediaType("attachment", map[string]string{"filename": oneLine(a.Filename)})},
		})
		if err != nil {
			return nil, err
		}
		writeBase64(part, a.Data)
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// writeBase64 wraps at 76 columns, the line limit RFC 2045 sets for base64 bodies.
// RFC 2045 limits base64 body lines to 76 characters.
// https://www.rfc-editor.org/rfc/rfc2045#section-6.8
func writeBase64(w interface{ Write([]byte) (int, error) }, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 76 {
		_, _ = w.Write([]byte(encoded[:76] + "\r\n"))
		encoded = encoded[76:]
	}
	_, _ = w.Write([]byte(encoded + "\r\n"))
}
