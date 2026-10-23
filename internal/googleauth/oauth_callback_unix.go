//go:build unix

package googleauth

import (
	"os/exec"
	"syscall"
)

// callbackHoldSupported is true where the listening socket can be passed to
// another process. Tests may set it. Production leaves the default.
var callbackHoldSupported = true

func callbackHolderSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

func holderExited(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return true
	}

	return cmd.Process.Signal(syscall.Signal(0)) != nil
}
