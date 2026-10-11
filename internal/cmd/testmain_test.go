package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if strings.TrimSpace(os.Getenv("GOG_LITE_KEYRING_BACKEND")) == "" {
		_ = os.Setenv("GOG_LITE_KEYRING_BACKEND", "file")
	}
	if strings.TrimSpace(os.Getenv("GOG_LITE_KEYRING_PASSWORD")) == "" {
		_ = os.Setenv("GOG_LITE_KEYRING_PASSWORD", "test-password")
	}

	os.Exit(m.Run())
}
