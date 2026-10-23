//go:build !unix

package googleauth

import "testing"

func TestStep1_PlatformDefaultDoesNotHoldCallbackPort(t *testing.T) {
	if callbackHoldSupported {
		t.Fatal("this platform must not hold the callback port")
	}

	exerciseStep1WithoutCallbackHolder(t)
}
