//go:build linux

package detector

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
	"netgap/internal/routenet"
)

func extraNetworkState(ctx context.Context) (string, error) {
	revision, err := routenet.Revision()
	if err != nil {
		return "", err
	}
	messages, err := routenet.Dump(ctx)
	if err != nil {
		return "", err
	}
	var routes []string
	for _, msg := range messages {
		if msg.Header.Type != unix.RTM_NEWROUTE {
			continue
		}
		stable, err := routenet.Stable(msg.Data)
		if err != nil {
			return "", err
		}
		routes = append(routes, fmt.Sprintf("%x", stable))
	}
	sort.Strings(routes)
	after, err := routenet.Revision()
	if err != nil {
		return "", err
	}
	if revision != after {
		return "", ErrStateChanged
	}
	return fmt.Sprintf("generation=%d|", after) + strings.Join(routes, "|"), nil
}
