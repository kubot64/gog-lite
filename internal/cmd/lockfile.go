package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	lockAcquireTimeout = 5 * time.Second
	lockRetryDelay     = 10 * time.Millisecond
	lockStaleAfter     = 30 * time.Second
)

func withFileLock(targetPath string, fn func() error) error {
	lockPath := targetPath + ".lock"
	deadline := time.Now().Add(lockAcquireTimeout)

	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			defer func() { _ = os.Remove(lockPath) }()

			return fn()
		}

		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("acquire lock %s: %w", lockPath, err)
		}

		if stale, staleErr := staleLock(lockPath); staleErr == nil && stale {
			if removeErr := os.Remove(lockPath); removeErr == nil || os.IsNotExist(removeErr) {
				continue
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("acquire lock %s: timeout", lockPath)
		}

		time.Sleep(lockRetryDelay)
	}
}

func staleLock(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}

	return time.Since(info.ModTime()) > lockStaleAfter, nil
}
