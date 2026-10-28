package monitor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"

	"netgap/internal/detector"
	"netgap/internal/ipchecker"
	"netgap/internal/iproute"
)

var ErrLocalResolution = errors.New("local IP location lookup failed")
var ErrRoutePermission = errors.New("admin permission required")

// CheckFailureLog separates invalidated observations from query failures.
func CheckFailureLog(ipSource string, err error) (level, message string) {
	switch {
	case errors.Is(err, detector.ErrStateChanged):
		return "WARN", fmt.Sprintf("Check discarded ipSource[%s] [state=network_changed]: network or proxy settings changed; this result is invalid and will not trigger alerts or disconnection; waiting for the next scheduled check: %v", ipSource, err)
	case errors.Is(err, context.Canceled):
		return "INFO", fmt.Sprintf("Check canceled ipSource[%s] [state=canceled]: %v", ipSource, err)
	default:
		return "ERROR", fmt.Sprintf("Check failed ipSource[%s] [state=%s]: %v", ipSource, FailureKind(err), err)
	}
}

// FailureKind describes what failed without assuming the proxy or API is bad.
func FailureKind(err error) string {
	var pending *iproute.PendingDisconnectError
	switch {
	case errors.Is(err, ErrRoutePermission):
		return "permission"
	case errors.Is(err, detector.ErrStateChanged):
		return "network_changed"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, iproute.ErrDisconnectObsolete):
		return "disconnect_obsolete"
	case errors.As(err, &pending):
		return "disconnect_pending"
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
