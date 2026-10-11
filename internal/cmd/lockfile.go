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

// errLockHeld means another opener already holds the exclusive lock.
var errLockHeld = errors.New("lock held")

func withFileLock(targetPath string, fn func() error) error {
	lockPath := targetPath + ".lock"
	f, err := acquireLock(lockPath, time.Now().Add(lockAcquireTimeout))
	if err != nil {
		return err
	}
	defer releaseLock(f, lockPath)

	return fn()
}

func acquireLock(lockPath string, deadline time.Time) (*os.File, error) {
	for {
		f, err := createLock(lockPath)
		if err == nil {
			return f, nil
		}

		if errors.Is(err, syscall.ELOOP) {
			return nil, fmt.Errorf("acquire lock %s: lock file is a symlink", lockPath)
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("acquire lock %s: %w", lockPath, err)
		}

		if hasFileLock {
			// Several waiters can all observe one dead lock. Taking the
			// exclusive lock and rewriting that inode, instead of removing
			// the path, stops the loser from deleting the winner's new file.
			f, err = adoptStaleLock(lockPath)
			if err != nil {
				return nil, fmt.Errorf("acquire lock %s: %w", lockPath, err)
			}
			if f != nil {
				return f, nil
			}
		} else {
			removed, removeErr := removeStaleLock(lockPath)
			if removeErr != nil {
				return nil, fmt.Errorf("acquire lock %s: %w", lockPath, removeErr)
			}
			if removed {
				if time.Now().After(deadline) {
					return nil, fmt.Errorf("acquire lock %s: timeout", lockPath)
				}
				continue
			}
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("acquire lock %s: timeout", lockPath)
		}

		time.Sleep(lockRetryDelay)
	}
}

func createLock(lockPath string) (*os.File, error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_RDWR|lockNoFollow, 0o600)
	if err != nil {
		return nil, err
	}

	locked := false
	success := false
	defer func() {
		if success {
			return
		}
		if ok, sameErr := sameLockFile(f, lockPath); sameErr == nil && ok {
			_ = os.Remove(lockPath)
		}
		if locked {
			_ = unlockFile(f)
		}
		_ = f.Close()
	}()

	if hasFileLock {
		// Block until we own the inode we just created. A waiter that opened
		// the empty file first releases it once it sees the file is not stale.
		if err := lockFile(f, false); err != nil {
			return nil, err
		}
		locked = true
	}
	if err := writeLockPID(f); err != nil {
		return nil, err
	}
	if locked {
		ok, err := sameLockFile(f, lockPath)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, os.ErrExist
		}
	}

	success = true
	return f, nil
}

// adoptStaleLock claims a stale lock inode. The returned file stays locked
// until releaseLock. A nil file means the caller should wait and retry.
func adoptStaleLock(lockPath string) (*os.File, error) {
	info, err := os.Lstat(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("lock file is a symlink")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("lock file is not a regular file")
	}

	f, err := os.OpenFile(lockPath, os.O_RDWR|lockNoFollow, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		if errors.Is(err, syscall.ELOOP) {
			return nil, fmt.Errorf("lock file is a symlink")
		}

		return nil, err
	}

	if err := lockFile(f, true); err != nil {
		_ = f.Close()
		if errors.Is(err, errLockHeld) {
			return nil, nil
		}

		return nil, err
	}

	owned := false
	defer func() {
		if owned {
			return
		}
		_ = unlockFile(f)
		_ = f.Close()
	}()

	ok, err := sameLockFile(f, lockPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("lock file is not a regular file")
	}
	b, err := readLockFile(f)
	if err != nil {
		return nil, err
	}
	if !lockContentsStale(st, b) {
		return nil, nil
	}
	if err := writeLockPID(f); err != nil {
		if same, sameErr := sameLockFile(f, lockPath); sameErr == nil && same {
			_ = os.Remove(lockPath)
		}

		return nil, err
	}
	ok, err = sameLockFile(f, lockPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	owned = true
	return f, nil
}

func releaseLock(f *os.File, lockPath string) {
	if f == nil {
		return
	}
	// Remove only the inode we hold. A waiter must not be able to delete a
	// successor's lock by path after this one is already gone.
	if ok, err := sameLockFile(f, lockPath); err == nil && ok {
		_ = os.Remove(lockPath)
	}
	_ = unlockFile(f)
	_ = f.Close()
}

func removeStaleLock(lockPath string) (bool, error) {
	stealable, err := stealableLock(lockPath)
	if err != nil || !stealable {
		return false, err
	}
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return false, nil
	}

	return true, nil
}

func writeLockPID(f *os.File) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.WriteString(strconv.Itoa(os.Getpid()) + "\n"); err != nil {
		return err
	}

	return f.Sync()
}

func readLockFile(f *os.File) ([]byte, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	return io.ReadAll(io.LimitReader(f, 64))
}

func sameLockFile(f *os.File, path string) (bool, error) {
	fi, err := f.Stat()
	if err != nil {
		return false, err
	}
	pi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}
	if pi.Mode()&os.ModeSymlink != 0 || !pi.Mode().IsRegular() {
		return false, nil
	}

	return os.SameFile(fi, pi), nil
}

func lockContentsStale(info os.FileInfo, b []byte) bool {
	if pid, ok := parseLockPID(b); ok {
		return !processAlive(pid)
	}

	return time.Since(info.ModTime()) > lockStaleAfter
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

	// Unreadable contents have no PID. Age decides, as it did before.
	b, _ := readLockPIDBytes(path)

	return lockContentsStale(info, b), nil
}

func readLockPIDBytes(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|lockNoFollow, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	return io.ReadAll(io.LimitReader(f, 64))
}

func parseLockPID(b []byte) (int, bool) {
	line := strings.TrimSpace(string(b))
	pid, err := strconv.Atoi(line)
	if err != nil || pid <= 0 {
		return 0, false
	}

	return pid, true
}
