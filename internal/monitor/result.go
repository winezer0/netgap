package monitor

import (
	"context"
	"net"
	"strings"

	"netgap/internal/detector"
	"netgap/internal/ipchecker/types"
)

type CheckResult struct {
	ValidateCurrent func(context.Context) error `json:"-"`
	FetcherName     string
	Proxy           *detector.ProxyConfig
	IPInfo          *types.IPInfo
	IsLocal         bool
	ObservationOnly string
}

func (r *CheckResult) ProtectionGuard(ctx context.Context) error {
	if r != nil && r.ObservationOnly != "" {
		return detector.ErrObservationOnly
	}
	if err := detector.AutomaticProtectionError(ctx); err != nil {
		return err
	}
	if r != nil && r.ValidateCurrent != nil {
		if err := r.ValidateCurrent(ctx); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// PathTag reports the actual final request path without proxy credentials.
func (r *CheckResult) PathTag() string {
	if r != nil && r.ObservationOnly != "" {
		return "SYSTEM_ROUTE (PAC/WPAD not applied; alerts only)"
	}
	if r == nil || r.Proxy == nil {
		return "unknown"
	}
	if !r.Proxy.Enabled {
		return "DIRECT"
	}
	scheme := r.Proxy.Scheme
	if scheme == "" {
		scheme = "http"
	}
	return scheme + "://" + net.JoinHostPort(r.Proxy.Address, r.Proxy.Port)
}

func (r *CheckResult) LocationTag() string {
	if r == nil || r.IPInfo == nil {
		return ""
	}
	region := strings.TrimSpace(r.IPInfo.Region)
	city := strings.TrimSpace(r.IPInfo.City)
	if region == "" && city == "" {
		return ""
	}
	return region + city
}
