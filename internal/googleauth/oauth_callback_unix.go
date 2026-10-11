//go:build unix

package googleauth

import (
	"os/exec"
	"syscall"
)

func callbackHolderSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

func holderExited(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return true
	}

	return cmd.Process.Signal(syscall.Signal(0)) != nil
}
