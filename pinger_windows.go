//go:build windows

package main

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// On Windows we use the same API as ping.exe (IcmpSendEcho), which works
// without administrator rights.

var (
	iphlpapi            = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreateFile  = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpCloseHandle = iphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho    = iphlpapi.NewProc("IcmpSendEcho")
)

type ipOptionInformation struct {
	TTL         uint8
	Tos         uint8
	Flags       uint8
	OptionsSize uint8
	OptionsData uintptr
}

type icmpEchoReply struct {
	Address       uint32
	Status        uint32
	RoundTripTime uint32
	DataSize      uint16
	Reserved      uint16
	Data          uintptr
	Options       ipOptionInformation
}

const ipReqTimedOut = 11010

var ipStatusText = map[uint32]string{
	11002: "Destination net unreachable",
	11003: "Destination host unreachable",
	11004: "Destination protocol unreachable",
	11005: "Destination port unreachable",
	11010: "Request timed out",
	11013: "TTL expired in transit",
	11050: "General failure",
}

type pinger struct{}

func newPinger() (*pinger, error) {
	if err := procIcmpSendEcho.Find(); err != nil {
		return nil, fmt.Errorf("ICMP API not available: %w", err)
	}
	return &pinger{}, nil
}

func (p *pinger) Ping(ip net.IP, size int, timeout time.Duration, seq int) (reply, error) {
	ip4 := ip.To4()
	if ip4 == nil {
		return reply{}, errors.New("only IPv4 is supported")
	}

	h, _, err := procIcmpCreateFile.Call()
	if windows.Handle(h) == windows.InvalidHandle {
		return reply{}, err
	}
	defer procIcmpCloseHandle.Call(h)

	// IPAddr is in network byte order, i.e. the raw 4 bytes in memory.
	dest := *(*uint32)(unsafe.Pointer(&ip4[0]))

	payload := makePayload(size)
	var payloadPtr uintptr
	if size > 0 {
		payloadPtr = uintptr(unsafe.Pointer(&payload[0]))
	}

	// Room for one reply + echoed data + an ICMP error header, with margin.
	replySize := unsafe.Sizeof(icmpEchoReply{}) + uintptr(size) + 8 + 64
	buf := make([]byte, replySize)
	opts := ipOptionInformation{TTL: 128}

	n, _, callErr := procIcmpSendEcho.Call(
		h,
		uintptr(dest),
		payloadPtr,
		uintptr(size),
		uintptr(unsafe.Pointer(&opts)),
		uintptr(unsafe.Pointer(&buf[0])),
		replySize,
		uintptr(uint32(timeout/time.Millisecond)),
	)
	if n == 0 {
		if errno, ok := callErr.(syscall.Errno); ok {
			if errno == ipReqTimedOut {
				return reply{}, errTimeout
			}
			if txt, ok := ipStatusText[uint32(errno)]; ok {
				return reply{}, errors.New(txt)
			}
		}
		return reply{}, callErr
	}

	r := (*icmpEchoReply)(unsafe.Pointer(&buf[0]))
	from := net.IPv4(byte(r.Address), byte(r.Address>>8), byte(r.Address>>16), byte(r.Address>>24))
	if r.Status != 0 {
		if r.Status == ipReqTimedOut {
			return reply{}, errTimeout
		}
		txt, ok := ipStatusText[r.Status]
		if !ok {
			txt = fmt.Sprintf("ICMP error status %d", r.Status)
		}
		return reply{from: from, unreachable: txt}, nil
	}
	return reply{
		from:  from,
		bytes: int(r.DataSize),
		rtt:   time.Duration(r.RoundTripTime) * time.Millisecond,
		ttl:   int(r.Options.TTL),
	}, nil
}
