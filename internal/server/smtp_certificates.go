package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/caddyserver/certmagic"
)

var errUnusableSMTPKeypair = errors.New("unusable SMTP certificate/key pair")

type smtpCertificates struct {
	hostname      string
	config        *certmagic.Config
	issuer        *certmagic.ACMEIssuer
	cache         *certmagic.Cache
	current       atomic.Pointer[tls.Certificate]
	ready         chan struct{}
	readyOnce     sync.Once
	retryInterval time.Duration
	checkInterval time.Duration
}

func newSMTPCertificates(hostname string, httpPort int, settings smtpACMEConfig) (*smtpCertificates, error) {
	if httpPort <= 0 || httpPort > 65535 {
		return nil, fmt.Errorf("SMTP ACME requires an explicit backend PORT between 1 and 65535")
	}
	if err := os.MkdirAll(settings.Storage, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(settings.Storage, 0o700); err != nil {
		return nil, err
	}
	var roots *x509.CertPool
	if settings.CARoot != "" {
		pem, err := os.ReadFile(settings.CARoot)
		if err != nil {
			return nil, err
		}
		roots = x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("SMTP_ACME_CA_ROOT contains no certificates")
		}
	}
	m := &smtpCertificates{hostname: hostname, ready: make(chan struct{}), retryInterval: time.Minute, checkInterval: time.Minute}
	// Keep this cache empty: synchronous, owned work avoids CertMagic's detached renewal jobs.
	m.cache = certmagic.NewCache(certmagic.CacheOptions{GetConfigForCert: func(certmagic.Certificate) (*certmagic.Config, error) { return m.config, nil }})
	m.config = certmagic.New(m.cache, certmagic.Config{Storage: &certmagic.FileStorage{Path: settings.Storage}})
	ca := settings.CA
	if ca == "" {
		ca = certmagic.LetsEncryptProductionCA
	}
	m.issuer = certmagic.NewACMEIssuer(m.config, certmagic.ACMEIssuer{
		CA: ca, Email: settings.Email, Agreed: true,
		DisableTLSALPNChallenge: true, AltHTTPPort: httpPort,
		TrustedRoots: roots, CertObtainTimeout: 2 * time.Minute,
	})
	m.config.Issuers = []certmagic.Issuer{m.issuer}
	return m, nil
}

func (m *smtpCertificates) tlsConfig() *tls.Config {
	// Resumed handshakes skip GetCertificate and its SNI/expiry checks.
	return &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: m.getCertificate, SessionTicketsDisabled: true}
}

func (m *smtpCertificates) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if hello.ServerName != "" && !strings.EqualFold(hello.ServerName, m.hostname) {
		return nil, fmt.Errorf("unexpected SMTP server name")
	}
	cert := m.current.Load()
	if cert == nil {
		return nil, fmt.Errorf("SMTP certificate is not ready")
	}
	if err := validateSMTPTLSCertificate(&tls.Config{Certificates: []tls.Certificate{*cert}}, m.hostname, time.Now()); err != nil {
		return nil, err
	}
	return cert, nil
}

func (m *smtpCertificates) load(ctx context.Context) (*tls.Certificate, error) {
	certPEM, err := m.config.Storage.Load(ctx, certmagic.StorageKeys.SiteCert(m.issuer.IssuerKey(), m.hostname))
	if err != nil {
		return nil, err
	}
	keyPEM, err := m.config.Storage.Load(ctx, certmagic.StorageKeys.SitePrivateKey(m.issuer.IssuerKey(), m.hostname))
	if err != nil {
		return nil, err
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUnusableSMTPKeypair, err)
	}
	if cert.Leaf == nil {
		cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return nil, err
		}
	}
	if err := cert.Leaf.VerifyHostname(m.hostname); err != nil {
		return nil, err
	}
	return &cert, nil
}

func (m *smtpCertificates) publish(cert *tls.Certificate) error {
	if err := validateSMTPTLSCertificate(&tls.Config{Certificates: []tls.Certificate{*cert}}, m.hostname, time.Now()); err != nil {
		return err
	}
	previous := m.current.Swap(cert)
	m.readyOnce.Do(func() { close(m.ready) })
	if previous == nil || !previous.Leaf.Equal(cert.Leaf) {
		slog.Info("SMTP certificate ready", "hostname", m.hostname, "expires", cert.Leaf.NotAfter)
	}
	return nil
}

func (m *smtpCertificates) ensure(ctx context.Context) error {
	cert, err := m.load(ctx)
	if errors.Is(err, fs.ErrNotExist) {
		if err := m.config.ObtainCertSync(ctx, m.hostname); err != nil {
			return err
		}
	} else if errors.Is(err, errUnusableSMTPKeypair) {
		// CertMagic writes a rotated key before its certificate; interruption can split the pair.
		if err := m.config.RenewCertSync(ctx, m.hostname, true); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// A valid persisted certificate remains usable while a due renewal is retried.
		_ = m.publish(cert)
		if err := m.config.RenewCertSync(ctx, m.hostname, false); err != nil {
			return err
		}
	}
	cert, err = m.load(ctx)
	if err != nil {
		return err
	}
	return m.publish(cert)
}

func (m *smtpCertificates) run(ctx context.Context, httpReady <-chan struct{}) {
	select {
	case <-ctx.Done():
		return
	case <-httpReady:
	}
	backoff := m.retryInterval
	for ctx.Err() == nil {
		attempt, cancel := context.WithTimeout(ctx, 3*time.Minute)
		err := m.ensure(attempt)
		cancel()
		delay := m.checkInterval
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			delay = backoff
			backoff = min(backoff*2, 6*time.Hour)
			slog.Warn("SMTP certificate maintenance failed; retrying", "hostname", m.hostname, "retry_in", delay, "err", err)
		} else {
			backoff = m.retryInterval
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

type certificateSMTPListener struct {
	net.Listener
	certificates *smtpCertificates
	closed       chan struct{}
	closeOnce    sync.Once
}

func newCertificateSMTPListener(listener net.Listener, certificates *smtpCertificates) net.Listener {
	return &certificateSMTPListener{Listener: listener, certificates: certificates, closed: make(chan struct{})}
}

func (l *certificateSMTPListener) Accept() (net.Conn, error) {
	select {
	case <-l.closed:
		return nil, net.ErrClosed
	case <-l.certificates.ready:
	}
	for {
		if _, err := l.certificates.getCertificate(&tls.ClientHelloInfo{}); err != nil {
			timer := time.NewTimer(time.Second)
			select {
			case <-l.closed:
				timer.Stop()
				return nil, net.ErrClosed
			case <-timer.C:
			}
			continue
		}
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if _, err := l.certificates.getCertificate(&tls.ClientHelloInfo{}); err == nil {
			return conn, nil
		}
		_ = conn.Close()
	}
}

func (l *certificateSMTPListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return l.Listener.Close()
}

func redirectInsecureHTTP(next http.Handler, appHost string, behindFly bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil || behindFly && r.Header.Get("X-Forwarded-Proto") == "https" || r.Method == http.MethodGet && r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		http.Redirect(w, r, "https://"+appHost+r.URL.RequestURI(), http.StatusPermanentRedirect)
	})
}
