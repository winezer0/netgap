package monitor

import (
	"context"
	"errors"
	"net"
	"net/url"

	"netgap/internal/detector"
	"netgap/internal/ipchecker"
)

var ErrLocalResolution = errors.New("local IP location lookup failed")

// FailureKind describes what failed without assuming the proxy or API is bad.
func FailureKind(err error) string {
	switch {
	case errors.Is(err, detector.ErrStateChanged):
		return "network_changed"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, detector.ErrProxyRules):
		return "proxy_rules"
	case errors.Is(err, ipchecker.ErrProxyUnavailable):
		return "proxy_unavailable"
	case errors.Is(err, ipchecker.ErrNoAvailableFetchers):
		return "api_unavailable"
	case errors.Is(err, ipchecker.ErrAPIResponse):
		return "api_response"
	case errors.Is(err, ErrLocalResolution):
		return "local_resolution"
	}
	var request *url.Error
	var network net.Error
	if errors.As(err, &request) || errors.As(err, &network) {
		return "network_request"
	}
	return "check_failed"
}
