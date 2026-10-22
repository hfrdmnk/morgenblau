package newsletter

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-smtp"
)

const (
	defaultMaxMessageBytes = int64(10 << 20)
	defaultMaxConnections  = 8
	maxSMTPRecipients      = 100
	defaultSMTPReadTimeout = 30 * time.Second
)

type SMTPConfig struct {
	Addr            string
	Domain          string
	TLSConfig       *tls.Config
	MaxMessageBytes int64
	MaxConnections  int
}

func NewSMTPServer(service *Service, cfg SMTPConfig) *smtp.Server {
	if cfg.Addr == "" {
		cfg.Addr = ":25"
	}
	if cfg.Domain == "" {
		cfg.Domain = service.domain
	}
	if cfg.MaxMessageBytes <= 0 {
		cfg.MaxMessageBytes = defaultMaxMessageBytes
	}
	if cfg.MaxConnections <= 0 {
		cfg.MaxConnections = defaultMaxConnections
	}
	backend := &smtpBackend{
		service: service,
		maxSize: cfg.MaxMessageBytes,
		slots:   make(chan struct{}, cfg.MaxConnections),
	}
	server := smtp.NewServer(backend)
	server.Addr = cfg.Addr
	server.Domain = cfg.Domain
	server.TLSConfig = cfg.TLSConfig
	server.MaxMessageBytes = cfg.MaxMessageBytes
	server.MaxRecipients = maxSMTPRecipients
	server.ReadTimeout = defaultSMTPReadTimeout
	server.WriteTimeout = 30 * time.Second
	return server
}

func LimitSMTPListener(listener net.Listener, maxConnections int) net.Listener {
	if maxConnections <= 0 {
		maxConnections = defaultMaxConnections
	}
	return &limitedSMTPListener{
		Listener: listener,
		slots:    make(chan struct{}, maxConnections),
		closed:   make(chan struct{}),
	}
}

type limitedSMTPListener struct {
	net.Listener
	slots     chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
}

func (l *limitedSMTPListener) Accept() (net.Conn, error) {
	select {
	case l.slots <- struct{}{}:
	case <-l.closed:
		return nil, net.ErrClosed
	}
	connection, err := l.Listener.Accept()
	if err != nil {
		<-l.slots
		return nil, err
	}
	return &limitedSMTPConn{Conn: connection, release: func() { <-l.slots }}, nil
}

func (l *limitedSMTPListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return l.Listener.Close()
}

type limitedSMTPConn struct {
	net.Conn
	releaseOnce sync.Once
	release     func()
}

func (c *limitedSMTPConn) Close() error {
	err := c.Conn.Close()
	c.releaseOnce.Do(c.release)
	return err
}

type smtpBackend struct {
	service *Service
	maxSize int64
	slots   chan struct{}
}

func (b *smtpBackend) NewSession(*smtp.Conn) (smtp.Session, error) {
	select {
	case b.slots <- struct{}{}:
		return &smtpSession{backend: b}, nil
	default:
		return nil, &smtp.SMTPError{Code: 421, EnhancedCode: smtp.EnhancedCode{4, 3, 2}, Message: "server is busy"}
	}
}

type smtpSession struct {
	backend    *smtpBackend
	envelope   string
	recipients []deliveryRecipient
	released   sync.Once
}

func (s *smtpSession) Mail(from string, _ *smtp.MailOptions) error {
	s.envelope = strings.TrimSpace(from)
	s.recipients = nil
	return nil
}

func (s *smtpSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	recipient, err := s.backend.service.resolveRecipient(context.Background(), to)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalid) {
		return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "recipient not found"}
	}
	if err != nil {
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "temporary storage failure"}
	}
	s.recipients = append(s.recipients, recipient)
	return nil
}

func (s *smtpSession) Data(reader io.Reader) error {
	raw, err := readBounded(reader, s.backend.maxSize)
	if err != nil {
		return &smtp.SMTPError{Code: 552, EnhancedCode: smtp.EnhancedCode{5, 3, 4}, Message: "message exceeds fixed limit"}
	}
	if err := s.backend.service.acceptReceipts(context.Background(), s.envelope, s.recipients, raw); err != nil {
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "temporary storage failure"}
	}
	return nil
}

func (s *smtpSession) Reset() {
	s.envelope = ""
	s.recipients = nil
}

func (s *smtpSession) Logout() error {
	s.released.Do(func() { <-s.backend.slots })
	return nil
}
