//go:build unix

package cmd

import "syscall"

// lockNoFollow makes open fail when the lock path is a symlink.
const lockNoFollow = syscall.O_NOFOLLOW
