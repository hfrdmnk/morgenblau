package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"morgenblau/internal/newsletter"
)

func TestLoadNewsletterRuntimeConfig_DisabledWhenUnset(t *testing.T) {
	clearNewsletterEnv(t)

	cfg, err := loadNewsletterRuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.smtpEnabled() {
		t.Fatal("SMTP enabled with empty configuration")
	}
	if cfg.Domain != "" {
		t.Errorf("domain = %q", cfg.Domain)
	}
}

func TestLoadNewsletterRuntimeConfig_RequiresDomainForSMTP(t *testing.T) {
	clearNewsletterEnv(t)
	t.Setenv("SMTP_LISTEN_ADDR", ":2525")

	_, err := loadNewsletterRuntimeConfig()
	if err == nil || !strings.Contains(err.Error(), "NEWSLETTER_DOMAIN") {
		t.Fatalf("error = %v, want NEWSLETTER_DOMAIN validation", err)
	}
}

func TestLoadNewsletterRuntimeConfig_EnablesSMTPExplicitly(t *testing.T) {
	clearNewsletterEnv(t)
	t.Setenv("NEWSLETTER_DOMAIN", "inbound.example.test")
	t.Setenv("SMTP_LISTEN_ADDR", ":2525")

	cfg, err := loadNewsletterRuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.smtpEnabled() {
		t.Fatal("SMTP disabled with domain and listen address configured")
	}
	if cfg.MaxMessageBytes != 10<<20 {
		t.Errorf("max message bytes = %d", cfg.MaxMessageBytes)
	}
	if cfg.MaxConnections != 8 {
		t.Errorf("max connections = %d", cfg.MaxConnections)
	}
	if cfg.Hostname != "inbound.example.test" {
		t.Errorf("SMTP hostname = %q", cfg.Hostname)
	}
}

func TestLoadNewsletterRuntimeConfig_RejectsHalfTLSKeypair(t *testing.T) {
	clearNewsletterEnv(t)
	t.Setenv("NEWSLETTER_DOMAIN", "inbound.example.test")
	t.Setenv("SMTP_LISTEN_ADDR", ":2525")
	t.Setenv("SMTP_TLS_CERT_B64", "Y2VydA==")

	_, err := loadNewsletterRuntimeConfig()
	if err == nil || !strings.Contains(err.Error(), "SMTP_TLS_CERT_B64 and SMTP_TLS_KEY_B64") {
		t.Fatalf("error = %v, want paired TLS validation", err)
	}
}

func TestLoadNewsletterRuntimeConfig_ProductionSMTPRequiresTLS(t *testing.T) {
	clearNewsletterEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("NEWSLETTER_DOMAIN", "inbound.example.test")
	t.Setenv("SMTP_LISTEN_ADDR", ":2525")

	_, err := loadNewsletterRuntimeConfig()
	if err == nil || !strings.Contains(err.Error(), "SMTP TLS") {
		t.Fatalf("error = %v, want production TLS requirement", err)
	}
}

func TestLoadNewsletterRuntimeConfig_ProductionSMTPValidatesCertificate(t *testing.T) {
	tests := []struct {
		name       string
		certDomain string
		notBefore  time.Time
		notAfter   time.Time
	}{
		{
			name: "wrong domain", certDomain: "other.example.test",
			notBefore: time.Now().Add(-time.Hour), notAfter: time.Now().Add(time.Hour),
		},
		{
			name: "expired", certDomain: "inbound.example.test",
			notBefore: time.Now().Add(-2 * time.Hour), notAfter: time.Now().Add(-time.Hour),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearNewsletterEnv(t)
			t.Setenv("APP_ENV", "production")
			t.Setenv("NEWSLETTER_DOMAIN", "inbound.example.test")
			t.Setenv("SMTP_LISTEN_ADDR", ":2525")
			cert, key := testSMTPCertificate(t, tt.certDomain, tt.notBefore, tt.notAfter)
			t.Setenv("SMTP_TLS_CERT_B64", base64.StdEncoding.EncodeToString(cert))
			t.Setenv("SMTP_TLS_KEY_B64", base64.StdEncoding.EncodeToString(key))

			_, err := loadNewsletterRuntimeConfig()
			if err == nil || !strings.Contains(err.Error(), "SMTP TLS certificate") {
				t.Fatalf("error = %v, want certificate validation error", err)
			}
		})
	}
}

func TestLoadNewsletterRuntimeConfig_ProductionSMTPUsesSeparateMXHostname(t *testing.T) {
	clearNewsletterEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("NEWSLETTER_DOMAIN", "inbox.example.test")
	t.Setenv("SMTP_HOSTNAME", "mail.example.test")
	t.Setenv("SMTP_LISTEN_ADDR", ":2525")
	cert, key := testSMTPCertificate(t, "mail.example.test", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	t.Setenv("SMTP_TLS_CERT_B64", base64.StdEncoding.EncodeToString(cert))
	t.Setenv("SMTP_TLS_KEY_B64", base64.StdEncoding.EncodeToString(key))

	cfg, err := loadNewsletterRuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Domain != "inbox.example.test" || cfg.Hostname != "mail.example.test" {
		t.Fatalf("domains = recipient %q, SMTP %q", cfg.Domain, cfg.Hostname)
	}
}

func TestNewsletterServiceDomainRequiresEnabledReceiver(t *testing.T) {
	if got := effectiveNewsletterDomain(newsletterRuntimeConfig{Domain: "inbound.example.test"}); got != "" {
		t.Errorf("disabled receiver domain = %q, want empty", got)
	}
	if got := effectiveNewsletterDomain(newsletterRuntimeConfig{Domain: "inbound.example.test", ListenAddr: ":2525"}); got != "inbound.example.test" {
		t.Errorf("enabled receiver domain = %q", got)
	}
}

func TestStartNewsletterSMTPEnforcesConnectionLimitBeforeGreeting(t *testing.T) {
	server, addr, done, err := startNewsletterSMTP(&newsletter.Service{}, newsletterRuntimeConfig{
		Domain: "inbound.example.test", Hostname: "mail.example.test", ListenAddr: "127.0.0.1:0", MaxMessageBytes: 1 << 20, MaxConnections: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = server.Close()
		<-done
	})

	first, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	if err := second.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 64)
	if _, err := second.Read(buffer); err == nil {
		t.Fatal("second connection received a greeting while the first held the only slot")
	} else if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
		t.Fatalf("second read error = %v, want timeout", err)
	}
	_ = first.Close()
	if err := second.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if n, err := second.Read(buffer); err != nil || !strings.HasPrefix(string(buffer[:n]), "220 ") {
		t.Fatalf("second greeting = %q, %v", string(buffer[:n]), err)
	}
}

func clearNewsletterEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "local")
	for _, key := range []string{
		"NEWSLETTER_DOMAIN",
		"SMTP_HOSTNAME",
		"SMTP_LISTEN_ADDR",
		"SMTP_TLS_CERT_FILE",
		"SMTP_TLS_KEY_FILE",
		"SMTP_TLS_CERT_B64",
		"SMTP_TLS_KEY_B64",
		"SMTP_MAX_MESSAGE_BYTES",
		"SMTP_MAX_CONNECTIONS",
	} {
		t.Setenv(key, "")
	}
}

func testSMTPCertificate(t *testing.T, domain string, notBefore, notAfter time.Time) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain},
		NotBefore: notBefore, NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}
