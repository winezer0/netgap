//go:build !windows

package detector

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
)

func networkState(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	var state []string
	for _, i := range interfaces {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		addresses, err := i.Addrs()
		if err != nil {
			return "", err
		}
		state = append(state, fmt.Sprintf("%d/%s/%s/%v", i.Index, i.Name, i.HardwareAddr, i.Flags))
		for _, a := range addresses {
			state = append(state, fmt.Sprintf("%d/%s", i.Index, a))
		}
	}
	sort.Strings(state)
	routes, err := extraNetworkState(ctx)
	if err != nil {
		return "", err
	}
	return strings.Join(state, "|") + routes, nil
}
