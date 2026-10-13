//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

var (
	playerCmd string
	pongPath  string
)

// macOS ships afplay; on Linux we try PulseAudio/PipeWire or ALSA.
func initSound() error {
	for _, c := range []string{"afplay", "paplay", "aplay"} {
		if p, err := exec.LookPath(c); err == nil {
			playerCmd = p
			break
		}
	}
	if playerCmd == "" {
		return errors.New("no audio player found")
	}

	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "unicornping")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	pongPath = filepath.Join(dir, "pong.wav")
	return os.WriteFile(pongPath, pongWAV(), 0o644)
}

func playPong() {
	args := []string{pongPath}
	if filepath.Base(playerCmd) == "aplay" {
		args = append([]string{"-q"}, args...)
	}
	_ = exec.Command(playerCmd, args...).Run()
}
