package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/caddyserver/certmagic"

	"morgenblau/internal/newsletter"
)

func TestSMTPCertificateSelection(t *testing.T) {
	m := &smtpCertificates{hostname: "mx.example.com"}
	if _, err := m.getCertificate(&tls.ClientHelloInfo{}); err == nil {
		t.Fatal("certificate available before provisioning")
	}
	certPEM, keyPEM := testSMTPCertificate(t, m.hostname, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	m.current.Store(&cert)
	for _, name := range []string{"", "mx.example.com", "MX.EXAMPLE.COM"} {
		if got, err := m.getCertificate(&tls.ClientHelloInfo{ServerName: name}); err != nil || got != &cert {
			t.Fatalf("SNI %q: %p %v", name, got, err)
		}
	}
	if _, err := m.getCertificate(&tls.ClientHelloInfo{ServerName: "other.example.com"}); err == nil {
		t.Fatal("unexpected SNI accepted")
	}
	certPEM, keyPEM = testSMTPCertificate(t, m.hostname, time.Now().Add(-time.Hour), time.Now())
	expired, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	m.current.Store(&expired)
	if _, err := m.getCertificate(&tls.ClientHelloInfo{}); err == nil {
		t.Fatal("expired certificate served")
	}
}

func TestSMTPACMERequiresExplicitBackendHTTPPort(t *testing.T) {
	m, err := newSMTPCertificates("mx.example.com", 0, smtpACMEConfig{Email: "operator@example.com", Storage: t.TempDir()})
	if m != nil {
		defer m.cache.Stop()
	}
	if err == nil {
		t.Fatal("automatic SMTP TLS accepted an unknown backend HTTP port")
	}
}

func TestSMTPCertificateLoadWithoutTLSParsedLeaf(t *testing.T) {
	t.Setenv("GODEBUG", "x509keypairleaf=0")
	m, err := newSMTPCertificates("mx.example.com", 18080, smtpACMEConfig{Email: "operator@example.com", Storage: t.TempDir(), CA: "http://127.0.0.1:1/directory"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.cache.Stop()
	certPEM, keyPEM := testSMTPCertificate(t, m.hostname, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	for key, data := range map[string][]byte{
		certmagic.StorageKeys.SiteCert(m.issuer.IssuerKey(), m.hostname):       certPEM,
		certmagic.StorageKeys.SitePrivateKey(m.issuer.IssuerKey(), m.hostname): keyPEM,
	} {
		if err := m.config.Storage.Store(context.Background(), key, data); err != nil {
			t.Fatal(err)
		}
	}
	cert, err := m.load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cert.Leaf == nil {
		t.Fatal("loaded certificate has no parsed leaf")
	}
	if err := m.publish(cert); err != nil {
		t.Fatal(err)
	}
}

func TestSMTPSTARTTLSNoSNIAndReplacement(t *testing.T) {
	m := &smtpCertificates{hostname: "mx.example.com"}
	server, addr, done, err := startNewsletterSMTP(&newsletter.Service{}, newsletterRuntimeConfig{
		Domain: "news.example.com", Hostname: m.hostname, ListenAddr: "127.0.0.1:0", TLSConfig: m.tlsConfig(), MaxConnections: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(); <-done })
	for i := range 2 {
		certPEM, keyPEM := testSMTPCertificate(t, m.hostname, time.Now().Add(-time.Hour), time.Now().Add(time.Duration(i+1)*time.Hour))
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			t.Fatal(err)
		}
		m.current.Store(&cert)
		roots := x509.NewCertPool()
		roots.AppendCertsFromPEM(certPEM)
		client, err := smtp.Dial(addr.String())
		if err != nil {
			t.Fatal(err)
		}
		// An IP ServerName omits SNI; verify the intended DNS identity independently.
		err = client.StartTLS(&tls.Config{ServerName: "127.0.0.1", InsecureSkipVerify: true, VerifyConnection: func(cs tls.ConnectionState) error {
			_, err := cs.PeerCertificates[0].Verify(x509.VerifyOptions{DNSName: m.hostname, Roots: roots})
			return err
		}})
		_ = client.Close()
		if err != nil {
			t.Fatalf("replacement %d: %v", i, err)
		}
	}
}

type smtpTicketCache struct{ tls.ClientSessionCache }

func (c smtpTicketCache) Get(string) (*tls.ClientSessionState, bool) {
	return c.ClientSessionCache.Get("smtp")
}

func (c smtpTicketCache) Put(_ string, state *tls.ClientSessionState) {
	c.ClientSessionCache.Put("smtp", state)
}

func TestSMTPSTARTTLSTicketsCannotBypassPolicy(t *testing.T) {
	for _, policy := range []string{"unexpected SNI", "expired current certificate"} {
		t.Run(policy, func(t *testing.T) {
			m := &smtpCertificates{hostname: "mx.example.com", ready: make(chan struct{})}
			certPEM, keyPEM := testSMTPCertificate(t, m.hostname, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
			cert, err := tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.publish(&cert); err != nil {
				t.Fatal(err)
			}
			start := func(config *tls.Config) string {
				config.SetSessionTicketKeys([][32]byte{{1, 2, 3}})
				server, addr, done, err := startNewsletterSMTP(&newsletter.Service{}, newsletterRuntimeConfig{
					Domain: "news.example.com", Hostname: m.hostname, ListenAddr: "127.0.0.1:0", TLSConfig: config, certificates: m,
				})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = server.Close(); <-done })
				return addr.String()
			}
			legacy := m.tlsConfig()
			legacy.SessionTicketsDisabled = false
			legacyAddr := start(legacy)
			addr := start(m.tlsConfig())
			cache := smtpTicketCache{tls.NewLRUClientSessionCache(1)}
			// A malicious peer may reuse a ticket across SNI names without verifying the server.
			config := &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, ServerName: m.hostname, InsecureSkipVerify: true, ClientSessionCache: cache}
			dial := func(address string) *smtp.Client {
				conn, err := net.DialTimeout("tcp", address, time.Second)
				if err != nil {
					t.Fatal(err)
				}
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				client, err := smtp.NewClient(conn, m.hostname)
				if err != nil {
					_ = conn.Close()
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = client.Close() })
				return client
			}
			for i := range 2 {
				client := dial(legacyAddr)
				if err := client.StartTLS(config); err != nil {
					t.Fatal(err)
				}
				if err := client.Noop(); err != nil {
					t.Fatal(err)
				}
				state, _ := client.TLSConnectionState()
				if state.DidResume != (i == 1) {
					t.Fatalf("legacy control handshake %d resumed=%v", i, state.DidResume)
				}
				_ = client.Close()
			}
			if _, ok := cache.Get("smtp"); !ok {
				t.Fatal("legacy SMTP handshake did not yield a TLS 1.3 ticket")
			}
			client := dial(addr)
			if policy == "unexpected SNI" {
				config = config.Clone()
				config.ServerName = "other.example.com"
			} else {
				certPEM, keyPEM = testSMTPCertificate(t, m.hostname, time.Now().Add(-time.Hour), time.Now().Add(-time.Minute))
				expired, err := tls.X509KeyPair(certPEM, keyPEM)
				if err != nil {
					t.Fatal(err)
				}
				m.current.Store(&expired)
			}
			if err := client.StartTLS(config); err == nil {
				state, _ := client.TLSConnectionState()
				t.Fatalf("STARTTLS bypassed %s with legacy ticket: resumed=%v", policy, state.DidResume)
			} else {
				var remote *net.OpError
				if !errors.As(err, &remote) || remote.Op != "remote error" {
					t.Fatalf("STARTTLS failed without a peer TLS alert: %v", err)
				}
			}
		})
	}
}

