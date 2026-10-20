//go:build windows

package detector

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var networkGeneration atomic.Uint64
var networkWatchOnce sync.Once
var networkWatchErr error
var networkWatchHandles [2]uintptr
var networkChanged = syscall.NewCallback(func(context, row, notification uintptr) uintptr { networkGeneration.Add(1); return 0 })

func networkState(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	networkWatchOnce.Do(func() {
		dll := windows.NewLazySystemDLL("iphlpapi.dll")
		for i, name := range []string{"NotifyRouteChange2", "NotifyIpInterfaceChange"} {
			proc := dll.NewProc(name)
			if err := proc.Find(); err != nil {
				networkWatchErr = err
				return
			}
			code, _, _ := proc.Call(0, networkChanged, 0, 0, uintptr(unsafe.Pointer(&networkWatchHandles[i])))
			if code != 0 {
				networkWatchErr = syscall.Errno(code)
				return
			}
		}
	})
	if networkWatchErr != nil {
		return "", fmt.Errorf("watch network changes: %w", networkWatchErr)
	}
	before := networkGeneration.Load()
	size := uint32(15000)
	var buffer []byte
	for attempts := 0; attempts < 4; attempts++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		buffer = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, 0x80, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0])), &size)
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue
		}
		if err != nil {
			return "", err
		}
		var entries []string
		for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0])); a != nil; a = a.Next {
			entries = append(entries, fmt.Sprintf("%d/%d/%d/%v/%d/%d", a.Luid, a.IfIndex, a.OperStatus, a.NetworkGuid, a.Ipv4Metric, a.Ipv6Metric))
			for u := a.FirstUnicastAddress; u != nil; u = u.Next {
				entries = append(entries, fmt.Sprintf("%d/ip/%s/%d", a.Luid, u.Address.IP(), u.OnLinkPrefixLength))
			}
			for g := a.FirstGatewayAddress; g != nil; g = g.Next {
				entries = append(entries, fmt.Sprintf("%d/gw/%s", a.Luid, g.Address.IP()))
			}
			for d := a.FirstDnsServerAddress; d != nil; d = d.Next {
				entries = append(entries, fmt.Sprintf("%d/dns/%s", a.Luid, d.Address.IP()))
			}
		}
		sort.Strings(entries)
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if before != networkGeneration.Load() {
			return "", fmt.Errorf("%w while capturing state", ErrStateChanged)
		}
		return fmt.Sprint(before) + strings.Join(entries, "|"), nil
	}
	return "", errors.New("network adapter table kept changing")
}
