package main

import (
	"errors"
	"net"
	"time"
)

// reply is the result of a single echo request.
type reply struct {
	from  net.IP
	bytes int
	rtt   time.Duration
	ttl   int // -1 when unknown

	// unreachable is set when an ICMP error (e.g. "Destination host
	// unreachable") came back instead of an echo reply.
	unreachable string
}

var errTimeout = errors.New("request timed out")

// makePayload fills the echo request with a sparkly pattern.
func makePayload(size int) []byte {
	const pattern = "unicorn-pong!"
	b := make([]byte, size)
	for i := range b {
		b[i] = pattern[i%len(pattern)]
	}
	return b
}
