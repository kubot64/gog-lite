package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	ExitCodeOK         = 0
	ExitCodeError      = 1
	ExitCodeAuth       = 2
	ExitCodeNotFound   = 3
	ExitCodePermission = 4
)

// ExitCodeError wraps an error with a specific exit code.
type ExitCodeErr struct {
	Code int
	Err  error
}

func (e *ExitCodeErr) Error() string {
	return e.Err.Error()
}

func (e *ExitCodeErr) Unwrap() error {
	return e.Err
}

// NewError creates an ExitCodeErr with the given code and error.
func NewError(code int, err error) *ExitCodeErr {
	return &ExitCodeErr{Code: code, Err: err}
}

// ExitCode walks the error chain and returns the exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitCodeOK
	}

	var e *ExitCodeErr
	if errors.As(err, &e) {
		return e.Code
	}

	return ExitCodeError
}

// WriteJSON writes val as indented JSON to w with HTML escaping disabled.
func WriteJSON(w io.Writer, val any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	return enc.Encode(val)
}

// errorPayload is the JSON structure written to stderr on errors.
type errorPayload struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// WriteError writes a JSON error to stderr and returns an ExitCodeErr.
// The returned error should be returned from Run() to trigger os.Exit.
func WriteError(code int, codeStr, msg string) error {
	payload := errorPayload{Error: msg, Code: codeStr}
	enc := json.NewEncoder(os.Stderr)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	_ = enc.Encode(payload)

	return &ExitCodeErr{Code: code, Err: fmt.Errorf("%s", msg)}
}

// Errorf writes a JSON error to stderr and returns an ExitCodeErr.
func Errorf(code int, codeStr, format string, args ...any) error {
	return WriteError(code, codeStr, fmt.Sprintf(format, args...))
}
