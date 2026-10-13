package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"sync"
	"time"
)

// pongWAV synthesizes a short bouncy "pong" as a 16-bit mono WAV file,
// so no audio assets need to ship with the binary.
func pongWAV() []byte {
	const (
		rate = 22050
		dur  = 0.17
	)
	n := rate * 17 / 100 // dur seconds of samples
	pcm := make([]int16, n)
	phase := 0.0
	for i := range pcm {
		t := float64(i) / rate
		freq := 700.0 - 260.0*(t/dur) // pitch drops a little: "pooong"
		phase += 2 * math.Pi * freq / rate
		attack := math.Min(1, t/0.004)
		env := attack * math.Exp(-t*20)
		v := 0.65*math.Sin(phase) + 0.25*math.Sin(2*phase) + 0.1*math.Sin(3*phase)
		pcm[i] = int16(v * env * 0.8 * math.MaxInt16)
	}

	var b bytes.Buffer
	dataLen := uint32(len(pcm) * 2)
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, 36+dataLen)
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))     // fmt chunk size
	binary.Write(&b, binary.LittleEndian, uint16(1))      // PCM
	binary.Write(&b, binary.LittleEndian, uint16(1))      // mono
	binary.Write(&b, binary.LittleEndian, uint32(rate))   // sample rate
	binary.Write(&b, binary.LittleEndian, uint32(rate*2)) // byte rate
	binary.Write(&b, binary.LittleEndian, uint16(2))      // block align
	binary.Write(&b, binary.LittleEndian, uint16(16))     // bits/sample
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, dataLen)
	binary.Write(&b, binary.LittleEndian, pcm)
	return b.Bytes()
}

// soundPlayer plays pongs on a background goroutine. If a pong is already
// queued, extra requests are dropped so a subnet sweep doesn't turn into
// a ten-minute pong symphony.
type soundPlayer struct {
	mu      sync.Mutex
	enabled bool
	closed  bool
	ch      chan struct{}
	done    chan struct{}
}

func newSoundPlayer(enabled bool) *soundPlayer {
	s := &soundPlayer{enabled: enabled}
	if !enabled {
		return s
	}
	if err := initSound(); err != nil {
		s.enabled = false
		return s
	}
	s.ch = make(chan struct{}, 1)
	s.done = make(chan struct{})
	go func() {
		for range s.ch {
			playPong()
		}
		close(s.done)
	}()
	return s
}

func (s *soundPlayer) Play() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled || s.closed {
		return
	}
	select {
	case s.ch <- struct{}{}:
	default:
	}
}

// Close lets the last queued pong finish playing.
func (s *soundPlayer) Close() {
	s.mu.Lock()
	if !s.enabled || s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.ch)
	s.mu.Unlock()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}
