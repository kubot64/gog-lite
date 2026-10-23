//go:build !unix && !windows

package cmd

// processAlive cannot probe a process here. Leave the lock in place.
func processAlive(pid int) bool {
	return pid > 0
}