func TestSMTPHTTPRedirectBoundary(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, tc := range []struct {
		name, path, proto string
		fly               bool
		status            int
	}{
		{"http redirect", "/a%2Fb?x=1", "", true, 308},
		{"forwarded https", "/api/digest", "https", true, 204},
		{"untrusted header", "/api/digest", "https", false, 308},
		{"health routing", "/api/health", "", true, 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://attacker.example.com"+tc.path, nil)
			req.Header.Set("X-Forwarded-Proto", tc.proto)
			rr := httptest.NewRecorder()
			redirectInsecureHTTP(next, "app.example.com", tc.fly).ServeHTTP(rr, req)
			if rr.Code != tc.status {
				t.Fatalf("status = %d, want %d", rr.Code, tc.status)
			}
			if tc.status == 308 && rr.Header().Get("Location") != "https://app.example.com"+tc.path {
				t.Fatalf("unsafe or corrupted redirect: %s", rr.Header().Get("Location"))
			}
		})
	}
}

func TestSMTPCertificateWorkerCancellationAndHTTPReadiness(t *testing.T) {
	caCalls := make(chan struct{}, 10)
	ca := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { caCalls <- struct{}{}; w.WriteHeader(503) }))
	defer ca.Close()
	m, err := newSMTPCertificates("mx.example.com", 18080, smtpACMEConfig{Email: "operator@example.com", Storage: filepath.Join(t.TempDir(), "certs"), CA: ca.URL})
	if err != nil {
		t.Fatal(err)
	}
	m.retryInterval = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	httpReady := make(chan struct{})
	done := make(chan struct{})
	go func() { defer close(done); m.run(ctx, httpReady) }()
	select {
	case <-caCalls:
		t.Fatal("issuance started before HTTP")
	case <-time.After(50 * time.Millisecond):
	}
	close(httpReady)
	select {
	case <-caCalls:
	case <-time.After(3 * time.Second):
		t.Fatal("no proactive issuance")
	}
	select {
	case <-caCalls:
	case <-time.After(3 * time.Second):
		t.Fatal("initial failure was not retried")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("issuance not joined on cancellation")
	}
	m.cache.Stop()
}

