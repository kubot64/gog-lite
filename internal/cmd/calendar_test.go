package cmd

import (
	"testing"

	"google.golang.org/api/calendar/v3"
)

func TestValidateRFC3339_Valid(t *testing.T) {
	for _, tt := range []string{
		"2026-03-01T10:00:00Z",
		"2026-03-01T10:00:00+09:00",
		"2026-03-01T00:00:00-05:30",
		"2000-01-01T00:00:00Z",
	} {
		if err := validateRFC3339("--start", tt); err != nil {
			t.Errorf("validateRFC3339(%q): unexpected error: %v", tt, err)
		}
	}
}

func TestValidateRFC3339_Invalid(t *testing.T) {
	for _, tt := range []string{
		"",
		"2026-03-01",            // date only, no time
		"2026-03-01T10:00:00",   // no timezone
		"not-a-date",
		"2026/03/01T10:00:00Z",  // wrong separator
	} {
		if err := validateRFC3339("--start", tt); err == nil {
			t.Errorf("validateRFC3339(%q): expected error, got nil", tt)
		}
	}
}

func TestValidateRFC3339_EmptyIsRequired(t *testing.T) {
	err := validateRFC3339("--start", "")
	if err == nil {
		t.Error("expected error for empty string, got nil")
	}
}

func TestValidateRFC3339Optional_Empty(t *testing.T) {
	if err := validateRFC3339Optional("--from", ""); err != nil {
		t.Errorf("expected nil for empty string, got: %v", err)
	}
}

func TestValidateRFC3339Optional_Valid(t *testing.T) {
	if err := validateRFC3339Optional("--from", "2026-01-01T00:00:00Z"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateRFC3339Optional_Invalid(t *testing.T) {
	if err := validateRFC3339Optional("--from", "2026-01-01"); err == nil {
		t.Error("expected error for date-only string, got nil")
	}
}

func TestEventTimeString_DateTime(t *testing.T) {
	edt := &calendar.EventDateTime{DateTime: "2026-03-01T10:00:00Z"}
	if got := eventTimeString(edt); got != "2026-03-01T10:00:00Z" {
		t.Errorf("got %q, want %q", got, "2026-03-01T10:00:00Z")
	}
}

func TestEventTimeString_DateOnly(t *testing.T) {
	edt := &calendar.EventDateTime{Date: "2026-03-01"}
	if got := eventTimeString(edt); got != "2026-03-01" {
		t.Errorf("got %q, want %q", got, "2026-03-01")
	}
}

func TestEventTimeString_Nil(t *testing.T) {
	if got := eventTimeString(nil); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestEventTimeString_DateTimeTakesPrecedence(t *testing.T) {
	edt := &calendar.EventDateTime{
		DateTime: "2026-03-01T10:00:00Z",
		Date:     "2026-03-01",
	}
	if got := eventTimeString(edt); got != "2026-03-01T10:00:00Z" {
		t.Errorf("DateTime should take precedence: got %q", got)
	}
}

func TestEventTimeString_EmptyDateTime(t *testing.T) {
	edt := &calendar.EventDateTime{DateTime: "", Date: ""}
	if got := eventTimeString(edt); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
