//go:build !unix

package cmd

import "os"

// abandonLockedProcess stops this process without running defers.
// Windows has no SIGKILL. os.Exit also skips defers, so the lock file
// keeps the PID.
func abandonLockedProcess() {
	os.Exit(1)
}
