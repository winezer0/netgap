//go:build linux

// Package routenet provides bounded, cancellable Linux route netlink exchanges.
package routenet

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func Dump(ctx context.Context) ([]syscall.NetlinkMessage, error) {
	body := make([]byte, unix.SizeofRtMsg)
	body[0] = unix.AF_UNSPEC
	return exchange(ctx, unix.RTM_GETROUTE, unix.NLM_F_DUMP, body)
}

func Change(ctx context.Context, kind uint16, flags uint16, body []byte) error {
	_, err := exchange(ctx, kind, flags|unix.NLM_F_ACK, body)
	return err
}

func exchange(ctx context.Context, kind, flags uint16, body []byte) ([]syscall.NetlinkMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "route-netlink")
	defer file.Close()
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, err
	}
	packet := make([]byte, unix.NLMSG_HDRLEN+len(body))
	binary.NativeEndian.PutUint32(packet, uint32(len(packet)))
	binary.NativeEndian.PutUint16(packet[4:], kind)
	binary.NativeEndian.PutUint16(packet[6:], flags|unix.NLM_F_REQUEST)
	binary.NativeEndian.PutUint32(packet[8:], 1)
	copy(packet[unix.NLMSG_HDRLEN:], body)
	if err := unix.Sendto(fd, packet, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, err
	}
	var rows []syscall.NetlinkMessage
	buffer := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		if _, err := unix.Poll(poll, 50); err != nil && !errors.Is(err, unix.EINTR) {
			return nil, err
		}
		n, _, recvFlags, from, err := unix.Recvmsg(fd, buffer, nil, unix.MSG_DONTWAIT)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if recvFlags&unix.MSG_TRUNC != 0 {
			return nil, errors.New("route netlink response truncated")
		}
		peer, ok := from.(*unix.SockaddrNetlink)
		if !ok || peer.Pid != 0 {
			continue
		}
		messages, err := syscall.ParseNetlinkMessage(buffer[:n])
		if err != nil {
			return nil, err
		}
		for _, msg := range messages {
			if msg.Header.Seq != 1 {
				continue
			}
			if msg.Header.Flags&unix.NLM_F_DUMP_INTR != 0 {
				return nil, errors.New("route table changed during netlink dump")
			}
			switch msg.Header.Type {
			case unix.NLMSG_DONE:
				if len(msg.Data) >= 4 && int32(binary.NativeEndian.Uint32(msg.Data)) < 0 {
					return nil, syscall.Errno(-int32(binary.NativeEndian.Uint32(msg.Data)))
				}
				return rows, nil
			case unix.NLMSG_ERROR:
				if len(msg.Data) < 4 {
					return nil, errors.New("invalid route netlink acknowledgement")
				}
				code := int32(binary.NativeEndian.Uint32(msg.Data))
				if code != 0 {
					return nil, fmt.Errorf("route netlink: %w", syscall.Errno(-code))
				}
				return rows, nil
			case unix.NLMSG_OVERRUN:
				return nil, errors.New("route netlink response overrun")
			default:
				msg.Data = append([]byte(nil), msg.Data...)
				rows = append(rows, msg)
			}
		}
	}
}

// Stable removes kernel counters and decreasing lifetimes from comparisons.
func Stable(data []byte) ([]byte, error) {
	if len(data) < unix.SizeofRtMsg {
		return nil, errors.New("short route message")
	}
	result := append([]byte(nil), data[:unix.SizeofRtMsg]...)
	for offset := unix.SizeofRtMsg; offset < len(data); {
		if len(data)-offset < 4 {
			return nil, errors.New("short route attribute")
		}
		n := int(binary.NativeEndian.Uint16(data[offset:]))
		kind := binary.NativeEndian.Uint16(data[offset+2:]) & 0x3fff
		aligned := (n + 3) & ^3
		if n < 4 || offset+aligned > len(data) {
			return nil, errors.New("invalid route attribute length")
		}
		if kind != unix.RTA_CACHEINFO && kind != unix.RTA_EXPIRES {
			result = append(result, data[offset:offset+aligned]...)
		}
		offset += aligned
	}
	return result, nil
}
