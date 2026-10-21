//go:build windows

package detector

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// MIB_IPFORWARD_ROW2, Windows SDK netioapi.h (104 bytes on amd64/arm64).
type windowsRouteRow struct {
	Luid                                               uint64
	Index                                              uint32
	Destination                                        [28]byte
	Prefix                                             uint8
	Pad1                                               [3]byte
	NextHop                                            [28]byte
	SitePrefix                                         uint8
	Pad2                                               [3]byte
	ValidLifetime, PreferredLifetime, Metric, Protocol uint32
	Loopback, Autoconfigure, Publish, Immortal         uint8
	Age, Origin                                        uint32
}

var routeDLL = windows.NewLazySystemDLL("iphlpapi.dll")
var routeGetTable = routeDLL.NewProc("GetIpForwardTable2")
var routeFreeTable = routeDLL.NewProc("FreeMibTable")
var routeGetEntry = routeDLL.NewProc("GetIpForwardEntry2")
var routeSeen = struct {
	sync.Mutex
	rows map[string]string
}{rows: map[string]string{}}

func routeSocketAddress(s [28]byte) string {
	family := uint16(s[0]) | uint16(s[1])<<8
	if family == windows.AF_INET {
		return netip.AddrFrom4([4]byte{s[4], s[5], s[6], s[7]}).String()
	}
	var ip [16]byte
	copy(ip[:], s[8:24])
	return fmt.Sprintf("%s%%%d", netip.AddrFrom16(ip), uint32(s[24])|uint32(s[25])<<8|uint32(s[26])<<16|uint32(s[27])<<24)
}

func routeRowSignature(r windowsRouteRow) (string, string) {
	key := fmt.Sprintf("%d/%d/%s/%d/%s", r.Luid, r.Index, routeSocketAddress(r.Destination), r.Prefix, routeSocketAddress(r.NextHop))
	// Age and remaining lifetimes change without changing forwarding decisions.
	value := fmt.Sprintf("%d/%d/%d/%d/%d/%d/%d/%d", r.SitePrefix, r.Metric, r.Protocol, r.Loopback, r.Autoconfigure, r.Publish, r.Immortal, r.Origin)
	return key, value
}

func windowsRouteState() (string, map[string]string, error) {
	var table unsafe.Pointer
	code, _, _ := routeGetTable.Call(windows.AF_UNSPEC, uintptr(unsafe.Pointer(&table)))
	if code != 0 {
		return "", nil, syscall.Errno(code)
	}
	defer routeFreeTable.Call(uintptr(table))
	count := *(*uint32)(table)
	// The SDK inserts padding between NumEntries and the first aligned row.
	type nativeTable struct {
		Count uint32
		Pad   uint32
		Row   windowsRouteRow
	}
	rows := unsafe.Slice((*windowsRouteRow)(unsafe.Add(table, unsafe.Offsetof(nativeTable{}.Row))), int(count))
	state := make(map[string]string, len(rows))
	entries := make([]string, 0, len(rows))
	for _, row := range rows {
		key, value := routeRowSignature(row)
		state[key] = value
		entries = append(entries, key+"="+value)
	}
	sort.Strings(entries)
	return strings.Join(entries, "|"), state, nil
}

var windowsRouteChanged = syscall.NewCallback(func(_ uintptr, rowPointer uintptr, notification uintptr) uintptr {
	if rowPointer == 0 {
		return 0
	}
	row := *(*windowsRouteRow)(unsafe.Pointer(rowPointer))
	key, _ := routeRowSignature(row)
	if notification != 2 { // MibDeleteInstance: only the key remains valid.
		code, _, _ := routeGetEntry.Call(uintptr(unsafe.Pointer(&row)))
		if code != 0 {
			networkGeneration.Add(1)
			return 0
		}
	}
	_, value := routeRowSignature(row)
	routeSeen.Lock()
	defer routeSeen.Unlock()
	old, found := routeSeen.rows[key]
	if notification == 2 {
		if found {
			delete(routeSeen.rows, key)
			networkGeneration.Add(1)
		}
	} else if !found || old != value {
		routeSeen.rows[key] = value
		networkGeneration.Add(1)
	}
	return 0
})
