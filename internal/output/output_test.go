package output_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/kubot64/gog-lite/internal/output"
)

func TestWriteJSON_NoHTMLEscape(t *testing.T) {
	var buf bytes.Buffer
	err := output.WriteJSON(&buf, map[string]string{"url": "https://example.com/a?b=1&c=2<3"})
	if err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, `\u003c`) {
		t.Errorf("URL was HTML-escaped: %s", got)
	}
	if !strings.Contains(got, "https://example.com/a?b=1&c=2<3") {
		t.Errorf("URL not preserved in output: %s", got)
	}
}

func TestWriteJSON_Indented(t *testing.T) {
	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "  ") {
		t.Errorf("expected indented JSON, got: %s", buf.String())
	}
}

func TestExitCode_Nil(t *testing.T) {
	if code := output.ExitCode(nil); code != output.ExitCodeOK {
		t.Errorf("want %d, got %d", output.ExitCodeOK, code)
	}
}

func TestExitCode_ExitCodeErr(t *testing.T) {
	err := output.NewError(output.ExitCodeAuth, fmt.Errorf("auth failed"))
	if code := output.ExitCode(err); code != output.ExitCodeAuth {
		t.Errorf("want %d, got %d", output.ExitCodeAuth, code)
	}
}

func TestExitCode_WrappedExitCodeErr(t *testing.T) {
	inner := output.NewError(output.ExitCodeNotFound, fmt.Errorf("not found"))
	wrapped := fmt.Errorf("outer: %w", inner)
	if code := output.ExitCode(wrapped); code != output.ExitCodeNotFound {
		t.Errorf("want %d, got %d", output.ExitCodeNotFound, code)
	}
}

func TestExitCode_PlainError(t *testing.T) {
	if code := output.ExitCode(fmt.Errorf("some error")); code != output.ExitCodeError {
		t.Errorf("want %d, got %d", output.ExitCodeError, code)
	}
}

func TestExitCodeErr_ErrorString(t *testing.T) {
	err := output.NewError(output.ExitCodeAuth, fmt.Errorf("auth failed"))
	if err.Error() != "auth failed" {
		t.Errorf("want %q, got %q", "auth failed", err.Error())
	}
}

func TestExitCodeErr_Unwrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	err := output.NewError(output.ExitCodeError, inner)
	if err.Unwrap() != inner {
		t.Errorf("Unwrap() should return the inner error")
	}
}
