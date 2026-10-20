//go:build darwin

package iproute

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
	"netgap/internal/darwinroute"
	"netgap/internal/macnetwork"
)

type darwinRouteBackend struct{}

func newRouteBackend() routeBackend { return darwinRouteBackend{} }

func lockSnapshotFile(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another process may be changing routes: %w", err)
	}
	return func() { f.Close() }, nil
}

func zeroRouteAddress(address route.Addr) bool {
	switch a := address.(type) {
	case nil:
		return true
	case *route.Inet4Addr:
		return a.IP == [4]byte{}
	case *route.Inet6Addr:
		return a.IP == [16]byte{}
	case *route.DefaultAddr:
		for _, b := range a.Raw {
			if b != 0 {
				return false
			}
		}
		return true
	}
	return false
}

func darwinBinding(ctx context.Context, index uint32, destination string) (RouteEntry, error) {
	if err := ctx.Err(); err != nil {
		return RouteEntry{}, err
	}
	i, err := net.InterfaceByIndex(int(index))
	if err != nil {
		return RouteEntry{}, err
	}
	family := 4
	if destination == "::/0" {
		family = 6
	}
	state, err := macnetwork.Binding(ctx, i.Name, family)
	if err != nil {
		return RouteEntry{}, err
	}
	boot, err := unix.Sysctl("kern.bootsessionuuid")
	if err != nil {
		return RouteEntry{}, err
	}
	r := RouteEntry{InterfaceIndex: index, InterfaceGUID: state.Identity, Network: fmt.Sprintf("%x", sha256.Sum256([]byte(boot+state.Network)))}
	addresses, err := i.Addrs()
	if err != nil {
		return RouteEntry{}, err
	}
	for _, a := range addresses {
		r.Addresses = append(r.Addresses, a.String())
	}
	sort.Strings(r.Addresses)
	if err := ctx.Err(); err != nil {
		return RouteEntry{}, err
	}
	return r, nil
}

func (darwinRouteBackend) list(ctx context.Context) ([]RouteEntry, error) {
	entries, err := darwinroute.Dump(ctx)
	if err != nil {
		return nil, err
	}
	var rows []RouteEntry
	for _, entry := range entries {
		m := entry.Message
		if len(m.Addrs) <= unix.RTAX_NETMASK || m.Addrs[unix.RTAX_DST] == nil || !zeroRouteAddress(m.Addrs[unix.RTAX_DST]) || !zeroRouteAddress(m.Addrs[unix.RTAX_NETMASK]) || m.Flags&unix.RTF_HOST != 0 {
			continue
		}
		family := m.Addrs[unix.RTAX_DST].Family()
		if family != unix.AF_INET && family != unix.AF_INET6 {
			continue
		}
		destination := "0.0.0.0/0"
		if family == unix.AF_INET6 {
			destination = "::/0"
		}
		if m.Index <= 0 {
			return nil, errors.New("macOS default route has no bindable interface; no routes removed")
		}
		r, err := darwinBinding(ctx, uint32(m.Index), destination)
		if err != nil {
			return nil, err
		}
		r.Destination, r.Store, r.Raw = destination, "Darwin", entry.Raw
		r.Bindings = []RouteEntry{{Network: r.Network}}
		r.Network += "/scope=" + strconv.Itoa(m.Flags&unix.RTF_IFSCOPE)
		switch a := m.Addrs[unix.RTAX_GATEWAY].(type) {
		case *route.Inet4Addr:
			r.NextHop = net.IP(a.IP[:]).String()
		case *route.Inet6Addr:
			r.NextHop = net.IP(a.IP[:]).String()
		case *route.LinkAddr:
			r.NextHop = "link#" + strconv.Itoa(a.Index)
		default:
			return nil, errors.New("unsupported macOS default gateway address; no routes removed")
		}
		r.NativeKey, err = darwinroute.Stable(entry.Raw)
		if err != nil {
			return nil, err
		}
		rows = append(rows, r)
	}
	return rows, nil
}

func (darwinRouteBackend) validate(ctx context.Context, rows []RouteEntry) error {
	for _, saved := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := darwinBinding(ctx, saved.InterfaceIndex, saved.Destination)
		if err != nil {
			return err
		}
		if current.InterfaceGUID != saved.InterfaceGUID || len(saved.Bindings) != 1 || current.Network != saved.Bindings[0].Network {
			return errors.New("macOS interface/network identity changed")
		}
		a, b := relevantNetworks(saved.Addresses, saved.Destination, saved.NextHop), relevantNetworks(current.Addresses, saved.Destination, saved.NextHop)
		matches := false
		for p := range a {
			if b[p] {
				matches = true
				break
			}
		}
		if !matches {
			return errors.New("macOS relevant route subnet changed")
		}
	}
	return nil
}

func (b darwinRouteBackend) remove(ctx context.Context, r RouteEntry) error {
	if err := b.validate(ctx, []RouteEntry{r}); err != nil {
		return err
	}
	rows, err := b.list(ctx)
	if err != nil {
		return err
	}
	for _, current := range rows {
		if !sameRouteIdentity(current, r) {
			continue
		}
		if !bytes.Equal(current.NativeKey, r.NativeKey) {
			return errors.New("macOS route attributes changed before removal")
		}
		return darwinroute.Change(ctx, current.Raw, unix.RTM_DELETE)
	}
	return nil
}

func (b darwinRouteBackend) restore(ctx context.Context, r RouteEntry, _ time.Time) error {
	if err := b.validate(ctx, []RouteEntry{r}); err != nil {
		return err
	}
	rows, err := b.list(ctx)
	if err != nil {
		return err
	}
	for _, current := range rows {
		if !sameRouteIdentity(current, r) {
			continue
		}
		if bytes.Equal(current.NativeKey, r.NativeKey) {
			return nil
		}
		return errors.New("existing macOS route conflicts with recovery snapshot")
	}
	if err := darwinroute.Change(ctx, r.Raw, unix.RTM_ADD); err != nil {
		return err
	}
	rows, err = b.list(ctx)
	if err != nil {
		return err
	}
	for _, current := range rows {
		if sameRouteIdentity(current, r) && bytes.Equal(current.NativeKey, r.NativeKey) {
			return nil
		}
	}
	return errors.New("macOS route restoration not confirmed")
}
