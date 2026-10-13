//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

var procSetConsoleOutputCP = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetConsoleOutputCP")

// enableTerminal switches the console to UTF-8 (for emoji) and turns on
// ANSI color support. Returns false when stdout isn't a console.
func enableTerminal() bool {
	h := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	procSetConsoleOutputCP.Call(65001)
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
