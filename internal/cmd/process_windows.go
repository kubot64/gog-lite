//go:build windows

package cmd

import (
	"errors"
	"os"
	"syscall"
)

// errProcessNotFound is ERROR_INVALID_PARAMETER (87). OpenProcess returns it
// when pid does not exist. The syscall package keeps that name unexported.
const errProcessNotFound = syscall.Errno(87)

// processAlive reports whether pid is still running.
// Windows has no signal 0, and flock is unavailable (lockfile_noflock.go),
// so a dead lock is removed and recreated. OpenProcess is the probe.
// ERROR_INVALID_PARAMETER means the PID is gone. Access denied, and any
// other failure, leave the lock in place.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	proc, err := os.FindProcess(pid)
	if err != nil || proc == nil {
		return !errors.Is(err, errProcessNotFound)
	}
	defer func() { _ = proc.Release() }()

	return true
}
