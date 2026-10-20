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
}

// PathTag reports the actual final request path without proxy credentials.
func (r *CheckResult) PathTag() string {
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
