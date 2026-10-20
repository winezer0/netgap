//go:build darwin

// Package darwinroute preserves native macOS route messages and socket ACKs.
package darwinroute

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
	"netgap/internal/netwatch"
)

type Entry struct {
	Message *route.RouteMessage
	Raw     []byte
}

var routeWatch = netwatch.New(openRouteWatch)
var commandSequence atomic.Uint32

func Revision() (uint64, error) {
	return routeWatch.Revision()
}

type routeWatchSocket struct{ fd int }

func openRouteWatch() (netwatch.Source, error) {
	fd, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
	if err != nil {
		return nil, err
	}
	unix.CloseOnExec(fd)
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return &routeWatchSocket{fd}, nil
}

func (s *routeWatchSocket) Close() error { return unix.Close(s.fd) }
func (s *routeWatchSocket) Drain() (uint64, error) {
	buffer := make([]byte, 1<<16)
	var count uint64
	for i := 0; i < 4096; i++ {
		n, _, flags, _, err := unix.Recvmsg(s.fd, buffer, nil, unix.MSG_DONTWAIT)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if errors.Is(err, unix.EAGAIN) {
			return count, nil
		}
		if err != nil {
			return 0, err
		}
		if n == 0 || flags&unix.MSG_TRUNC != 0 {
			return 0, errors.New("macOS route notification truncated or closed")
		}
		for data := buffer[:n]; len(data) > 0; {
			if len(data) < 4 {
				return 0, errors.New("short macOS route notification")
			}
			length := int(binary.NativeEndian.Uint16(data))
			if length < 4 || length > len(data) {
				return 0, errors.New("invalid macOS route notification")
			}
			switch data[3] {
			case unix.RTM_ADD, unix.RTM_DELETE, unix.RTM_CHANGE:
				if length < unix.SizeofRtMsghdr {
					return 0, errors.New("short macOS route change")
				}
				if binary.NativeEndian.Uint32(data[24:28]) == 0 && ConfigurationFlags(int(binary.NativeEndian.Uint32(data[8:12]))) {
					count++
				}
			case unix.RTM_NEWADDR, unix.RTM_DELADDR, unix.RTM_IFINFO:
				count++
			}
			data = data[length:]
		}
	}
	return 0, errors.New("macOS route notification queue did not settle")
}

// ConfigurationFlags excludes ARP/NDP entries and per-connection cloned host
// routes: ordinary query traffic must not invalidate its own network snapshot.
func ConfigurationFlags(flags int) bool {
	return flags&(unix.RTF_LLINFO|unix.RTF_WASCLONED) == 0
}

func Parse(raw []byte) (*route.RouteMessage, error) {
	if len(raw) < unix.SizeofRtMsghdr {
		return nil, errors.New("short macOS route message")
	}
	messages, err := route.ParseRIB(route.RIBTypeRoute, raw)
	if err != nil {
		return nil, err
	}
	if len(messages) != 1 {
		return nil, errors.New("expected one macOS route message")
	}
	m, ok := messages[0].(*route.RouteMessage)
	if !ok {
		return nil, errors.New("unexpected macOS route message type")
	}
	return m, nil
}

func Dump(ctx context.Context) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rib, err := route.FetchRIB(unix.AF_UNSPEC, route.RIBTypeRoute, 0)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for len(rib) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(rib) < 4 {
			return nil, errors.New("short macOS routing table")
		}
		n := int(binary.NativeEndian.Uint16(rib))
		if n < unix.SizeofRtMsghdr || n > len(rib) {
			return nil, errors.New("invalid macOS route length")
		}
		raw := append([]byte(nil), rib[:n]...)
		rib = rib[n:]
		m, err := Parse(raw)
		if err != nil {
			return nil, err
		}
		entries = append(entries, Entry{Message: m, Raw: raw})
	}
	return entries, nil
}

const transientFlags = unix.RTF_DONE | unix.RTF_WASCLONED | unix.RTF_IFREF

func normalized(raw []byte) (*route.RouteMessage, error) {
	m, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	m.Flags &^= transientFlags
	// These are lookup hints, not the route's destination or gateway. A source
	// address may change within the same subnet before recovery.
	for _, slot := range []int{unix.RTAX_IFA, unix.RTAX_BRD, unix.RTAX_AUTHOR} {
		if len(m.Addrs) > slot {
			m.Addrs[slot] = nil
		}
	}
	return m, nil
}

func Stable(raw []byte) ([]byte, error) {
	m, err := normalized(raw)
	if err != nil {
		return nil, err
	}
	m.Type, m.ID, m.Seq = unix.RTM_GET, 0, 0
	data, err := m.Marshal()
	if err != nil {
		return nil, err
	}
	// Keep configuration metrics, not live usage/RTT counters or expiration.
	copy(data[36:48], raw[36:48]) // locks, MTU, hop count
	copy(data[52:64], raw[52:64]) // pipes and slow-start threshold
	return data, nil
}

func Change(ctx context.Context, raw []byte, kind int) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	m, err := normalized(raw)
	if err != nil {
		return err
	}
	sequence := commandSequence.Add(1)
	m.Type, m.ID, m.Seq = kind, uintptr(os.Getpid()), int(sequence)
	packet, err := m.Marshal()
	if err != nil {
		return err
	}
	if kind == unix.RTM_ADD {
		// Preserve every writable metric. Expiration is an absolute calendar
		// timestamp on Darwin; never renew an expired dynamic route.
		expires := int64(int32(binary.NativeEndian.Uint32(raw[48:52])))
		if expires > 0 && expires <= time.Now().Unix() {
			return errors.New("saved macOS route expired; reconnect network")
		}
		copy(packet[32:92], raw[32:92])
		inits := uint32(unix.RTV_MTU | unix.RTV_HOPCOUNT | unix.RTV_EXPIRE | unix.RTV_RPIPE | unix.RTV_SPIPE | unix.RTV_SSTHRESH | unix.RTV_RTT | unix.RTV_RTTVAR)
		binary.NativeEndian.PutUint32(packet[32:36], inits)
		clear(packet[72:92]) // packet counters and internal state are read-only
	}
	fd, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), "macOS-route")
	defer file.Close()
	unix.CloseOnExec(fd)
	if err := unix.SetNonblock(fd, true); err != nil {
		return err
	}
	if n, err := unix.Write(fd, packet); err != nil {
		return err
	} else if n != len(packet) {
		return errors.New("short macOS route write")
	}
	buffer := make([]byte, 1<<16)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		if _, err := unix.Poll(poll, 50); err != nil && !errors.Is(err, unix.EINTR) {
			return err
		}
		n, _, flags, _, err := unix.Recvmsg(fd, buffer, nil, unix.MSG_DONTWAIT)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if flags&unix.MSG_TRUNC != 0 {
			return errors.New("truncated macOS route acknowledgement")
		}
		for data := buffer[:n]; len(data) >= 4; {
			length := int(binary.NativeEndian.Uint16(data))
			if length < 4 || length > len(data) {
				return errors.New("invalid macOS route acknowledgement")
			}
			message := data[:length]
			data = data[length:]
			if length < unix.SizeofRtMsghdr || message[2] != unix.RTM_VERSION || int(message[3]) != kind {
				continue
			}
			if binary.NativeEndian.Uint32(message[16:20]) != uint32(os.Getpid()) || binary.NativeEndian.Uint32(message[20:24]) != sequence {
				continue
			}
			code := binary.NativeEndian.Uint32(message[24:28])
			if code != 0 {
				return fmt.Errorf("macOS route operation: %w", syscall.Errno(code))
			}
			return nil
		}
	}
}
