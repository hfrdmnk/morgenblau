package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/bluesky-social/indigo/atproto/identity"

	"morgenblau/internal/atidentity"
	"morgenblau/internal/safehttp"
)

// localNetwork is the throwaway PLC and PDS a verify run starts beside an APP_ENV=local server; the server's loopback and identity exceptions all hang off it.
type localNetwork struct {
	plc, pds string
	ports    []int
}

func loadLocalNetwork(getenv func(string) string) (*localNetwork, error) {
	if getenv("APP_ENV") != "local" || getenv("PLC_URL") == "" {
		return nil, nil
	}
	plc, plcPort, err := loopbackOrigin(getenv("PLC_URL"))
	if err != nil {
		return nil, fmt.Errorf("PLC_URL: %w", err)
	}
	pds, pdsPort, err := loopbackOrigin(getenv("ATPROTO_PDS"))
	if err != nil {
		return nil, fmt.Errorf("ATPROTO_PDS beside PLC_URL: %w", err)
	}
	return &localNetwork{plc: plc, pds: pds, ports: []int{plcPort, pdsPort}}, nil
}

func loopbackOrigin(raw string) (string, int, error) {
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || u.Scheme != "http" || !safehttp.IsLoopbackHost(u.Hostname()) || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", 0, fmt.Errorf("want a plain HTTP loopback origin, got %q", raw)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return "", 0, errors.New("the origin needs an explicit port")
	}
	return u.String(), port, nil
}

func (n *localNetwork) clientOptions() []safehttp.Option {
	if n == nil {
		return nil
	}
	return []safehttp.Option{safehttp.WithAllowLoopbackPorts(n.ports...)}
}

func (n *localNetwork) identityDirectory(client *http.Client) identity.Directory {
	if n == nil {
		return atidentity.Guarded(client)
	}
	return atidentity.Local(client, n.plc, n.pds)
}
