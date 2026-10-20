//go:build linux

package iproute

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"netgap/internal/routenet"
)

type linuxRouteBackend struct{}

func newRouteBackend() routeBackend { return linuxRouteBackend{} }

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

func linuxBinding(index uint32) (RouteEntry, error) {
	i, err := net.InterfaceByIndex(int(index))
	if err != nil {
		return RouteEntry{}, err
	}
	device, err := os.Readlink("/sys/class/net/" + i.Name)
	if err != nil {
		return RouteEntry{}, err
	}
	addresses, err := i.Addrs()
	if err != nil {
		return RouteEntry{}, err
	}
	r := RouteEntry{InterfaceIndex: index, InterfaceGUID: device + "/" + i.HardwareAddr.String()}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return RouteEntry{}, err
	}
	carrier, err := os.ReadFile("/sys/class/net/" + i.Name + "/carrier_changes")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return RouteEntry{}, err
	}
	r.Network = strings.TrimSpace(string(boot)) + "/carrier=" + strings.TrimSpace(string(carrier))
	for _, a := range addresses {
		r.Addresses = append(r.Addresses, a.String())
	}
	sort.Strings(r.Addresses)
	return r, nil
}

func (linuxRouteBackend) list(ctx context.Context) ([]RouteEntry, error) {
	messages, err := routenet.Dump(ctx)
	if err != nil {
		return nil, err
	}
	namespace, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		return nil, err
	}
	var rows []RouteEntry
	for _, msg := range messages {
		if msg.Header.Type != unix.RTM_NEWROUTE || len(msg.Data) < unix.SizeofRtMsg || msg.Data[1] != 0 {
			continue
		}
		family := msg.Data[0]
		if family != unix.AF_INET && family != unix.AF_INET6 {
			continue
		}
		attrs, err := syscall.ParseNetlinkRouteAttr(&msg)
		if err != nil {
			return nil, err
		}
		r := RouteEntry{Store: "Linux", Protocol: strconv.Itoa(int(msg.Data[5])), Raw: append([]byte(nil), msg.Data...), Destination: "0.0.0.0/0", NextHop: "0.0.0.0"}
		if family == unix.AF_INET6 {
			r.Destination = "::/0"
			r.NextHop = "::"
		}
		table := uint32(msg.Data[4])
		var source []byte
		var indices []uint32
		for _, a := range attrs {
			switch a.Attr.Type & 0x3fff {
			case unix.RTA_SRC:
				source = a.Value
			case unix.RTA_OIF:
				if len(a.Value) >= 4 {
					indices = append(indices, binary.NativeEndian.Uint32(a.Value))
				}
			case unix.RTA_GATEWAY:
				r.NextHop = net.IP(a.Value).String()
			case unix.RTA_PRIORITY:
				if len(a.Value) >= 4 {
					r.Metric = binary.NativeEndian.Uint32(a.Value)
				}
			case unix.RTA_TABLE:
				if len(a.Value) >= 4 {
					table = binary.NativeEndian.Uint32(a.Value)
				}
			case unix.RTA_MULTIPATH:
				for data := a.Value; len(data) > 0; {
					if len(data) < 8 {
						return nil, errors.New("invalid multipath route")
					}
					n := int(binary.NativeEndian.Uint16(data))
					if n < 8 || (n+3) & ^3 > len(data) {
						return nil, errors.New("invalid multipath hop")
					}
					indices = append(indices, binary.NativeEndian.Uint32(data[4:]))
					data = data[(n+3) & ^3:]
				}
			}
		}
		if len(indices) == 0 {
			return nil, errors.New("default route without an interface cannot be safely bound; no routes removed")
		}
		for _, index := range indices {
			b, err := linuxBinding(index)
			if err != nil {
				return nil, err
			}
			r.Bindings = append(r.Bindings, b)
		}
		r.InterfaceIndex, r.InterfaceGUID, r.Addresses = r.Bindings[0].InterfaceIndex, r.Bindings[0].InterfaceGUID, r.Bindings[0].Addresses
		// Linux permits parallel routes differing only in priority; table/TOS/
		// source selector and priority are part of its route identity.
		var generations []string
		for _, binding := range r.Bindings {
			generations = append(generations, binding.InterfaceGUID+"/"+binding.Network)
		}
		r.Network = fmt.Sprintf("%s/table=%d/tos=%d/type=%d/src=%x/%d/metric=%d/links=%s", namespace, table, msg.Data[3], msg.Data[7], source, msg.Data[2], r.Metric, strings.Join(generations, "|"))
		r.NativeKey, err = routenet.Stable(r.Raw)
		if err != nil {
			return nil, err
		}
		rows = append(rows, r)
	}
	return rows, nil
}

