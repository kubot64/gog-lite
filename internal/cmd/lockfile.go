package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// lockAcquireTimeout is how long a waiter polls before failing.
// Tests shorten it. Production code should leave the default in place.
var lockAcquireTimeout = 5 * time.Second

const (
	lockRetryDelay = 10 * time.Millisecond
	// lockStaleAfter applies only to lock files with no PID. Current binaries
	// write a PID and are preempted only after that process is gone.
	lockStaleAfter = 30 * time.Second
)

func withFileLock(targetPath string, fn func() error) error {
	lockPath := targetPath + ".lock"
	if err := acquireLock(lockPath, time.Now().Add(lockAcquireTimeout)); err != nil {
		return err
	}
	defer func() { _ = os.Remove(lockPath) }()

	return fn()
}

func acquireLock(lockPath string, deadline time.Time) error {
	for {
		err := createLock(lockPath)
		if err == nil {
			return nil
		}

		if errors.Is(err, syscall.ELOOP) {
			return fmt.Errorf("acquire lock %s: lock file is a symlink", lockPath)
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("acquire lock %s: %w", lockPath, err)
		}

		stealable, stealErr := stealableLock(lockPath)
		if stealErr != nil {
			return fmt.Errorf("acquire lock %s: %w", lockPath, stealErr)
		}
		if stealable {
			if removeErr := os.Remove(lockPath); removeErr == nil || os.IsNotExist(removeErr) {
				if time.Now().After(deadline) {
					return fmt.Errorf("acquire lock %s: timeout", lockPath)
				}
				continue
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("acquire lock %s: timeout", lockPath)
		}

		time.Sleep(lockRetryDelay)
	}
}

func createLock(lockPath string) error {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY|lockNoFollow, 0o600)
	if err != nil {
		return err
	}

	_, writeErr := f.WriteString(strconv.Itoa(os.Getpid()) + "\n")
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(lockPath)
		if writeErr != nil {
			return writeErr
		}
		if syncErr != nil {
			return syncErr
		}

		return closeErr
	}

	return nil
}

// stealableLock reports whether the lock file can be removed.
// A lock whose PID is still running is kept, however old the file is.
func stealableLock(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("lock file is a symlink")
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("lock file is not a regular file")
	}

	pid, ok := readLockPID(path)
	if ok {
		return !processAlive(pid), nil
	}

	return time.Since(info.ModTime()) > lockStaleAfter, nil
}

func readLockPID(path string) (int, bool) {
	f, err := os.OpenFile(path, os.O_RDONLY|lockNoFollow, 0)
	if err != nil {
		return 0, false
	}
	defer func() { _ = f.Close() }()

	b, err := io.ReadAll(io.LimitReader(f, 64))
	if err != nil {
		return 0, false
	}

	line := strings.TrimSpace(string(b))
	pid, err := strconv.Atoi(line)
	if err != nil || pid <= 0 {
		return 0, false
	}

	return pid, true
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	proc, err := os.FindProcess(pid)
	if err != nil || proc == nil {
		// Unknown probe result. Leave the lock in place.
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
