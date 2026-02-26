package googleauth_test

import (
	"testing"

	"github.com/kubot64/gog-lite/internal/googleauth"
)

func TestParseService_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  googleauth.Service
	}{
		{"gmail", googleauth.ServiceGmail},
		{"GMAIL", googleauth.ServiceGmail},
		{"  calendar  ", googleauth.ServiceCalendar},
		{"docs", googleauth.ServiceDocs},
		{"drive", googleauth.ServiceDrive},
		{"sheets", googleauth.ServiceSheets},
		{"SHEETS", googleauth.ServiceSheets},
		{"slides", googleauth.ServiceSlides},
		{"SLIDES", googleauth.ServiceSlides},
	}
	for _, tt := range tests {
		got, err := googleauth.ParseService(tt.input)
		if err != nil {
			t.Errorf("ParseService(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseService(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseService_Invalid(t *testing.T) {
	for _, s := range []string{"forms", "", "gma il", "unknown"} {
		if _, err := googleauth.ParseService(s); err == nil {
			t.Errorf("ParseService(%q): expected error, got nil", s)
		}
	}
}

func TestScopesForServices_DeduplicatesDriveScope(t *testing.T) {
	scopes, err := googleauth.ScopesForServices([]googleauth.Service{
		googleauth.ServiceDocs,
		googleauth.ServiceDrive,
	})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, s := range scopes {
		if s == "https://www.googleapis.com/auth/drive" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("drive scope should appear exactly once, got %d", count)
	}
}

func TestScopesForServices_IncludesIdentityScopes(t *testing.T) {
	scopes, err := googleauth.ScopesForServices([]googleauth.Service{googleauth.ServiceGmail})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"openid", "email", "https://www.googleapis.com/auth/userinfo.email"} {
		found := false
		for _, s := range scopes {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("identity scope %q not found in scopes %v", want, scopes)
		}
	}
}

func TestScopesForServices_Sorted(t *testing.T) {
	scopes, err := googleauth.ScopesForServices([]googleauth.Service{
		googleauth.ServiceGmail,
		googleauth.ServiceCalendar,
		googleauth.ServiceDocs,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(scopes); i++ {
		if scopes[i-1] > scopes[i] {
			t.Errorf("scopes not sorted at index %d: %q > %q", i, scopes[i-1], scopes[i])
		}
	}
}

func TestScopesForServices_UnknownService(t *testing.T) {
	_, err := googleauth.ScopesForServices([]googleauth.Service{"unknown"})
	if err == nil {
		t.Error("expected error for unknown service, got nil")
	}
}
