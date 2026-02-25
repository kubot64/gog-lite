package cmd

import (
	"errors"

	"github.com/morikubo-takashi/gog-lite/internal/googleapi"
)

// isAuthErr checks if err is an *googleapi.AuthRequiredError and sets target.
func isAuthErr(err error, target **googleapi.AuthRequiredError) bool {
	return errors.As(err, target)
}
