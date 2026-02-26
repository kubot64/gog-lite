package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kubot64/gog-lite/internal/config"
	"github.com/kubot64/gog-lite/internal/googleauth"
	"github.com/kubot64/gog-lite/internal/output"
	"github.com/kubot64/gog-lite/internal/secrets"
)

// AuthCmd groups auth subcommands.
type AuthCmd struct {
	Login           AuthLoginCmd           `cmd:"" help:"Authenticate a Google account (2-step headless flow)."`
	List            AuthListCmd            `cmd:"" help:"List authenticated accounts."`
	Remove          AuthRemoveCmd          `cmd:"" help:"Remove a stored account token."`
	Preflight       AuthPreflightCmd       `cmd:"" help:"Check readiness for AI-agent operations."`
	ApprovalToken   AuthApprovalTokenCmd   `cmd:"" help:"Issue one-time approval token for dangerous actions."`
	EmergencyRevoke AuthEmergencyRevokeCmd `cmd:"" help:"Immediately revoke account token and block account by policy."`
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

func (c *AuthLoginCmd) Run(ctx context.Context, root *RootFlags) error {
	account := normalizeEmail(c.Account)
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
		tokenEmail := normalizeEmail(result.Email)
		if tokenEmail == "" || tokenEmail != account {
			return output.WriteError(
				output.ExitCodeError,
				"account_mismatch",
				fmt.Sprintf("--account %q does not match authorized account %q", c.Account, result.Email),
			)
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
			Email:        account,
			Services:     serviceNames,
			Scopes:       scopes,
			RefreshToken: result.RefreshToken,
		}

		if err := store.SetToken(account, tok); err != nil {
			return output.WriteError(output.ExitCodeError, "store_token_error", err.Error())
		}
		if err := appendAuditLog(root.AuditLog, auditEntry{
			Action:  "auth.login",
			Account: account,
			Target:  strings.Join(serviceNames, ","),
			DryRun:  false,
		}); err != nil {
			return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
		}

		return output.WriteJSON(os.Stdout, map[string]any{
			"stored":   true,
			"email":    account,
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

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
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

func (c *AuthRemoveCmd) Run(_ context.Context, root *RootFlags) error {
	store, err := secrets.OpenDefault()
	if err != nil {
		return output.WriteError(output.ExitCodeError, "keyring_error", err.Error())
	}

	if err := store.DeleteToken(c.Account); err != nil {
		return output.WriteError(output.ExitCodeError, "remove_error", err.Error())
	}
	if err := appendAuditLog(root.AuditLog, auditEntry{
		Action:  "auth.remove",
		Account: normalizeEmail(c.Account),
		DryRun:  false,
	}); err != nil {
		return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"removed": true,
		"email":   c.Account,
	})
}

type AuthPreflightCmd struct {
	Account        string `name:"account" required:"" short:"a" help:"Google account email."`
	RequireActions string `name:"require-actions" help:"Comma-separated action IDs to verify (e.g. gmail.send,calendar.create)."`
}

func (c *AuthPreflightCmd) Run(_ context.Context, _ *RootFlags) error {
	account := normalizeEmail(c.Account)
	required := splitCSV(c.RequireActions)

	type checkResult struct {
		Name    string `json:"name"`
		OK      bool   `json:"ok"`
		Message string `json:"message,omitempty"`
	}

	checks := make([]checkResult, 0, 4+len(required))
	ready := true

	if _, err := config.ReadCredentials(); err != nil {
		ready = false
		checks = append(checks, checkResult{Name: "credentials", OK: false, Message: err.Error()})
	} else {
		checks = append(checks, checkResult{Name: "credentials", OK: true})
	}

	store, err := secrets.OpenDefault()
	if err != nil {
		ready = false
		checks = append(checks, checkResult{Name: "keyring", OK: false, Message: err.Error()})
	} else {
		checks = append(checks, checkResult{Name: "keyring", OK: true})
		if _, err := store.GetToken(account); err != nil {
			ready = false
			checks = append(checks, checkResult{Name: "token", OK: false, Message: err.Error()})
		} else {
			checks = append(checks, checkResult{Name: "token", OK: true})
		}
	}

	for _, action := range required {
		if err := enforceActionPolicy(account, action); err != nil {
			ready = false
			checks = append(checks, checkResult{Name: "policy:" + action, OK: false, Message: err.Error()})
		} else {
			checks = append(checks, checkResult{Name: "policy:" + action, OK: true})
		}
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"ready":  ready,
		"email":  account,
		"checks": checks,
	})
}

type AuthApprovalTokenCmd struct {
	Account string `name:"account" required:"" short:"a" help:"Google account email."`
	Action  string `name:"action" required:"" help:"Action ID (e.g. docs.write.replace)."`
	TTL     string `name:"ttl" default:"10m" help:"Token TTL duration (e.g. 5m, 15m, 1h)."`
}

func (c *AuthApprovalTokenCmd) Run(_ context.Context, root *RootFlags) error {
	account := normalizeEmail(c.Account)
	action := strings.ToLower(strings.TrimSpace(c.Action))
	if action == "" {
		return output.WriteError(output.ExitCodeError, "invalid_action", "action is required")
	}

	if err := enforceActionPolicy(account, "auth.approval_token"); err != nil {
		return output.WriteError(output.ExitCodePermission, "policy_denied", err.Error())
	}
	if err := enforceActionPolicy(account, action); err != nil {
		return output.WriteError(output.ExitCodePermission, "policy_denied", err.Error())
	}

	required, err := actionRequiresApproval(action)
	if err != nil {
		return output.WriteError(output.ExitCodeError, "policy_error", err.Error())
	}
	if !required {
		return output.WriteError(output.ExitCodeError, "approval_not_required",
			fmt.Sprintf("action %q does not require approval token", action))
	}

	ttl, err := time.ParseDuration(c.TTL)
	if err != nil {
		return output.WriteError(output.ExitCodeError, "invalid_ttl", fmt.Sprintf("parse ttl: %v", err))
	}

	if root.DryRun {
		if err := appendAuditLog(root.AuditLog, auditEntry{
			Action:  "auth.approval_token",
			Account: account,
			Target:  action,
			DryRun:  true,
		}); err != nil {
			return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
		}

		return output.WriteJSON(os.Stdout, map[string]any{
			"dry_run": true,
			"action":  "auth.approval_token",
			"params": map[string]any{
				"account": account,
				"action":  action,
				"ttl":     c.TTL,
			},
		})
	}

	token, expiresAt, err := issueApprovalToken(account, action, ttl)
	if err != nil {
		return output.WriteError(output.ExitCodeError, "approval_token_error", err.Error())
	}
	if err := appendAuditLog(root.AuditLog, auditEntry{
		Action:  "auth.approval_token",
		Account: account,
		Target:  action,
		DryRun:  false,
	}); err != nil {
		return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"issued":     true,
		"account":    account,
		"action":     action,
		"token":      token,
		"expires_at": expiresAt,
	})
}

type AuthEmergencyRevokeCmd struct {
	Account string `name:"account" required:"" short:"a" help:"Google account email to revoke and block."`
}

func (c *AuthEmergencyRevokeCmd) Run(_ context.Context, root *RootFlags) error {
	account := normalizeEmail(c.Account)
	store, err := secrets.OpenDefault()
	if err != nil {
		return output.WriteError(output.ExitCodeError, "keyring_error", err.Error())
	}

	if err := store.DeleteToken(account); err != nil {
		return output.WriteError(output.ExitCodeError, "remove_error", err.Error())
	}

	p, err := config.ReadPolicy()
	if err != nil {
		return output.WriteError(output.ExitCodeError, "policy_error", err.Error())
	}
	p.BlockedAccounts = append(p.BlockedAccounts, account)
	if err := config.WritePolicy(p); err != nil {
		return output.WriteError(output.ExitCodeError, "policy_error", err.Error())
	}
	if err := appendAuditLog(root.AuditLog, auditEntry{
		Action:  "auth.emergency_revoke",
		Account: account,
		DryRun:  false,
	}); err != nil {
		return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"revoked": true,
		"blocked": true,
		"email":   account,
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

func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}

	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}

	return out
}
