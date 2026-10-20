//go:build !windows

package detector

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

func networkState() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	var state []string
	for _, i := range interfaces {
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
	return strings.Join(state, "|"), nil
}
