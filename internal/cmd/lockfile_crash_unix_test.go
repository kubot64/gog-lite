//go:build unix

package cmd

import (
	"os"
	"syscall"
)

// abandonLockedProcess stops this process without running defers, so the
// lock file keeps the PID the way a crash would.
func abandonLockedProcess() {
	_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
}
