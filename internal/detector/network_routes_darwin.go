//go:build darwin

package detector

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"netgap/internal/darwinroute"
	"netgap/internal/macnetwork"
)

func extraNetworkState(ctx context.Context) (string, error) {
	before, err := darwinroute.Revision()
	if err != nil {
		return "", err
	}
	state, err := macnetwork.NetworkState(ctx)
	if err != nil {
		return "", err
	}
	entries, err := darwinroute.Dump(ctx)
	if err != nil {
		return "", err
	}
	var routes []string
	for _, entry := range entries {
		if !darwinroute.ConfigurationFlags(entry.Message.Flags) {
			continue
		}
		stable, err := darwinroute.Stable(entry.Raw)
		if err != nil {
			return "", err
		}
		routes = append(routes, fmt.Sprintf("%x", stable))
	}
	sort.Strings(routes)
	after, err := darwinroute.Revision()
	if err != nil {
		return "", err
	}
	if before != after {
		return "", fmt.Errorf("%w while capturing macOS state", ErrStateChanged)
	}
	return fmt.Sprintf("%d|%s|%s", after, state, strings.Join(routes, "|")), nil
}
