package googleauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const tokenRevokeEndpointDefault = "https://oauth2.googleapis.com/revoke"

// tokenRevokeEndpoint is the Google OAuth revocation URL. Tests point it at a local server.
var tokenRevokeEndpoint = tokenRevokeEndpointDefault

var (
	// ErrAlreadyRevoked means Google returned HTTP 400 with error=invalid_token.
	// The refresh token is already unusable.
	ErrAlreadyRevoked = errors.New("token already revoked")
	// ErrRevokeUnavailable means the revoke call did not reach a definitive Google answer
	// (transport error or HTTP 5xx). The token may still be valid.
	ErrRevokeUnavailable = errors.New("token revoke unavailable")
	// ErrRevokeRejected means Google refused the revoke for a reason other than invalid_token.
	ErrRevokeRejected = errors.New("token revoke rejected")
	// ErrEmptyRefreshToken means there is no refresh token to send.
	ErrEmptyRefreshToken = errors.New("refresh token is empty")
)

const (
	revokeHTTPTimeout   = 30 * time.Second
	revokeResponseLimit = 1 << 16
)

// RevokeRefreshToken asks Google to revoke refreshToken.
// A nil error means HTTP 200. ErrAlreadyRevoked means HTTP 400 invalid_token.
// The token value is never included in errors.
func RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return ErrEmptyRefreshToken
	}

	form := url.Values{}
	form.Set("token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenRevokeEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{
		Timeout: revokeHTTPTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRevokeUnavailable, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, revokeResponseLimit))
	return classifyRevokeResponse(resp.StatusCode, body)
}

func classifyRevokeResponse(status int, body []byte) error {
	if status == http.StatusOK {
		return nil
	}
	if status == http.StatusBadRequest && revokeErrorCode(body) == "invalid_token" {
		return ErrAlreadyRevoked
	}
	if status >= 500 && status <= 599 {
		return fmt.Errorf("%w: HTTP %d", ErrRevokeUnavailable, status)
	}
	if code := revokeErrorCode(body); code != "" {
		return fmt.Errorf("%w: HTTP %d %s", ErrRevokeRejected, status, code)
	}
	return fmt.Errorf("%w: HTTP %d", ErrRevokeRejected, status)
}

// revokeErrorCode returns Google's error code when it is a short token-safe identifier.
// Response bodies are not copied into errors, because they may echo the refresh token.
func revokeErrorCode(body []byte) string {
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	code := strings.TrimSpace(payload.Error)
	if code == "" || len(code) > 64 || !isSafeErrorCode(code) {
		return ""
	}
	return code
}

func isSafeErrorCode(code string) bool {
	for _, r := range code {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			continue
		}
		return false
	}
	return true
}
