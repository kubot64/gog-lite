package googleapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/99designs/keyring"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"

	"github.com/morikubo-takashi/gog-lite/internal/config"
	"github.com/morikubo-takashi/gog-lite/internal/googleauth"
	"github.com/morikubo-takashi/gog-lite/internal/secrets"
)

const defaultHTTPTimeout = 30 * time.Second

// AuthRequiredError is returned when no stored token is found for an account.
type AuthRequiredError struct {
	Service string
	Email   string
	Cause   error
}

func (e *AuthRequiredError) Error() string {
	return fmt.Sprintf("auth required for %s %s; run: gog-lite auth login --account %s --services %s",
		e.Service, e.Email, e.Email, e.Service)
}

func (e *AuthRequiredError) Unwrap() error {
	return e.Cause
}

// optionsForEmail creates Google API client options for the given email and service.
func optionsForEmail(ctx context.Context, service googleauth.Service, email string) ([]option.ClientOption, error) {
	scopes, err := googleauth.Scopes(service)
	if err != nil {
		return nil, fmt.Errorf("resolve scopes: %w", err)
	}

	return optionsForEmailWithScopes(ctx, string(service), email, scopes)
}

func optionsForEmailWithScopes(
	ctx context.Context,
	serviceName string,
	email string,
	scopes []string,
) ([]option.ClientOption, error) {
	scopes = normalizeScopes(scopes)
	if len(scopes) == 0 {
		return nil, fmt.Errorf("no scopes configured for %s", serviceName)
	}

	creds, err := config.ReadCredentials()
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	store, err := secrets.OpenDefault()
	if err != nil {
		return nil, fmt.Errorf("open secrets store: %w", err)
	}

	tok, err := store.GetToken(email)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return nil, &AuthRequiredError{Service: serviceName, Email: email, Cause: err}
		}

		return nil, fmt.Errorf("get token for %s: %w", email, err)
	}

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       scopes,
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: defaultHTTPTimeout})
	ts := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: tok.RefreshToken})

	baseTransport := newBaseTransport()
	retryTransport := NewRetryTransport(&oauth2.Transport{
		Source: ts,
		Base:   baseTransport,
	})

	c := &http.Client{
		Transport: retryTransport,
		Timeout:   defaultHTTPTimeout,
	}

	return []option.ClientOption{option.WithHTTPClient(c)}, nil
}

func normalizeScopes(scopes []string) []string {
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}

	return out
}