func TestSMTPCertificateGatesGreetingAndClosesWhileWaiting(t *testing.T) {
	for _, provision := range []bool{false, true} {
		m := &smtpCertificates{hostname: "mx.example.com", ready: make(chan struct{})}
		server, addr, done, err := startNewsletterSMTP(&newsletter.Service{}, newsletterRuntimeConfig{
			Domain: "news.example.com", Hostname: m.hostname, ListenAddr: "127.0.0.1:0", TLSConfig: m.tlsConfig(), certificates: m,
		})
		if err != nil {
			t.Fatal(err)
		}
		conn, err := net.Dial("tcp", addr.String())
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		buffer := make([]byte, 64)
		if _, err := conn.Read(buffer); err == nil {
			t.Fatal("SMTP greeting before certificate readiness")
		}
		if provision {
			certPEM, keyPEM := testSMTPCertificate(t, m.hostname, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
			cert, err := tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.publish(&cert); err != nil {
				t.Fatal(err)
			}
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			if n, err := conn.Read(buffer); err != nil || string(buffer[:4]) != "220 " {
				t.Fatalf("greeting = %q, %v", buffer[:n], err)
			}
		}
		_ = conn.Close()
		_ = server.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("SMTP shutdown stuck on certificate readiness")
		}
	}
}

