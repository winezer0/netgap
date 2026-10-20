package monitor

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"netgap/internal/detector"
	"netgap/internal/ipchecker"
)

func TestFailureKind(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{detector.ErrStateChanged, "network_changed"},
		{&url.Error{Op: "Get", URL: "https://example", Err: detector.ErrProxyRules}, "proxy_rules"},
		{ipchecker.ErrProxyUnavailable, "proxy_unavailable"},
		{ipchecker.ErrNoAvailableFetchers, "api_unavailable"},
		{ipchecker.ErrAPIResponse, "api_response"},
		{ErrLocalResolution, "local_resolution"},
		{context.Canceled, "canceled"},
		{context.DeadlineExceeded, "timeout"},
	} {
		if got := FailureKind(fmt.Errorf("wrapped: %w", tc.err)); got != tc.want {
			t.Fatalf("%v: %s != %s", tc.err, got, tc.want)
		}
	}
}
