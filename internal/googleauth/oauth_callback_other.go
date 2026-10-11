//go:build !unix

package googleauth

import (
	"os/exec"
	"syscall"
)

// callbackHoldSupported is false where ExtraFiles cannot hand a listening
// socket to another process. Windows StartProcess returns EWINDOWS when
// more than the three standard files are inherited, so net.FileListener
// never sees the port. Tests may set it. Production leaves the default.
var callbackHoldSupported = false

func callbackHolderSysProcAttr() *syscall.SysProcAttr {
	return nil
}

func holderExited(_ *exec.Cmd) bool {
	return false
}