func newSMTPACMETestManager(t *testing.T, profile string) (*smtpCertificates, smtpACMEConfig) {
	t.Helper()
	caURL := os.Getenv("MORGENBLAU_TEST_ACME_CA")
	if caURL == "" {
		t.Skip("requires disposable Pebble CA via MORGENBLAU_TEST_ACME_*; see DEPLOY.md")
	}
	ca, err := url.Parse(caURL)
	if err != nil || ca.Hostname() != "localhost" && ca.Hostname() != "127.0.0.1" {
		t.Fatal("ACME integration test requires a loopback CA")
	}
	port, err := strconv.Atoi(os.Getenv("MORGENBLAU_TEST_ACME_HTTP_PORT"))
	if err != nil {
		t.Fatal(err)
	}
	settings := smtpACMEConfig{Email: "operator@example.com", Storage: filepath.Join(t.TempDir(), "certs"), CA: caURL, CARoot: os.Getenv("MORGENBLAU_TEST_ACME_ROOT")}
	m, err := newSMTPCertificates("localhost", port, settings)
	if err != nil {
		t.Fatal(err)
	}
	m.issuer.Profile = profile
	app := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	httpServer := &http.Server{Handler: m.issuer.HTTPChallengeHandler(redirectInsecureHTTP(app, "app.example.com", true))}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	httpDone := make(chan error, 1)
	go func() { httpDone <- httpServer.Serve(listener) }()
	t.Cleanup(func() {
		m.cache.Stop()
		_ = httpServer.Close()
		<-httpDone
	})
	return m, settings
}

