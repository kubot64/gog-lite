//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package cmd

import "os"

// hasFileLock is false where the process cannot take an advisory flock.
// Those builds keep the previous remove-and-retry steal.
const hasFileLock = false

func lockFile(_ *os.File, _ bool) error { return nil }

func unlockFile(_ *os.File) error { return nil }
