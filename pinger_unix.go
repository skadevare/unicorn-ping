//go:build !windows

package main

import (
	"errors"
	"net"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// On macOS (and Linux with ping_group_range set) unprivileged "udp4" ICMP
// sockets work without sudo. We fall back to raw sockets if needed.

type pinger struct {
	network string
}

var idCounter atomic.Uint32

func newPinger() (*pinger, error) {
	for _, nw := range []string{"udp4", "ip4:icmp"} {
		c, err := icmp.ListenPacket(nw, "0.0.0.0")
		if err == nil {
			c.Close()
			return &pinger{network: nw}, nil
		}
	}
	return nil, errors.New("cannot open an ICMP socket (on Linux run with sudo or set net.ipv4.ping_group_range)")
}

func (p *pinger) Ping(ip net.IP, size int, timeout time.Duration, seq int) (reply, error) {
	ip4 := ip.To4()
	if ip4 == nil {
		return reply{}, errors.New("only IPv4 is supported")
	}

	c, err := icmp.ListenPacket(p.network, "0.0.0.0")
	if err != nil {
		return reply{}, err
	}
	defer c.Close()

	id := int((uint32(os.Getpid()) + idCounter.Add(1)) & 0xffff)
	seq &= 0xffff
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Body: &icmp.Echo{ID: id, Seq: seq, Data: makePayload(size)},
	}
	wb, err := msg.Marshal(nil)
	if err != nil {
		return reply{}, err
	}

	var dst net.Addr = &net.IPAddr{IP: ip4}
	if p.network == "udp4" {
		dst = &net.UDPAddr{IP: ip4}
	}

	pc := c.IPv4PacketConn()
	_ = pc.SetControlMessage(ipv4.FlagTTL, true) // best effort
	if err := pc.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return reply{}, err
	}

	start := time.Now()
	if _, err := c.WriteTo(wb, dst); err != nil {
		return reply{}, err
	}

	buf := make([]byte, size+1024)
	for {
		n, cm, src, err := pc.ReadFrom(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return reply{}, errTimeout
			}
			return reply{}, err
		}
		rtt := time.Since(start)

		data := buf[:n]
		ttl := -1
		if cm != nil && cm.TTL > 0 {
			ttl = cm.TTL
		}
		// macOS hands us the IPv4 header on datagram ICMP sockets.
		// An echo reply starts with type 0, so 0x4? means an IP header.
		if n >= 20 && data[0]>>4 == 4 {
			hl := int(data[0]&0x0f) * 4
			if n >= hl {
				ttl = int(data[8])
				data = data[hl:]
			}
		}

		m, err := icmp.ParseMessage(1, data)
		if err != nil || m.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		echo, ok := m.Body.(*icmp.Echo)
		if !ok || echo.Seq != seq {
			continue
		}
		from := addrIP(src)
		if !from.Equal(ip4) {
			continue
		}
		// Raw sockets see everyone's replies; datagram sockets may have
		// their ID rewritten by the kernel, so only check ID on raw.
		if p.network != "udp4" && echo.ID != id {
			continue
		}
		return reply{from: from, bytes: len(echo.Data), rtt: rtt, ttl: ttl}, nil
	}
}

func addrIP(a net.Addr) net.IP {
	switch v := a.(type) {
	case *net.UDPAddr:
		return v.IP
	case *net.IPAddr:
		return v.IP
	}
	return nil
}
