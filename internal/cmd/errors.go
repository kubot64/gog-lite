package cmd

import (
	"errors"
	"net/http"

	"github.com/kubot64/gog-lite/internal/googleapi"
	"github.com/kubot64/gog-lite/internal/output"
	gapi "google.golang.org/api/googleapi"
)

// isAuthErr checks if err is an *googleapi.AuthRequiredError and sets target.
func isAuthErr(err error, target **googleapi.AuthRequiredError) bool {
	return errors.As(err, target)
}

// writeGoogleAPIError maps common Google API HTTP errors to exit codes.
func writeGoogleAPIError(defaultCode string, err error) error {
	var apiErr *gapi.Error
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case http.StatusNotFound:
			return output.WriteError(output.ExitCodeNotFound, "not_found", err.Error())
		case http.StatusForbidden:
			return output.WriteError(output.ExitCodePermission, "permission_denied", err.Error())
		}
	}

	return output.WriteError(output.ExitCodeError, defaultCode, err.Error())
}
