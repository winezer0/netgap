//go:build !windows

package detector

import (
	"context"
	"fmt"
	"net/url"
)

func resolveAutoProxy(ctx context.Context, rules *proxyRules, target *url.URL) ([]ProxyRoute, error) {
	return nil, fmt.Errorf("automatic system proxy resolution is unavailable on this platform")
}
