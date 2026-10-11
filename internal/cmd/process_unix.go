//go:build unix

package cmd

import (
	"errors"
	"os"
	"syscall"
)

// processAlive reports whether pid is still running.
// An unknown probe result leaves the lock in place.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	proc, err := os.FindProcess(pid)
	if err != nil || proc == nil {
		return true
	}
	// Linux FindProcess opens a pidfd. Close it after the probe.
	defer func() { _ = proc.Release() }()

	err = proc.Signal(syscall.Signal(0))
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true
	}
	// Go reports a missing process as ErrProcessDone (ESRCH is converted).
	if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
		return false
	}

	return true
}
