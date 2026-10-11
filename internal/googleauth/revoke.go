package googleauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const tokenRevokeEndpointDefault = "https://oauth2.googleapis.com/revoke"

// tokenRevokeEndpoint is the Google OAuth revocation URL. Tests point it at a local server.
var tokenRevokeEndpoint = tokenRevokeEndpointDefault

var (
	errEmptyRefreshToken = errors.New("refresh token is empty")
	errRevokeRejected    = errors.New("token revoke rejected")
)

const revokeHTTPTimeout = 30 * time.Second

// RevokeRefreshToken asks Google to revoke refreshToken.
// Only HTTP 200 is success. The token value is never included in errors.
func RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return errEmptyRefreshToken
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
		return fmt.Errorf("revoke request: %w", err)
	}
	defer resp.Body.Close()

	// Drop the body. It is not part of the error text, so a response that echoes the token stays out of logs.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d", errRevokeRejected, resp.StatusCode)
	}

	return nil
}
