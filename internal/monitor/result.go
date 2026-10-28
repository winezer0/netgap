package monitor

import (
	"context"
	"fmt"
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
	MatchedField    string
	MatchedKeyword  string
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
	fields := []struct {
		name  string
		value string
	}{
		{name: "Country", value: r.IPInfo.Country},
		{name: "Province", value: r.IPInfo.Region},
		{name: "City", value: r.IPInfo.City},
		{name: "ISP", value: r.IPInfo.Isp},
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(field.value)
		if value == "" {
			value = "Unknown"
		}
		parts = append(parts, field.name+": "+value)
	}
	return strings.Join(parts, " | ")
}

// MatchTag describes the raw field and configured keyword that triggered a match.
func (r *CheckResult) MatchTag() string {
	if r == nil || !r.IsLocal {
		return ""
	}
	return fmt.Sprintf("matchedField[%s] keyword[%s]", r.MatchedField, r.MatchedKeyword)
}
