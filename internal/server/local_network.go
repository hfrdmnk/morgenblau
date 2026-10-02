package server

import (
	"fmt"

	"morgenblau/internal/safehttp"
	"morgenblau/internal/session"
)

// localNetwork is the throwaway PLC and PDS a verify run starts beside an APP_ENV=local server; NewServer's loopback, identity and OAuth overrides all come from it.
type localNetwork struct {
	plc, pds string
	ports    []int
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
	return &localNetwork{plc: plc, pds: pds, ports: []int{plcPort, pdsPort}}, nil
}

// checkDevPDS refuses a plain-HTTP dev PDS other than the local network's, since the safe client reaches loopback only on that network's ports.
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
