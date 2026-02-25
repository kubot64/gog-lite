package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/morikubo-takashi/gog-lite/internal/config"
	"github.com/morikubo-takashi/gog-lite/internal/googleauth"
	"github.com/morikubo-takashi/gog-lite/internal/output"
	"github.com/morikubo-takashi/gog-lite/internal/secrets"
)

// AuthCmd groups auth subcommands.
type AuthCmd struct {
	Login  AuthLoginCmd  `cmd:"" help:"Authenticate a Google account (2-step headless flow)."`
	List   AuthListCmd   `cmd:"" help:"List authenticated accounts."`
	Remove AuthRemoveCmd `cmd:"" help:"Remove a stored account token."`
}

// AuthLoginCmd implements the 2-step headless OAuth flow.
//
// Step 1 (no --auth-url):  prints {"auth_url": "..."}
// Step 2 (--auth-url set): exchanges code, stores token, prints {"stored": true, ...}
type AuthLoginCmd struct {
	Account      string `name:"account" required:"" short:"a" help:"Google account email."`
	Services     string `name:"services" default:"gmail,calendar,docs" help:"Comma-separated services to authorize (gmail,calendar,docs,drive)."`
	AuthURL      string `name:"auth-url" help:"Redirect URL from browser (step 2)."`
	ForceConsent bool   `name:"force-consent" help:"Force Google consent screen (re-requests refresh token)."`
}

func (c *AuthLoginCmd) Run(ctx context.Context, _ *RootFlags) error {
	services, err := parseServices(c.Services)
	if err != nil {
		return output.WriteError(output.ExitCodeError, "invalid_services", err.Error())
	}

	scopes, err := googleauth.ScopesForServices(services)
	if err != nil {
		return output.WriteError(output.ExitCodeError, "scope_error", err.Error())
	}

	creds, err := config.ReadCredentials()
	if err != nil {
		var credsMissing *config.CredentialsMissingError
		if errors.As(err, &credsMissing) {
			return output.WriteError(output.ExitCodeError, "credentials_missing", err.Error())
		}

		return output.WriteError(output.ExitCodeError, "credentials_error", err.Error())
	}

	opts := googleauth.AuthorizeOptions{
		Scopes:       scopes,
		ForceConsent: c.ForceConsent,
	}

	// Step 2: exchange code for token.
	if strings.TrimSpace(c.AuthURL) != "" {
		result, err := googleauth.Step2(ctx, creds, opts, c.AuthURL)
		if err != nil {
			return output.WriteError(output.ExitCodeError, "auth_exchange_error", err.Error())
		}

		store, err := secrets.OpenDefault()
		if err != nil {
			return output.WriteError(output.ExitCodeError, "keyring_error", err.Error())
		}

		serviceNames := make([]string, 0, len(services))
		for _, svc := range services {
			serviceNames = append(serviceNames, string(svc))
		}

		tok := secrets.Token{
			Email:        c.Account,
			Services:     serviceNames,
			Scopes:       scopes,
			RefreshToken: result.RefreshToken,
		}

		if err := store.SetToken(c.Account, tok); err != nil {
			return output.WriteError(output.ExitCodeError, "store_token_error", err.Error())
		}

		return output.WriteJSON(os.Stdout, map[string]any{
			"stored":   true,
			"email":    c.Account,
			"services": serviceNames,
		})
	}

	// Step 1: generate auth URL.
	step1, err := googleauth.Step1(ctx, creds, opts)
	if err != nil {
		return output.WriteError(output.ExitCodeError, "auth_url_error", err.Error())
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"auth_url":  step1.AuthURL,
		"next_step": step1.NextStep,
	})
}

// AuthListCmd lists all authenticated accounts.
type AuthListCmd struct{}

func (c *AuthListCmd) Run(_ context.Context, _ *RootFlags) error {
	store, err := secrets.OpenDefault()
	if err != nil {
		return output.WriteError(output.ExitCodeError, "keyring_error", err.Error())
	}

	tokens, err := store.ListTokens()
	if err != nil {
		return output.WriteError(output.ExitCodeError, "list_error", err.Error())
	}

	type accountInfo struct {
		Email     string   `json:"email"`
		Services  []string `json:"services,omitempty"`
		CreatedAt string   `json:"created_at,omitempty"`
	}

	accounts := make([]accountInfo, 0, len(tokens))
	for _, tok := range tokens {
		ai := accountInfo{
			Email:    tok.Email,
			Services: tok.Services,
		}
		if !tok.CreatedAt.IsZero() {
			ai.CreatedAt = tok.CreatedAt.Format("2006-01-02T15:04:05Z")
		}
		accounts = append(accounts, ai)
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"accounts": accounts,
	})
}

// AuthRemoveCmd removes a stored account token.
type AuthRemoveCmd struct {
	Account string `name:"account" required:"" short:"a" help:"Google account email to remove."`
}

func (c *AuthRemoveCmd) Run(_ context.Context, _ *RootFlags) error {
	store, err := secrets.OpenDefault()
	if err != nil {
		return output.WriteError(output.ExitCodeError, "keyring_error", err.Error())
	}

	if err := store.DeleteToken(c.Account); err != nil {
		return output.WriteError(output.ExitCodeError, "remove_error", err.Error())
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"removed": true,
		"email":   c.Account,
	})
}

// parseServices parses a comma-separated list of service names.
func parseServices(csv string) ([]googleauth.Service, error) {
	parts := strings.Split(csv, ",")
	out := make([]googleauth.Service, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		svc, err := googleauth.ParseService(p)
		if err != nil {
			return nil, fmt.Errorf("unknown service %q", p)
		}

		out = append(out, svc)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no services specified")
	}

	return out, nil
}
