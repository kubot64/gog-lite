package googleapi

import (
	"testing"

	"github.com/kubot64/gog-lite/internal/googleauth"
)

func TestRuntimeScopesAreGrantedAtLogin(t *testing.T) {
	cases := []struct {
		service googleauth.Service
		scopes  []string
	}{
		{googleauth.ServiceGmail, []string{scopeGmailReadonly, scopeGmailCompose}},
		{googleauth.ServiceCalendar, []string{scopeCalendarReadonly, scopeCalendarWrite}},
		{googleauth.ServiceDocs, []string{scopeDocsReadonly, scopeDocsWrite, scopeDriveReadonly}},
		{googleauth.ServiceDrive, []string{scopeDriveReadonly}},
		{googleauth.ServiceSheets, []string{scopeSheetsReadonly, scopeSheetsWrite}},
		{googleauth.ServiceSlides, []string{scopeSlidesReadonly, scopeSlidesWrite}},
	}

	for _, tc := range cases {
		granted, err := googleauth.Scopes(tc.service)
		if err != nil {
			t.Errorf("Scopes(%s): %v", tc.service, err)
			continue
		}

		for _, scope := range tc.scopes {
			found := false
			for _, grantedScope := range granted {
				if grantedScope == scope {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("login scopes for %s do not include runtime scope %s (granted %v)", tc.service, scope, granted)
			}
		}
	}
}
