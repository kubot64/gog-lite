//go:build !unix

package googleauth

import (
	"os/exec"
	"syscall"
)

func callbackHolderSysProcAttr() *syscall.SysProcAttr {
	return nil
}

func holderExited(_ *exec.Cmd) bool {
	return false
}
