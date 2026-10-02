// Package safehttp builds *http.Client values that block loopback, link-local,
// private, multicast, and unspecified addresses so hostile URLs can't pivot into the internal network.
package safehttp

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// UserAgent identifies Morgenblau's outbound bot traffic so operators can attribute and contact us.
const UserAgent = "Morgenblau/0.1 (+https://morgen.blue/about; bot@morgen.blue) Go-http-client/1.1"

// ErrBlockedAddress is returned when a resolved IP fails the safety check.
var ErrBlockedAddress = errors.New("safehttp: blocked address")

// ErrTooManyRedirects is returned when the redirect chain exceeds the cap.
var ErrTooManyRedirects = errors.New("safehttp: too many redirects")

// ErrBlockedScheme is returned when a redirect target isn't http or https.
var ErrBlockedScheme = errors.New("safehttp: blocked scheme")

// Option configures NewClient.
type Option func(*options)

type options struct {
	allowLoopback      bool
	allowLoopbackPorts map[int]bool
}

// WithAllowLoopback permits loopback connections; test-only, production never sets this.
func WithAllowLoopback() Option {
	return func(o *options) { o.allowLoopback = true }
}

// WithAllowLoopbackPorts permits loopback connections to the listed ports only, for an APP_ENV=local server talking to a throwaway PDS and PLC on this machine.
func WithAllowLoopbackPorts(ports ...int) Option {
	return func(o *options) {
		if o.allowLoopbackPorts == nil {
			o.allowLoopbackPorts = map[int]bool{}
		}
		for _, port := range ports {
			o.allowLoopbackPorts[port] = true
		}
	}
}

// IsLoopbackHost reports whether a URL hostname names this machine without a DNS lookup.
func IsLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// LoopbackOrigin parses a plain-HTTP origin on this machine with an explicit port, the only shape a verify run's throwaway PDS, PLC or authorization server takes.
// Loopback origins on the same port are the same server, so callers compare ports, not strings.
func LoopbackOrigin(raw string) (origin string, port int, err error) {
	u, err := url.Parse(strings.TrimSuffix(raw, "/"))
	if err != nil || !strings.EqualFold(u.Scheme, "http") || !IsLoopbackHost(strings.ToLower(u.Hostname())) ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return "", 0, fmt.Errorf("want a plain HTTP loopback origin, got %q", raw)
	}
	port, err = strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("%q needs an explicit port", raw)
	}
	return "http://" + strings.ToLower(u.Host), port, nil
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

// NewClient rejects disallowed peer IPs at TCP-dial time (defeating DNS-rebinding) and caps/revalidates redirects via CheckRedirect.
func NewClient(timeout time.Duration, maxRedirects int, opts ...Option) *http.Client {
	cfg := options{}
	for _, opt := range opts {
		opt(&cfg)
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, rawPort, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("safehttp: parse address: %w", err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("%w: not an ip %q", ErrBlockedAddress, host)
			}
			if ip.IsLoopback() {
				if port, err := strconv.Atoi(rawPort); cfg.allowLoopback || (err == nil && cfg.allowLoopbackPorts[port]) {
					return nil
				}
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
