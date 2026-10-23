//go:build windows

package cmd

import (
	"os"
	"testing"
)

func TestProcessAlive_WindowsOpenProcess(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Fatal("current process looks dead")
	}
	// Windows process IDs are multiples of 4, so this one cannot be assigned.
	if processAlive(0x0FFFFFFF) {
		t.Fatal("impossible pid looks alive")
	}
}
