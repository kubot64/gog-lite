package cmd

import (
	"fmt"
	"strings"

	"github.com/kubot64/gog-lite/internal/config"
)

var defaultApprovalActions = []string{
	"calendar.delete",
	"docs.write.replace",
	"docs.find_replace",
	"sheets.update",
	"slides.write",
}

func enforceActionPolicy(account, action string) error {
	p, err := config.ReadPolicy()
	if err != nil {
		return fmt.Errorf("read policy: %w", err)
	}

	account = normalizeEmail(account)
	action = strings.ToLower(strings.TrimSpace(action))

	for _, blocked := range p.BlockedAccounts {
		if blocked == account {
			return fmt.Errorf("account %q is blocked by policy", account)
		}
	}

	if len(p.AllowedActions) == 0 {
		return nil
	}

	for _, allowed := range p.AllowedActions {
		if allowed == action {
			return nil
		}
	}

	return fmt.Errorf("action %q is not allowed by policy", action)
}

func actionRequiresApproval(action string) (bool, error) {
	p, err := config.ReadPolicy()
	if err != nil {
		return false, fmt.Errorf("read policy: %w", err)
	}

	action = strings.ToLower(strings.TrimSpace(action))
	// require_approval_actions is united with the defaults.
	// exempt_approval_actions drops a default. An action listed in both stays required.
	if containsAction(p.RequireApprovalActions, action) {
		return true, nil
	}
	if containsAction(p.ExemptApprovalActions, action) {
		return false, nil
	}

	return containsAction(defaultApprovalActions, action), nil
}

func containsAction(actions []string, action string) bool {
	for _, v := range actions {
		if v == action {
			return true
		}
	}

	return false
}
