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
		if s == "https://www.googleapis.com/auth/drive.readonly" {
			count++
		}
		if s == "https://www.googleapis.com/auth/drive" || s == "https://www.googleapis.com/auth/drive.file" {
			t.Errorf("unexpected broad drive scope %q", s)
		}
	}
	if count != 1 {
		t.Errorf("drive.readonly scope should appear exactly once, got %d", count)
	}
}

func TestScopes_PinnedToRuntimeClients(t *testing.T) {
	want := map[googleauth.Service][]string{
		googleauth.ServiceGmail: {
			"https://www.googleapis.com/auth/gmail.readonly",
			"https://www.googleapis.com/auth/gmail.compose",
		},
		googleauth.ServiceCalendar: {
			"https://www.googleapis.com/auth/calendar.readonly",
			"https://www.googleapis.com/auth/calendar",
		},
		googleauth.ServiceDocs: {
			"https://www.googleapis.com/auth/documents.readonly",
			"https://www.googleapis.com/auth/documents",
			"https://www.googleapis.com/auth/drive.readonly",
		},
		googleauth.ServiceDrive: {
			"https://www.googleapis.com/auth/drive.readonly",
		},
		googleauth.ServiceSheets: {
			"https://www.googleapis.com/auth/spreadsheets.readonly",
			"https://www.googleapis.com/auth/spreadsheets",
		},
		googleauth.ServiceSlides: {
			"https://www.googleapis.com/auth/presentations.readonly",
			"https://www.googleapis.com/auth/presentations",
		},
	}
	forbidden := []string{
		"https://www.googleapis.com/auth/gmail.modify",
		"https://www.googleapis.com/auth/gmail.settings.basic",
		"https://www.googleapis.com/auth/gmail.settings.sharing",
		"https://www.googleapis.com/auth/drive",
		"https://www.googleapis.com/auth/drive.file",
	}

	if len(want) != len(googleauth.AllServices()) {
		t.Fatalf("pinned scopes cover %d services, AllServices has %d", len(want), len(googleauth.AllServices()))
	}

	for _, svc := range googleauth.AllServices() {
		got, err := googleauth.Scopes(svc)
		if err != nil {
			t.Errorf("Scopes(%s): %v", svc, err)
			continue
		}
		expected := want[svc]
		if len(got) != len(expected) {
			t.Errorf("Scopes(%s) = %v, want %v", svc, got, expected)
			continue
		}
		for i := range expected {
			if got[i] != expected[i] {
				t.Errorf("Scopes(%s)[%d] = %q, want %q", svc, i, got[i], expected[i])
			}
		}
		for _, bad := range forbidden {
			for _, s := range got {
				if s == bad {
					t.Errorf("Scopes(%s) includes forbidden scope %s", svc, bad)
				}
			}
		}
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
