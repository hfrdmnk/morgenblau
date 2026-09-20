package server

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSMTPMaxMessageBytes = 10 << 20
	defaultSMTPMaxConnections  = 8
)

type newsletterRuntimeConfig struct {
	Domain          string
	Hostname        string
	ListenAddr      string
	TLSConfig       *tls.Config
	MaxMessageBytes int64
	MaxConnections  int
}

func (c newsletterRuntimeConfig) smtpEnabled() bool {
	return c.Domain != "" && c.ListenAddr != ""
}

func effectiveNewsletterDomain(c newsletterRuntimeConfig) string {
	if !c.smtpEnabled() {
		return ""
	}
	return c.Domain
}

func loadNewsletterRuntimeConfig() (newsletterRuntimeConfig, error) {
	cfg := newsletterRuntimeConfig{
		Domain:          strings.TrimSpace(os.Getenv("NEWSLETTER_DOMAIN")),
		Hostname:        strings.TrimSpace(os.Getenv("SMTP_HOSTNAME")),
		ListenAddr:      strings.TrimSpace(os.Getenv("SMTP_LISTEN_ADDR")),
		MaxMessageBytes: defaultSMTPMaxMessageBytes,
		MaxConnections:  defaultSMTPMaxConnections,
	}
	if cfg.ListenAddr != "" && cfg.Domain == "" {
		return newsletterRuntimeConfig{}, fmt.Errorf("NEWSLETTER_DOMAIN is required when SMTP_LISTEN_ADDR is set")
	}
	if cfg.Hostname == "" {
		cfg.Hostname = cfg.Domain
	}

	var err error
	if cfg.MaxMessageBytes, err = positiveInt64Env("SMTP_MAX_MESSAGE_BYTES", cfg.MaxMessageBytes); err != nil {
		return newsletterRuntimeConfig{}, err
	}
	if cfg.MaxConnections, err = positiveIntEnv("SMTP_MAX_CONNECTIONS", cfg.MaxConnections); err != nil {
		return newsletterRuntimeConfig{}, err
	}
	if cfg.TLSConfig, err = loadSMTPTLSConfig(); err != nil {
		return newsletterRuntimeConfig{}, err
	}
	if cfg.smtpEnabled() && os.Getenv("APP_ENV") != "local" {
		if cfg.TLSConfig == nil {
			return newsletterRuntimeConfig{}, fmt.Errorf("SMTP TLS certificate is required outside APP_ENV=local")
		}
		if err := validateSMTPTLSCertificate(cfg.TLSConfig, cfg.Hostname, time.Now()); err != nil {
			return newsletterRuntimeConfig{}, fmt.Errorf("SMTP TLS certificate: %w", err)
		}
	}
	return cfg, nil
}

func validateSMTPTLSCertificate(cfg *tls.Config, hostname string, now time.Time) error {
	if len(cfg.Certificates) == 0 || len(cfg.Certificates[0].Certificate) == 0 {
		return fmt.Errorf("certificate chain is empty")
	}
	leaf := cfg.Certificates[0].Leaf
	if leaf == nil {
		var err error
		leaf, err = x509.ParseCertificate(cfg.Certificates[0].Certificate[0])
		if err != nil {
			return fmt.Errorf("parse leaf: %w", err)
		}
	}
	if now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
		return fmt.Errorf("is not valid at the current time")
	}
	if err := leaf.VerifyHostname(hostname); err != nil {
		return fmt.Errorf("is not valid for SMTP_HOSTNAME %q: %w", hostname, err)
	}
	return nil
}

func loadSMTPTLSConfig() (*tls.Config, error) {
	certFile := strings.TrimSpace(os.Getenv("SMTP_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("SMTP_TLS_KEY_FILE"))
	certB64 := strings.TrimSpace(os.Getenv("SMTP_TLS_CERT_B64"))
	keyB64 := strings.TrimSpace(os.Getenv("SMTP_TLS_KEY_B64"))

	if (certFile == "") != (keyFile == "") {
		return nil, fmt.Errorf("SMTP_TLS_CERT_FILE and SMTP_TLS_KEY_FILE must be set together")
	}
	if (certB64 == "") != (keyB64 == "") {
		return nil, fmt.Errorf("SMTP_TLS_CERT_B64 and SMTP_TLS_KEY_B64 must be set together")
	}
	if certFile != "" && certB64 != "" {
		return nil, fmt.Errorf("configure SMTP TLS with files or base64 PEM, not both")
	}
	if certFile == "" && certB64 == "" {
		return nil, nil
	}

	var cert tls.Certificate
	var err error
	if certFile != "" {
		cert, err = tls.LoadX509KeyPair(certFile, keyFile)
	} else {
		certPEM, decodeErr := base64.StdEncoding.DecodeString(certB64)
		if decodeErr != nil {
			return nil, fmt.Errorf("SMTP_TLS_CERT_B64 base64 decode: %w", decodeErr)
		}
		keyPEM, decodeErr := base64.StdEncoding.DecodeString(keyB64)
		if decodeErr != nil {
			return nil, fmt.Errorf("SMTP_TLS_KEY_B64 base64 decode: %w", decodeErr)
		}
		cert, err = tls.X509KeyPair(certPEM, keyPEM)
	}
	if err != nil {
		return nil, fmt.Errorf("load SMTP TLS keypair: %w", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func positiveInt64Env(name string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid %s %q", name, raw)
	}
	return n, nil
}

func positiveIntEnv(name string, fallback int) (int, error) {
	n, err := positiveInt64Env(name, int64(fallback))
	return int(n), err
}
