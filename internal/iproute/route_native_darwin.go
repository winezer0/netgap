//go:build darwin

package iproute

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"time"

	"netgap/internal/macnetwork"
)

func getNativeDefaultRoute() (*RouteInfo, error) {
	rows, err := (darwinRouteBackend{}).list(context.Background())
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		i, err := net.InterfaceByIndex(int(r.InterfaceIndex))
		if err != nil {
			return nil, err
		}
		return &RouteInfo{Gateway: r.NextHop, Interface: i.Name}, nil
	}
	return nil, errors.New("no macOS default route found")
}

func addNativeDefaultRoute(gateway string) error {
	if net.ParseIP(gateway) == nil {
		return errors.New("invalid default gateway")
	}
	return exec.Command("/sbin/route", "-n", "add", "default", gateway).Run()
}

func nativeProtectionSupported() error {
	// Fail before enabling protection in no-CGO builds, rather than accepting
	// monitoring with an unavailable system proxy/network backend.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := macnetwork.ProxySettings(ctx)
	return err
}
