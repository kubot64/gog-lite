//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package cmd

import (
	"errors"
	"os"
	"syscall"
)

// hasFileLock is true where flock can exclude other processes.
const hasFileLock = true

func lockFile(f *os.File, nonblock bool) error {
	how := syscall.LOCK_EX
	if nonblock {
		how |= syscall.LOCK_NB
	}
	for {
		err := syscall.Flock(int(f.Fd()), how)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err == nil {
			return nil
		}
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return errLockHeld
		}

		return err
	}
}

func unlockFile(f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		if errors.Is(err, syscall.EINTR) {
			continue
		}

		return err
	}
}
