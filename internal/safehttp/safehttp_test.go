package safehttp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUserAgent_IdentifiesMorgenblau(t *testing.T) {
	if !strings.Contains(UserAgent, "Morgenblau/") {
		t.Errorf("UserAgent = %q, want it to contain %q", UserAgent, "Morgenblau/")
	}
	if !strings.Contains(UserAgent, "+https://morgen.blue/about") {
		t.Errorf("UserAgent = %q, want it to contain %q", UserAgent, "+https://morgen.blue/about")
	}
}

func TestValidator_Blocks(t *testing.T) {
	cases := []struct {
		name string
		ip   string
	}{
		{"loopback v4", "127.0.0.1"},
		{"loopback v6", "::1"},
		{"private 10.0.0.0/8", "10.0.0.1"},
		{"private 172.16.0.0/12", "172.16.5.4"},
		{"private 192.168.0.0/16", "192.168.1.1"},
		{"link-local v4", "169.254.169.254"},
		{"link-local v6", "fe80::1"},
		{"unique-local v6", "fc00::1"},
		{"multicast v4", "224.0.0.1"},
		{"multicast v6", "ff02::1"},
		{"unspecified v4", "0.0.0.0"},
		{"unspecified v6", "::"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("parse %q", tc.ip)
			}
			if err := Validator(ip); err == nil {
				t.Errorf("Validator(%s) = nil, want error", tc.ip)
			} else if !errors.Is(err, ErrBlockedAddress) {
				t.Errorf("err = %v, want ErrBlockedAddress", err)
			}
		})
	}
}

func TestValidator_Allows(t *testing.T) {
	cases := []string{
		"8.8.8.8",
		"1.1.1.1",
		"2606:4700:4700::1111",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			ip := net.ParseIP(raw)
			if ip == nil {
				t.Fatalf("parse %q", raw)
			}
			if err := Validator(ip); err != nil {
				t.Errorf("Validator(%s) = %v, want nil", raw, err)
			}
		})
	}
}

// Port 0 makes an unblocked dial fail with connection-refused too, so we assert the safehttp: marker to confirm our block fired.
func TestNewClient_BlocksBlockedIPs(t *testing.T) {
	cases := []string{
		"http://127.0.0.1:1/",
		"http://10.0.0.1:80/",
		"http://169.254.169.254/",
		"http://[::1]:1/",
		"http://0.0.0.0:1/",
		"http://[fe80::1]:1/",
		"http://224.0.0.1:1/",
	}
	c := NewClient(2*time.Second, 5)
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, raw, nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			_, err = c.Do(req)
			if err == nil {
				t.Fatalf("Do(%s) = nil error, expected block", raw)
			}
			if !strings.Contains(err.Error(), "safehttp:") {
				t.Errorf("err = %v, want safehttp: marker", err)
			}
		})
	}
}

// A redirect to a non-http scheme is rejected by CheckRedirect before any dial.
func TestNewClient_RejectsRedirectToBadScheme(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	defer srv.Close()

	c := NewClient(2*time.Second, 5, WithAllowLoopback())
	_, err := c.Get(srv.URL)
	if err == nil {
		t.Fatal("expected blocked-scheme error, got nil")
	}
	if !errors.Is(err, ErrBlockedScheme) {
		t.Errorf("err = %v, want ErrBlockedScheme", err)
	}
}

// The dialer's Control callback runs on every dial, including redirect dials, so a 302 to 127.0.0.1 can't pivot the client.
func TestNewClient_RejectsRedirectToBlockedIP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// WithAllowLoopback only opens 127/8, so this private RFC1918 redirect still gets blocked.
		http.Redirect(w, r, "http://10.0.0.1:1/", http.StatusFound)
	}))
	defer srv.Close()

	c := NewClient(2*time.Second, 5, WithAllowLoopback())
	_, err := c.Get(srv.URL)
	if err == nil {
		t.Fatal("expected blocked-address error, got nil")
	}
	if !strings.Contains(err.Error(), "safehttp:") {
		t.Errorf("err = %v, want safehttp marker", err)
	}
}

func TestNewClient_RedirectCap(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Always redirect, exceeding the cap.
		http.Redirect(w, r, "/next"+r.URL.Path, http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(2*time.Second, 3, WithAllowLoopback())
	_, err := c.Get(srv.URL + "/")
	if err == nil {
		t.Fatal("expected too-many-redirects error")
	}
	if !errors.Is(err, ErrTooManyRedirects) {
		t.Errorf("err = %v, want ErrTooManyRedirects", err)
	}
}

func TestNewClient_AllowLoopbackPermitsHTTPTest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient(2*time.Second, 5, WithAllowLoopback())
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestNewClient_DefaultBlocksHTTPTest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient(2*time.Second, 5)
	_, err := c.Get(srv.URL)
	if err == nil {
		t.Fatal("expected loopback block when WithAllowLoopback isn't set")
	}
}

// Sanity: the context flows through so callers can cancel.
func TestNewClient_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(1 * time.Second):
		}
	}))
	defer srv.Close()

	c := NewClient(5*time.Second, 5, WithAllowLoopback())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	_, err := c.Do(req)
	if err == nil {
		t.Fatal("expected context error")
	}
}
