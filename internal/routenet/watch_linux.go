//go:build linux

package routenet

import (
	"encoding/binary"
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
	"netgap/internal/netwatch"
)

var changes = netwatch.New(openChanges)

func Revision() (uint64, error) { return changes.Revision() }

type changeSocket struct{ fd int }

func openChanges() (netwatch.Source, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, err
	}
	// RTNLGRP_IPV6_RULE is group 19 in linux/rtnetlink.h; x/sys does not
	// expose the corresponding legacy RTMGRP bitmask constant.
	const ipv6RuleGroup = 1 << (19 - 1)
	groups := uint32(unix.RTMGRP_LINK | unix.RTMGRP_IPV4_IFADDR | unix.RTMGRP_IPV6_IFADDR | unix.RTMGRP_IPV4_ROUTE | unix.RTMGRP_IPV6_ROUTE | unix.RTMGRP_IPV4_RULE | ipv6RuleGroup)
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Groups: groups}); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return &changeSocket{fd}, nil
}

func (s *changeSocket) Close() error { return unix.Close(s.fd) }

func (s *changeSocket) Drain() (uint64, error) {
	buffer := make([]byte, 1<<20)
	var count uint64
	for i := 0; i < 4096; i++ {
		n, _, flags, from, err := unix.Recvmsg(s.fd, buffer, nil, unix.MSG_DONTWAIT)
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
			return 0, errors.New("network notification truncated or closed")
		}
		peer, ok := from.(*unix.SockaddrNetlink)
		if !ok || peer.Pid != 0 {
			continue
		}
		delta, err := changeEvents(buffer[:n])
		if err != nil {
			return 0, err
		}
		count += delta
	}
	return 0, errors.New("network notification queue did not settle")
}

func changeEvents(data []byte) (uint64, error) {
	if len(data) < unix.NLMSG_HDRLEN {
		return 0, errors.New("short network notification")
	}
	messages, err := syscall.ParseNetlinkMessage(data)
	if err != nil {
		return 0, err
	}
	consumed := 0
	for _, message := range messages {
		consumed += (int(message.Header.Len) + 3) &^ 3
	}
	if consumed != len(data) {
		return 0, errors.New("incomplete network notification datagram")
	}
	var count uint64
	for _, msg := range messages {
		switch msg.Header.Type {
		case unix.NLMSG_OVERRUN, unix.NLMSG_ERROR:
			return 0, errors.New("network notification events lost")
		case unix.RTM_NEWROUTE, unix.RTM_DELROUTE:
			if len(msg.Data) < unix.SizeofRtMsg {
				return 0, errors.New("short route notification")
			}
			if binary.NativeEndian.Uint32(msg.Data[8:12])&unix.RTM_F_CLONED == 0 {
				count++
			}
		case unix.RTM_NEWLINK, unix.RTM_DELLINK, unix.RTM_NEWADDR, unix.RTM_DELADDR, unix.RTM_NEWRULE, unix.RTM_DELRULE:
			count++
		}
	}
	return count, nil
}
