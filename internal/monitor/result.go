package monitor

import (
	"strings"

	"netgap/internal/detector"
	"netgap/internal/ipchecker/types"
)

type CheckResult struct {
	FetcherName string
	Proxy       *detector.ProxyConfig
	IPInfo      *types.IPInfo
	IsLocal     bool
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