func (linuxRouteBackend) validate(ctx context.Context, rows []RouteEntry) error {
	namespace, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !strings.HasPrefix(r.Network, namespace+"/") {
			return errors.New("network namespace changed")
		}
		for _, saved := range r.Bindings {
			current, err := linuxBinding(saved.InterfaceIndex)
			if err != nil {
				return err
			}
			if current.InterfaceGUID != saved.InterfaceGUID {
				return errors.New("route interface identity changed")
			}
			if current.Network != saved.Network {
				return errors.New("route link reconnected or machine restarted; refusing stale network")
			}
			// A multipath hop can have a different gateway; comparing related
			// family subnets still rejects moving its interface to another LAN.
			a, b := relevantNetworks(saved.Addresses, r.Destination, r.NextHop), relevantNetworks(current.Addresses, r.Destination, r.NextHop)
			match := false
			for p := range a {
				if b[p] {
					match = true
					break
				}
			}
			if !match {
				return errors.New("relevant route subnet changed")
			}
		}
	}
	return nil
}

func linuxSame(a, b RouteEntry) bool {
	x, e1 := routenet.Stable(a.Raw)
	y, e2 := routenet.Stable(b.Raw)
	return sameRouteIdentity(a, b) && e1 == nil && e2 == nil && bytes.Equal(x, y)
}

func (b linuxRouteBackend) remove(ctx context.Context, r RouteEntry) error {
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
		if !linuxSame(current, r) {
			return errors.New("Linux route attributes changed before removal")
		}
		body, err := routenet.Stable(current.Raw)
		if err != nil {
			return err
		}
		return routenet.Change(ctx, unix.RTM_DELROUTE, 0, body)
	}
	return nil
}

func (b linuxRouteBackend) restore(ctx context.Context, r RouteEntry, saved time.Time) error {
	if err := b.validate(ctx, []RouteEntry{r}); err != nil {
		return err
	}
	rows, err := b.list(ctx)
	if err != nil {
		return err
	}
	for _, current := range rows {
		if sameRouteIdentity(current, r) {
			if linuxSame(current, r) {
				return nil
			}
			return errors.New("existing Linux route conflicts with recovery snapshot")
		}
	}
	body, err := routenet.Stable(r.Raw)
	if err != nil {
		return err
	}
	msg := syscall.NetlinkMessage{Header: syscall.NlMsghdr{Type: unix.RTM_NEWROUTE}, Data: r.Raw}
	attrs, err := syscall.ParseNetlinkRouteAttr(&msg)
	if err != nil {
		return err
	}
	for _, a := range attrs {
		if a.Attr.Type&0x3fff == unix.RTA_EXPIRES {
			if len(a.Value) != 4 {
				return errors.New("invalid Linux route expiry")
			}
			seconds := int64(binary.NativeEndian.Uint32(a.Value)) - int64(time.Since(saved)/time.Second)
			if seconds <= 0 {
				return errors.New("saved Linux route expired; reconnect network")
			}
			expiry := make([]byte, 8)
			binary.NativeEndian.PutUint16(expiry, 8)
			binary.NativeEndian.PutUint16(expiry[2:], unix.RTA_EXPIRES)
			binary.NativeEndian.PutUint32(expiry[4:], uint32(seconds))
			body = append(body, expiry...)
		}
	}
	if err := routenet.Change(ctx, unix.RTM_NEWROUTE, unix.NLM_F_CREATE|unix.NLM_F_EXCL, body); err != nil {
		return err
	}
	rows, err = b.list(ctx)
	if err != nil {
		return err
	}
	for _, current := range rows {
		if linuxSame(current, r) {
			return nil
		}
	}
	return errors.New("Linux route restoration not confirmed")
}
