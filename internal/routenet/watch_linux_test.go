//go:build linux

package routenet

import (
	"encoding/binary"
	"golang.org/x/sys/unix"
	"testing"
)

func event(kind uint16, flags uint32) []byte {
	packet := make([]byte, unix.NLMSG_HDRLEN+unix.SizeofRtMsg)
	binary.NativeEndian.PutUint32(packet, uint32(len(packet)))
	binary.NativeEndian.PutUint16(packet[4:], kind)
	binary.NativeEndian.PutUint32(packet[unix.NLMSG_HDRLEN+8:], flags)
	return packet
}

func TestNetworkChangeEvents(t *testing.T) {
	for _, kind := range []uint16{unix.RTM_NEWLINK, unix.RTM_DELLINK, unix.RTM_NEWADDR, unix.RTM_DELADDR, unix.RTM_NEWROUTE, unix.RTM_DELROUTE, unix.RTM_NEWRULE, unix.RTM_DELRULE} {
		n, err := changeEvents(append(event(kind, 0), event(kind, 0)...))
		if err != nil || n != 2 {
			t.Fatal(kind, n, err)
		}
	}
	if n, err := changeEvents(event(unix.RTM_NEWROUTE, unix.RTM_F_CLONED)); err != nil || n != 0 {
		t.Fatal("traffic clone invalidated query", n, err)
	}
	for _, data := range [][]byte{{1, 2, 3, 4}, append(event(unix.RTM_NEWROUTE, 0), 1, 2, 3, 4), event(unix.NLMSG_OVERRUN, 0), event(unix.NLMSG_ERROR, 0)} {
		if _, err := changeEvents(data); err == nil {
			t.Fatal("lost notifications accepted")
		}
	}
}

func TestReadOnlyNetworkSubscription(t *testing.T) {
	before, err := Revision()
	if err != nil {
		t.Fatal(err)
	}
	after, err := Revision()
	if err != nil || after < before {
		t.Fatal(after, err)
	}
}
