package cmd

import "testing"

func TestValidateHeaderValue_Valid(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "subject", value: "Weekly report"},
		{name: "to", value: "alice@example.com"},
		{name: "cc", value: "alice@example.com,bob@example.com"},
		{name: "account", value: "sender@example.com"},
		{name: "bcc", value: ""},
	} {
		if err := validateHeaderValue(tc.name, tc.value); err != nil {
			t.Errorf("validateHeaderValue(%q, %q): unexpected error: %v", tc.name, tc.value, err)
		}
	}
}

func TestValidateHeaderValue_RejectsCRLF(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "subject", value: "ok\r\nBcc: evil@example.com"},
		{name: "to", value: "alice@example.com\nCc:evil@example.com"},
		{name: "account", value: "sender@example.com\rbad"},
	} {
		if err := validateHeaderValue(tc.name, tc.value); err == nil {
			t.Errorf("validateHeaderValue(%q, %q): expected error, got nil", tc.name, tc.value)
		}
	}
}
