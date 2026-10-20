//go:build darwin

package detector

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/url"
	"strconv"

	"netgap/internal/macnetwork"
)

func getProxyFromSystem(ctx context.Context) *ProxyConfig {
	settings, err := macnetwork.ProxySettings(ctx)
	if err != nil {
		return &ProxyConfig{configErr: err}
	}
	if !settings.Configured {
		return nil
	}
	return &ProxyConfig{Enabled: true, Type: "rules", rules: &proxyRules{native: true, servers: fmt.Sprintf("%x", sha256.Sum256([]byte(settings.State)))}}
}

func resolveNativeProxy(ctx context.Context, target *url.URL) ([]ProxyRoute, error) {
	paths, err := macnetwork.ProxyPaths(ctx, target.String())
	if err != nil {
		return nil, err
	}
	var result []ProxyRoute
	for _, path := range paths {
		if path.Scheme == "direct" {
			result = append(result, ProxyRoute{})
			continue
		}
		value := &url.URL{Scheme: path.Scheme, Host: net.JoinHostPort(path.Host, strconv.Itoa(path.Port))}
		if path.Username != "" {
			value.User = url.UserPassword(path.Username, path.Password)
		}
		routes, err := checkedRoute(value)
		if err != nil {
			return nil, err
		}
		result = append(result, routes...)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("macOS returned no system proxy paths")
	}
	return result, nil
}
