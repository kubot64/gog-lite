package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"golang.org/x/oauth2"
	gapi "google.golang.org/api/googleapi"

	"github.com/kubot64/gog-lite/internal/googleapi"
	"github.com/kubot64/gog-lite/internal/output"
)

// isAuthErr checks if err is an *googleapi.AuthRequiredError and sets target.
func isAuthErr(err error, target **googleapi.AuthRequiredError) bool {
	return errors.As(err, target)
}

// invalidGrantMessage is shown when the token endpoint rejects a refresh token.
// RetrieveError.Error includes error_description and, when the error code was
// not parsed, the response body. Those fields can echo the refresh token, so
// they are never copied into the CLI error.
const invalidGrantMessage = "refresh token rejected (invalid_grant); run: gog-lite auth login"

// writeGoogleAPIError maps common Google API HTTP errors to exit codes.
func writeGoogleAPIError(defaultCode string, err error) error {
	if isInvalidGrant(err) {
		return output.WriteError(output.ExitCodeAuth, "auth_required", invalidGrantMessage)
	}

	var apiErr *gapi.Error
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case http.StatusUnauthorized:
			// Google rejected the token. Agents treat this like a missing token and log in again.
			return output.WriteError(output.ExitCodeAuth, "auth_required", err.Error())
		case http.StatusNotFound:
			return output.WriteError(output.ExitCodeNotFound, "not_found", err.Error())
		case http.StatusForbidden:
			return output.WriteError(output.ExitCodePermission, "permission_denied", err.Error())
		}
	}

	return output.WriteError(output.ExitCodeError, defaultCode, err.Error())
}

func isInvalidGrant(err error) bool {
	var retrieve *oauth2.RetrieveError
	if !errors.As(err, &retrieve) || retrieve == nil {
		return false
	}
	if retrieve.ErrorCode == "invalid_grant" {
		return true
	}
	// A parsed code other than invalid_grant (invalid_client, and so on) stays a generic API error.
	if retrieve.ErrorCode != "" {
		return false
	}
	// Some token responses are not parsed into ErrorCode. Read only the error
	// code. The rest of the body is not copied into the CLI error.
	return tokenEndpointErrorCode(retrieve.Body) == "invalid_grant"
}

// tokenEndpointErrorCode reads RFC 6749's error field and drops anything that
// is not a short identifier.
func tokenEndpointErrorCode(body []byte) string {
	code := errorCodeFromJSON(body)
	if code == "" {
		code = errorCodeFromForm(body)
	}
	if !isSafeOAuthErrorCode(code) {
		return ""
	}
	return code
}

func errorCodeFromJSON(body []byte) string {
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Error)
}

func errorCodeFromForm(body []byte) string {
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(vals.Get("error"))
}

func isSafeOAuthErrorCode(code string) bool {
	if code == "" || len(code) > 64 {
		return false
	}
	for _, r := range code {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			continue
		}
		return false
	}
	return true
}
