// Package safehttp builds *http.Client values that refuse to connect to
// loopback, link-local, private, multicast, or unspecified addresses. Used by
// every outbound-fetch path that accepts attacker-influenced URLs so a hostile
// feed or redirect can't pivot the server into its own internal network.
package safehttp

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned when a resolved IP fails the safety check.
var ErrBlockedAddress = errors.New("safehttp: blocked address")

// ErrTooManyRedirects is returned when the redirect chain exceeds the cap.
var ErrTooManyRedirects = errors.New("safehttp: too many redirects")

// ErrBlockedScheme is returned when a redirect target isn't http or https.
var ErrBlockedScheme = errors.New("safehttp: blocked scheme")

// Option configures NewClient. The only public option is WithAllowLoopback,
// for tests that need to hit httptest.NewServer (which binds to 127.0.0.1).
type Option func(*options)

type options struct {
	allowLoopback bool
}

// WithAllowLoopback permits connections to loopback addresses. Test-only —
// production never sets this.
func WithAllowLoopback() Option {
	return func(o *options) { o.allowLoopback = true }
}

// Validator returns an error if ip falls into a disallowed range.
func Validator(ip net.IP) error {
	if ip == nil {
		return fmt.Errorf("%w: nil ip", ErrBlockedAddress)
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("%w: unspecified %s", ErrBlockedAddress, ip)
	}
	if ip.IsLoopback() {
		return fmt.Errorf("%w: loopback %s", ErrBlockedAddress, ip)
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("%w: link-local %s", ErrBlockedAddress, ip)
	}
	if ip.IsPrivate() {
		return fmt.Errorf("%w: private %s", ErrBlockedAddress, ip)
	}
	if ip.IsMulticast() {
		return fmt.Errorf("%w: multicast %s", ErrBlockedAddress, ip)
	}
	return nil
}

// NewClient builds an *http.Client whose Dialer's Control callback rejects
// disallowed peer IPs at TCP-dial time (so DNS-rebinding can't slip past)
// and whose CheckRedirect caps hops and re-validates each next-hop scheme.
func NewClient(timeout time.Duration, maxRedirects int, opts ...Option) *http.Client {
	cfg := options{}
	for _, opt := range opts {
		opt(&cfg)
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("safehttp: parse address: %w", err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("%w: not an ip %q", ErrBlockedAddress, host)
			}
			if cfg.allowLoopback && ip.IsLoopback() {
				return nil
			}
			return Validator(ip)
		},
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return ErrTooManyRedirects
			}
			scheme := strings.ToLower(req.URL.Scheme)
			if scheme != "http" && scheme != "https" {
				return fmt.Errorf("%w: %s", ErrBlockedScheme, scheme)
			}
			return nil
		},
	}
}

