package server

import (
	"fmt"

	"morgenblau/internal/fetcher"
	"morgenblau/internal/safehttp"
	"morgenblau/internal/session"
)

type localNetwork struct {
	plc, pds      string
	ports         []int
	feedFixture   int
	fixtureOrigin string
}

func loadLocalNetwork(getenv func(string) string) (*localNetwork, error) {
	if getenv("APP_ENV") != "local" || getenv("PLC_URL") == "" {
		return nil, nil
	}
	plc, plcPort, err := safehttp.LoopbackOrigin(getenv("PLC_URL"))
	if err != nil {
		return nil, fmt.Errorf("PLC_URL: %w", err)
	}
	pds, pdsPort, err := safehttp.LoopbackOrigin(getenv("ATPROTO_PDS"))
	if err != nil {
		return nil, fmt.Errorf("ATPROTO_PDS beside PLC_URL: %w", err)
	}
	n := &localNetwork{plc: plc, pds: pds, ports: []int{plcPort, pdsPort}}
	if raw := getenv("FEED_FIXTURE_ORIGIN"); raw != "" {
		n.fixtureOrigin, n.feedFixture, err = safehttp.LoopbackOrigin(raw)
		if err != nil {
			return nil, fmt.Errorf("FEED_FIXTURE_ORIGIN: %w", err)
		}
		n.ports = append(n.ports, n.feedFixture)
	}
	return n, nil
}

func feedFetchOptions(n *localNetwork) []fetcher.Option {
	if n == nil || n.feedFixture == 0 {
		return nil
	}
	return []fetcher.Option{fetcher.WithSafeHTTPOptions(safehttp.WithAllowLoopbackPorts(n.feedFixture))}
}

func checkDevPDS(dev *session.DevConfig, local *localNetwork) error {
	if dev == nil {
		return nil
	}
	_, port, err := safehttp.LoopbackOrigin(dev.PDS)
	if err != nil {
		return nil
	}
	if local != nil {
		if _, localPort, _ := safehttp.LoopbackOrigin(local.pds); port == localPort {
			return nil
		}
	}
	return fmt.Errorf("ATPROTO_PDS %s is plain HTTP on loopback, which the server reaches only as the PDS beside PLC_URL", dev.PDS)
}

func youtubeOrigin(n *localNetwork) string {
	if n == nil || n.fixtureOrigin == "" {
		return "https://www.youtube.com"
	}
	return n.fixtureOrigin
}
