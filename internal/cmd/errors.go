package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"golang.org/x/oauth2"
	gapi "google.golang.org/api/googleapi"

	"github.com/kubot64/gog-lite/internal/googleapi"
	"github.com/kubot64/gog-lite/internal/googleauth"
	"github.com/kubot64/gog-lite/internal/output"
)

// isAuthErr checks if err is an *googleapi.AuthRequiredError and sets target.
func isAuthErr(err error, target **googleapi.AuthRequiredError) bool {
	return errors.As(err, target)
}

// invalidGrantMessage is shown when a refresh is rejected with invalid_grant.
// tokenEndpointFailureMessage is shown for every other token endpoint failure,
// and for every auth login code-exchange failure that came from the token
// endpoint (including invalid_grant). RetrieveError.Error includes
// error_description and, when the error code was not parsed, the response
// body. Those fields can echo secrets, so they are never copied into the CLI
// error. The message is the fixed sentence plus a short error code when one
// can be read.
const (
	invalidGrantMessage         = "refresh token rejected (invalid_grant); run: gog-lite auth login"
	tokenEndpointFailureMessage = "token endpoint request failed"
)

// writeGoogleAPIError maps common Google API HTTP errors to exit codes.
func writeGoogleAPIError(defaultCode string, err error) error {
	if isInvalidGrant(err) {
		return output.WriteError(output.ExitCodeAuth, "auth_required", invalidGrantMessage)
	}
	if retrieve, ok := retrieveError(err); ok {
		// invalid_grant is handled above. Other token endpoint failures stay a
		// generic API error (exit code 1, the caller's code) but do not copy
		// error_description or the response body.
		return output.WriteError(output.ExitCodeError, defaultCode, formatTokenEndpointFailure(retrieve))
	}

	var apiErr *gapi.Error
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case http.StatusUnauthorized:
			// Google rejected the token. Agents treat this like a missing token and log in again.
			// A parsed message is kept. An unparsed body is not: it can be an
			// OAuth error document that echoes a token in error_description.
			return output.WriteError(output.ExitCodeAuth, "auth_required", formatUnauthorizedAPIError(err, apiErr))
		case http.StatusNotFound:
			return output.WriteError(output.ExitCodeNotFound, "not_found", err.Error())
		case http.StatusForbidden:
			return output.WriteError(output.ExitCodePermission, "permission_denied", err.Error())
		}
	}

	return output.WriteError(output.ExitCodeError, defaultCode, err.Error())
}

func isInvalidGrant(err error) bool {
	retrieve, ok := retrieveError(err)
	if !ok {
		return false
	}
	return shortTokenEndpointCode(retrieve) == "invalid_grant"
}

func retrieveError(err error) (*oauth2.RetrieveError, bool) {
	var retrieve *oauth2.RetrieveError
	if !errors.As(err, &retrieve) || retrieve == nil {
		return nil, false
	}
	return retrieve, true
}

// formatTokenEndpointFailure is the stderr text for a token endpoint failure
// that stays a generic error. It never includes error_description, error_uri,
// or the body. Refresh uses it for everything except invalid_grant. auth login
// uses it for every token endpoint failure, including invalid_grant.
func formatTokenEndpointFailure(retrieve *oauth2.RetrieveError) string {
	code := shortTokenEndpointCode(retrieve)
	if code == "" {
		return tokenEndpointFailureMessage
	}
	return tokenEndpointFailureMessage + " (" + code + ")"
}

// formatUnauthorizedAPIError is the stderr text for a Google API HTTP 401.
// googleapi.Error.Error copies the raw body when the response has no parsed
// message. That body is dropped. A parsed message is left as err.Error().
func formatUnauthorizedAPIError(err error, apiErr *gapi.Error) string {
	if apiErr != nil && apiErr.Message == "" && len(apiErr.Errors) == 0 && apiErr.Body != "" {
		return fmt.Sprintf("googleapi: got HTTP response code %d", apiErr.Code)
	}
	return err.Error()
}

// authExchangeErrorMessage is the stderr text for auth login step 2.
// A token endpoint response uses formatTokenEndpointFailure, which keeps only
// shortTokenEndpointCode. invalid_grant is not rewritten to auth_required.
// A non-200 userinfo response uses UserInfoFailureMessage, which keeps only
// the HTTP status. Other failures (state mismatch, transport) keep err.Error().
func authExchangeErrorMessage(err error) string {
	if retrieve, ok := retrieveError(err); ok {
		return formatTokenEndpointFailure(retrieve)
	}
	if msg, ok := googleauth.UserInfoFailureMessage(err); ok {
		return msg
	}
	return err.Error()
}

// shortTokenEndpointCode returns RFC 6749's error field when it is a short
// identifier. A parsed ErrorCode wins, which is the same field oauth2 already
// read. When that field is empty, the body is read with tokenEndpointErrorCode.
// Anything that is not a short identifier is dropped, including a parsed code
// that contains spaces or other characters. error_description and the rest of
// the body are never returned.
func shortTokenEndpointCode(retrieve *oauth2.RetrieveError) string {
	if isSafeOAuthErrorCode(retrieve.ErrorCode) {
		return retrieve.ErrorCode
	}
	if retrieve.ErrorCode != "" {
		return ""
	}
	return tokenEndpointErrorCode(retrieve.Body)
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
