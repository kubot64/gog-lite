package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PolicyFile stores execution constraints for AI-agent operations.
type PolicyFile struct {
	AllowedActions         []string `json:"allowed_actions,omitempty"`
	BlockedAccounts        []string `json:"blocked_accounts,omitempty"`
	RequireApprovalActions []string `json:"require_approval_actions,omitempty"`
}

func PolicyPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "policy.json"), nil
}

func ReadPolicy() (PolicyFile, error) {
	path, err := PolicyPath()
	if err != nil {
		return PolicyFile{}, err
	}

	b, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return PolicyFile{}, nil
		}

		return PolicyFile{}, fmt.Errorf("read policy: %w", err)
	}

	var p PolicyFile
	if err := json.Unmarshal(b, &p); err != nil {
		return PolicyFile{}, fmt.Errorf("decode policy: %w", err)
	}

	p.normalize()

	return p, nil
}

func WritePolicy(p PolicyFile) error {
	p.normalize()

	dir, err := EnsureDir()
	if err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}

	path := filepath.Join(dir, "policy.json")
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("encode policy: %w", err)
	}
	b = append(b, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write policy: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit policy: %w", err)
	}

	return nil
}

func (p *PolicyFile) normalize() {
	p.AllowedActions = normalizeUnique(p.AllowedActions)
	p.BlockedAccounts = normalizeUnique(p.BlockedAccounts)
	p.RequireApprovalActions = normalizeUnique(p.RequireApprovalActions)
}

func normalizeUnique(in []string) []string {
	if len(in) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(in))
	for _, v := range in {
		v = strings.ToLower(strings.TrimSpace(v))
		if v != "" {
			set[v] = struct{}{}
		}
	}

	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)

	return out
}
