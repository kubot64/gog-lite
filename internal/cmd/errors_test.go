package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/morikubo-takashi/gog-lite/internal/output"
	gapi "google.golang.org/api/googleapi"
)

func TestWriteGoogleAPIError_NotFound(t *testing.T) {
	err := withMutedStderr(t, func() error {
		return writeGoogleAPIError("get_error", &gapi.Error{Code: 404, Message: "not found"})
	})
	if got := output.ExitCode(err); got != output.ExitCodeNotFound {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeNotFound)
	}
}

func TestWriteGoogleAPIError_ForbiddenWrapped(t *testing.T) {
	err := withMutedStderr(t, func() error {
		base := &gapi.Error{Code: 403, Message: "forbidden"}
		return writeGoogleAPIError("get_error", fmt.Errorf("wrapped: %w", base))
	})
	if got := output.ExitCode(err); got != output.ExitCodePermission {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodePermission)
	}
}

func TestWriteGoogleAPIError_OtherGoogleAPIError(t *testing.T) {
	err := withMutedStderr(t, func() error {
		return writeGoogleAPIError("get_error", &gapi.Error{Code: 500, Message: "internal"})
	})
	if got := output.ExitCode(err); got != output.ExitCodeError {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeError)
	}
}

func TestWriteGoogleAPIError_NonGoogleError(t *testing.T) {
	err := withMutedStderr(t, func() error {
		return writeGoogleAPIError("get_error", errors.New("plain error"))
	})
	if got := output.ExitCode(err); got != output.ExitCodeError {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeError)
	}
}

func withMutedStderr(t *testing.T, fn func() error) error {
	t.Helper()

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}

	os.Stderr = w
	defer func() { os.Stderr = old }()

	runErr := fn()

	_ = w.Close()
	_, _ = io.Copy(io.Discard, r)
	_ = r.Close()

	return runErr
}
