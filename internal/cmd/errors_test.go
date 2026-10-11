package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	gapi "google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/kubot64/gog-lite/internal/googleapi"
	"github.com/kubot64/gog-lite/internal/output"
	"github.com/kubot64/gog-lite/internal/secrets"
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

func TestWriteGoogleAPIError_BadRequestStaysGeneric(t *testing.T) {
	err := withMutedStderr(t, func() error {
		return writeGoogleAPIError("get_error", &gapi.Error{Code: http.StatusBadRequest, Message: "invalid_grant"})
	})
	if got := output.ExitCode(err); got != output.ExitCodeError {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeError)
	}
}

func TestWriteGoogleAPIError_InvalidGrant(t *testing.T) {
	const refreshToken = "super-secret-refresh-token"
	retrieve := &oauth2.RetrieveError{
		Response:         &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request"},
		Body:             []byte(`{"error":"invalid_grant","error_description":"echo ` + refreshToken + `"}`),
		ErrorCode:        "invalid_grant",
		ErrorDescription: "echo " + refreshToken,
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("labels_error", fmt.Errorf("round trip: %w", retrieve))
	})
	assertInvalidGrant(t, runErr, stderr, refreshToken)
}

func TestWriteGoogleAPIError_InvalidGrantUnparsedJSON(t *testing.T) {
	const refreshToken = "super-secret-refresh-token"
	retrieve := &oauth2.RetrieveError{
		Response: &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request"},
		Body:     []byte(`{"error":"invalid_grant","error_description":"echo ` + refreshToken + `","refresh_token":"` + refreshToken + `"}`),
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("get_error", retrieve)
	})
	assertInvalidGrant(t, runErr, stderr, refreshToken)
}

func TestWriteGoogleAPIError_InvalidGrantUnparsedForm(t *testing.T) {
	const refreshToken = "super-secret-refresh-token"
	retrieve := &oauth2.RetrieveError{
		Response: &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request"},
		Body:     []byte("error=invalid_grant&error_description=echo-" + refreshToken),
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("get_error", fmt.Errorf("round trip: %w", retrieve))
	})
	assertInvalidGrant(t, runErr, stderr, refreshToken)
}

func TestWriteGoogleAPIError_InvalidGrantFromTokenEndpoint(t *testing.T) {
	const refreshToken = "super-secret-refresh-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			t.Errorf("parse form: %v", err)
		}
		if vals.Get("refresh_token") != refreshToken {
			t.Errorf("refresh_token = %q", vals.Get("refresh_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"echo `+refreshToken+`"}`)
	}))
	t.Cleanup(server.Close)

	cfg := oauth2.Config{
		ClientID:     "dummy-id",
		ClientSecret: "dummy-secret",
		Endpoint: oauth2.Endpoint{
			TokenURL:  server.URL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, server.Client())
	_, err := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken}).Token()
	if err == nil {
		t.Fatal("expected token error")
	}
	var retrieve *oauth2.RetrieveError
	if !errors.As(err, &retrieve) || retrieve.ErrorCode != "invalid_grant" {
		t.Fatalf("error = %v, want invalid_grant RetrieveError", err)
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = writeGoogleAPIError("labels_error", fmt.Errorf("round trip: %w", err))
	})
	assertInvalidGrant(t, runErr, stderr, refreshToken)
}

func TestWriteGoogleAPIError_OtherRetrieveError(t *testing.T) {
	retrieve := &oauth2.RetrieveError{
		Response:         &http.Response{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized"},
		Body:             []byte(`{"error":"invalid_client","error_description":"The OAuth client was not found."}`),
		ErrorCode:        "invalid_client",
		ErrorDescription: "The OAuth client was not found.",
	}
	err := withMutedStderr(t, func() error {
		return writeGoogleAPIError("get_error", fmt.Errorf("round trip: %w", retrieve))
	})
	if got := output.ExitCode(err); got != output.ExitCodeError {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeError)
	}
}

func TestWriteGoogleAPIError_UnparsedBodyIsNotInvalidGrant(t *testing.T) {
	retrieve := &oauth2.RetrieveError{
		Response: &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request"},
		Body:     []byte("not-json mentions invalid_grant"),
	}
	err := withMutedStderr(t, func() error {
		return writeGoogleAPIError("get_error", retrieve)
	})
	if got := output.ExitCode(err); got != output.ExitCodeError {
		t.Errorf("exit code = %d, want %d", got, output.ExitCodeError)
	}
}

func TestExecute_InvalidGrantIsAuthRequired(t *testing.T) {
	const refreshToken = "super-secret-refresh-token"
	const account = "test@example.com"

	setupFileKeyring(t)
	store := openTestStore(t)
	if err := store.SetToken(account, secrets.Token{
		Email:        account,
		RefreshToken: refreshToken,
		Services:     []string{"gmail"},
	}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	called := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case called <- struct{}{}:
		default:
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			t.Errorf("parse form: %v", err)
		}
		if vals.Get("refresh_token") != refreshToken {
			t.Errorf("refresh_token = %q", vals.Get("refresh_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"echo `+refreshToken+`","refresh_token":"`+refreshToken+`"}`)
	}))
	t.Cleanup(server.Close)

	restore := googleapi.SetOAuthEndpointForTest(oauth2.Endpoint{
		TokenURL:  server.URL,
		AuthStyle: oauth2.AuthStyleInParams,
	})
	t.Cleanup(restore)

	prevArgs := os.Args
	t.Cleanup(func() { os.Args = prevArgs })
	os.Args = []string{"gog-lite", "gmail", "labels", "--account", account}

	var runErr error
	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			runErr = Execute(context.Background(), "dev")
		})
	})
	select {
	case <-called:
	default:
		t.Fatal("token endpoint was not called")
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	assertInvalidGrant(t, runErr, stderr, refreshToken)
}

func assertInvalidGrant(t *testing.T, runErr error, stderr, secret string) {
	t.Helper()
	if got := output.ExitCode(runErr); got != output.ExitCodeAuth {
		t.Fatalf("exit code = %d, want %d (err=%v)", got, output.ExitCodeAuth, runErr)
	}
	assertErrorCode(t, stderr, "auth_required")

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
		t.Fatalf("decode stderr %q: %v", stderr, err)
	}
	if payload.Error != invalidGrantMessage {
		t.Errorf("error = %q, want %q", payload.Error, invalidGrantMessage)
	}
	if strings.Contains(stderr, secret) {
		t.Fatalf("stderr leaked refresh token: %q", stderr)
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
