package googleauth

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Service represents a Google service.
type Service string

const (
	ServiceGmail    Service = "gmail"
	ServiceCalendar Service = "calendar"
	ServiceDocs     Service = "docs"
	ServiceDrive    Service = "drive"
	ServiceSheets   Service = "sheets"
	ServiceSlides   Service = "slides"
)

const (
	scopeOpenID        = "openid"
	scopeEmail         = "email"
	scopeUserinfoEmail = "https://www.googleapis.com/auth/userinfo.email"

	scopeGmailReadonly    = "https://www.googleapis.com/auth/gmail.readonly"
	scopeGmailCompose     = "https://www.googleapis.com/auth/gmail.compose"
	scopeCalendarReadonly = "https://www.googleapis.com/auth/calendar.readonly"
	scopeCalendar         = "https://www.googleapis.com/auth/calendar"
	scopeDocsReadonly     = "https://www.googleapis.com/auth/documents.readonly"
	scopeDocs             = "https://www.googleapis.com/auth/documents"
	scopeDriveReadonly    = "https://www.googleapis.com/auth/drive.readonly"
	scopeSheetsReadonly   = "https://www.googleapis.com/auth/spreadsheets.readonly"
	scopeSheets           = "https://www.googleapis.com/auth/spreadsheets"
	scopeSlidesReadonly   = "https://www.googleapis.com/auth/presentations.readonly"
	scopeSlides           = "https://www.googleapis.com/auth/presentations"
)

var errUnknownService = errors.New("unknown service")

// serviceScopes is the consent set for each service. It is the union of the
// scopes runtime API clients actually send. Docs export calls Drive
// files.export on an existing document ID. That method allows drive,
// drive.file, and drive.readonly, but drive.file only reaches files this app
// created or opened through the Google Picker, so drive.readonly is the
// narrow scope that still works. The full drive scope is not requested.
var serviceScopes = map[Service][]string{
	ServiceGmail: {
		scopeGmailReadonly,
		scopeGmailCompose,
	},
	ServiceCalendar: {
		scopeCalendarReadonly,
		scopeCalendar,
	},
	ServiceDocs: {
		scopeDocsReadonly,
		scopeDocs,
		scopeDriveReadonly,
	},
	ServiceDrive: {
		scopeDriveReadonly,
	},
	ServiceSheets: {
		scopeSheetsReadonly,
		scopeSheets,
	},
	ServiceSlides: {
		scopeSlidesReadonly,
		scopeSlides,
	},
}

// AllServices returns all supported services.
func AllServices() []Service {
	return []Service{ServiceGmail, ServiceCalendar, ServiceDocs, ServiceDrive, ServiceSheets, ServiceSlides}
}

// ParseService parses a service name string.
func ParseService(s string) (Service, error) {
	svc := Service(strings.ToLower(strings.TrimSpace(s)))
	if _, ok := serviceScopes[svc]; ok {
		return svc, nil
	}

	return "", fmt.Errorf("%w %q (expected gmail, calendar, docs, drive, sheets, or slides)", errUnknownService, s)
}

// Scopes returns the OAuth2 scopes required for the given service.
func Scopes(service Service) ([]string, error) {
	scopes, ok := serviceScopes[service]
	if !ok {
		return nil, errUnknownService
	}

	return append([]string(nil), scopes...), nil
}

// ScopesForServices returns the union of scopes for the given services plus identity scopes.
func ScopesForServices(services []Service) ([]string, error) {
	set := make(map[string]struct{})

	for _, svc := range services {
		scopes, err := Scopes(svc)
		if err != nil {
			return nil, fmt.Errorf("unknown service %q: %w", svc, err)
		}

		for _, s := range scopes {
			set[s] = struct{}{}
		}
	}

	// Always include identity scopes.
	for _, s := range []string{scopeOpenID, scopeEmail, scopeUserinfoEmail} {
		set[s] = struct{}{}
	}

	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}

	sort.Strings(out)

	return out, nil
}
