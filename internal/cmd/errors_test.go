package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/kubot64/gog-lite/internal/output"

	"google.golang.org/api/gmail/v1"
	gapi "google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func TestWriteGoogleAPIError_Unauthorized(t *testing.T) {
	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("get_error", &gapi.Error{Code: http.StatusUnauthorized, Message: "Invalid Credentials"})
	})
	if got := output.ExitCode(runErr); got != output.ExitCodeAuth {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeAuth)
	}
	assertErrorCode(t, stderr, "auth_required")
}

func TestWriteGoogleAPIError_UnauthorizedWrapped(t *testing.T) {
	var runErr error
	stderr := captureStderr(t, func() {
		base := &gapi.Error{Code: http.StatusUnauthorized, Message: "Invalid Credentials"}
		runErr = writeGoogleAPIError("labels_error", fmt.Errorf("gmail labels: %w", base))
	})
	if got := output.ExitCode(runErr); got != output.ExitCodeAuth {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeAuth)
	}
	assertErrorCode(t, stderr, "auth_required")
}

func TestWriteGoogleAPIError_UnauthorizedFromClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":401,"message":"Invalid Credentials"}}`))
	}))
	t.Cleanup(server.Close)

	svc, err := gmail.NewService(context.Background(),
		option.WithEndpoint(server.URL+"/"),
		option.WithoutAuthentication(),
		option.WithHTTPClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("gmail.NewService: %v", err)
	}

	_, err = svc.Users.Labels.List("me").Do()
	if err == nil {
		t.Fatal("expected gmail client error, got nil")
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("labels_error", fmt.Errorf("gmail labels: %w", err))
	})
	if got := output.ExitCode(runErr); got != output.ExitCodeAuth {
		t.Fatalf("exit code = %d, want %d (err=%v)", got, output.ExitCodeAuth, err)
	}
	assertErrorCode(t, stderr, "auth_required")
}

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

func assertErrorCode(t *testing.T, stderr, want string) {
	t.Helper()

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
		t.Fatalf("decode stderr %q: %v", stderr, err)
	}
	if payload.Code != want {
		t.Errorf("code = %q, want %q", payload.Code, want)
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