func TestSMTPACMEIntegration(t *testing.T) {
	m, settings := newSMTPACMETestManager(t, "default")
	m.checkInterval = 100 * time.Millisecond
	m.retryInterval = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	workerDone := make(chan struct{})
	httpReady := make(chan struct{})
	go func() { defer close(workerDone); m.run(ctx, httpReady) }()
	close(httpReady)
	smtpServer, addr, smtpDone, err := startNewsletterSMTP(&newsletter.Service{}, newsletterRuntimeConfig{
		Domain: "news.example.com", Hostname: "localhost", ListenAddr: "127.0.0.1:0", TLSConfig: m.tlsConfig(), certificates: m,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		<-workerDone
		_ = smtpServer.Close()
		<-smtpDone
	})
	select {
	case <-m.ready:
	case <-time.After(30 * time.Second):
		t.Fatal("HTTP-01 issuance did not finish")
	}
	initial := m.current.Load().Leaf.SerialNumber.String()
	t.Logf("issued serial=%s", initial)
	deadline := time.Now().Add(30 * time.Second)
	for m.current.Load().Leaf.SerialNumber.String() == initial && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if m.current.Load().Leaf.SerialNumber.String() == initial {
		t.Fatal("automatic renewal did not replace certificate")
	}
	cancel()
	<-workerDone
	t.Logf("renewed serial=%s", m.current.Load().Leaf.SerialNumber.String())
	client, err := smtp.Dial(addr.String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	rootPEM, err := os.ReadFile(os.Getenv("MORGENBLAU_TEST_ACME_CERT_ROOT"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(rootPEM) {
		t.Fatal("test CA certificate root is invalid")
	}
	if err := client.StartTLS(&tls.Config{ServerName: "localhost", RootCAs: roots}); err != nil {
		t.Fatal(err)
	}
	state, ok := client.TLSConnectionState()
	if !ok || state.PeerCertificates[0].SerialNumber.String() != m.current.Load().Leaf.SerialNumber.String() {
		t.Fatal("SMTP did not serve renewed certificate")
	}
	// A new manager must load the same private persistent assets, not issue again.
	reloaded, err := newSMTPCertificates("localhost", m.issuer.AltHTTPPort, settings)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.cache.Stop()
	cert, err := reloaded.load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.publish(cert); err != nil {
		t.Fatal(err)
	}
	if cert.Leaf.SerialNumber.String() != state.PeerCertificates[0].SerialNumber.String() {
		t.Fatal("persistent reload changed certificate")
	}
	info, err := os.Stat(settings.Storage)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("storage permissions: %v %v", info, err)
	}
}

func TestSMTPACMERecoveryAndNotDue(t *testing.T) {
	m, _ := newSMTPACMETestManager(t, "normal")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	issuances := 0
	m.config.OnEvent = func(_ context.Context, event string, _ map[string]any) error {
		if event == "cert_obtaining" {
			issuances++
		}
		return nil
	}
	if err := m.ensure(ctx); err != nil {
		t.Fatal(err)
	}
	initial := m.current.Load().Leaf
	if time.Until(initial.NotAfter) < 30*24*time.Hour || issuances != 1 {
		t.Fatalf("expected one normal-lifetime issuance: expires=%s issuances=%d", initial.NotAfter, issuances)
	}
	for range 3 {
		if err := m.ensure(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if issuances != 1 || m.current.Load().Leaf.SerialNumber.Cmp(initial.SerialNumber) != 0 {
		t.Fatalf("not-yet-due maintenance issued again: issuances=%d", issuances)
	}
	t.Logf("normal-lifetime serial=%s unchanged after 3 maintenance checks; issuances=%d", initial.SerialNumber, issuances)
	keys, err := m.config.Storage.List(ctx, "acme", true)
	if err != nil {
		t.Fatal(err)
	}
	account := make(map[string][]byte)
	for _, key := range keys {
		info, err := m.config.Storage.Stat(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if info.IsTerminal && strings.Contains(key, "/users/") {
			account[key], err = m.config.Storage.Load(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(account) == 0 {
		t.Fatal("no persisted ACME account assets")
	}
	_, differentKey := testSMTPCertificate(t, m.hostname, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err := m.config.Storage.Store(ctx, certmagic.StorageKeys.SitePrivateKey(m.issuer.IssuerKey(), m.hostname), differentKey); err != nil {
		t.Fatal(err)
	}
	if _, err := m.load(ctx); err == nil {
		t.Fatal("mismatched keypair fixture was usable")
	} else {
		t.Logf("interrupted-renewal fixture reproduced: %v", err)
	}
	if err := m.ensure(ctx); err != nil {
		t.Fatalf("unusable stored keypair did not recover: %v", err)
	}
	if issuances != 2 || m.current.Load().Leaf.SerialNumber.Cmp(initial.SerialNumber) == 0 {
		t.Fatalf("keypair repair did not force exactly one renewal: issuances=%d", issuances)
	}
	for key, before := range account {
		after, err := m.config.Storage.Load(ctx, key)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("repair changed or deleted ACME account asset %s: %v", key, err)
		}
	}
	if err := m.ensure(ctx); err != nil || issuances != 2 {
		t.Fatalf("repaired keypair renewed again: issuances=%d error=%v", issuances, err)
	}
	t.Logf("repaired serial=%s; account assets unchanged; issuances=%d", m.current.Load().Leaf.SerialNumber, issuances)
}

type smtpReadErrorStorage struct {
	certmagic.Storage
	key string
	err error
}

func (s smtpReadErrorStorage) Load(ctx context.Context, key string) ([]byte, error) {
	if key == s.key {
		return nil, s.err
	}
	return s.Storage.Load(ctx, key)
}

func TestSMTPCertificateStorageErrorsDoNotIssue(t *testing.T) {
	for _, storageErr := range []error{fs.ErrPermission, syscall.EIO} {
		for _, privateKey := range []bool{false, true} {
			m, err := newSMTPCertificates("mx.example.com", 18080, smtpACMEConfig{Email: "operator@example.com", Storage: t.TempDir(), CA: "http://127.0.0.1:1/directory"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(m.cache.Stop)
			certPEM, keyPEM := testSMTPCertificate(t, m.hostname, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
			certKey := certmagic.StorageKeys.SiteCert(m.issuer.IssuerKey(), m.hostname)
			keyKey := certmagic.StorageKeys.SitePrivateKey(m.issuer.IssuerKey(), m.hostname)
			for key, data := range map[string][]byte{certKey: certPEM, keyKey: keyPEM} {
				if err := m.config.Storage.Store(context.Background(), key, data); err != nil {
					t.Fatal(err)
				}
			}
			failedKey := certKey
			if privateKey {
				failedKey = keyKey
			}
			m.config.Storage = smtpReadErrorStorage{Storage: m.config.Storage, key: failedKey, err: storageErr}
			if err := m.ensure(context.Background()); !errors.Is(err, storageErr) {
				t.Fatalf("privateKey=%v: error=%v, want storage error %v without issuance", privateKey, err, storageErr)
			}
		}
	}
}
