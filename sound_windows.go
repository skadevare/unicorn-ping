//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	winmm         = windows.NewLazySystemDLL("winmm.dll")
	procPlaySound = winmm.NewProc("PlaySoundW")
	pongData      []byte
)

const (
	sndSync      = 0x0000
	sndNoDefault = 0x0002
	sndMemory    = 0x0004
)

func initSound() error {
	pongData = pongWAV()
	return procPlaySound.Find()
}

func playPong() {
	procPlaySound.Call(uintptr(unsafe.Pointer(&pongData[0])), 0, sndSync|sndNoDefault|sndMemory)
}
