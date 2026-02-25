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
)

const (
	scopeOpenID        = "openid"
	scopeEmail         = "email"
	scopeUserinfoEmail = "https://www.googleapis.com/auth/userinfo.email"
)

var errUnknownService = errors.New("unknown service")

var serviceScopes = map[Service][]string{
	ServiceGmail: {
		"https://www.googleapis.com/auth/gmail.modify",
		"https://www.googleapis.com/auth/gmail.settings.basic",
		"https://www.googleapis.com/auth/gmail.settings.sharing",
	},
	ServiceCalendar: {
		"https://www.googleapis.com/auth/calendar",
	},
	ServiceDocs: {
		"https://www.googleapis.com/auth/drive",
		"https://www.googleapis.com/auth/documents",
	},
	ServiceDrive: {
		"https://www.googleapis.com/auth/drive",
	},
}

// AllServices returns all supported services.
func AllServices() []Service {
	return []Service{ServiceGmail, ServiceCalendar, ServiceDocs, ServiceDrive}
}

// ParseService parses a service name string.
func ParseService(s string) (Service, error) {
	svc := Service(strings.ToLower(strings.TrimSpace(s)))
	if _, ok := serviceScopes[svc]; ok {
		return svc, nil
	}

	return "", fmt.Errorf("%w %q (expected gmail, calendar, docs, or drive)", errUnknownService, s)
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
