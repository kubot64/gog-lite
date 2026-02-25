package cmd

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kubot64/gog-lite/internal/config"
)

type approvalTokenState struct {
	Token     string `json:"token"`
	Account   string `json:"account"`
	Action    string `json:"action"`
	ExpiresAt string `json:"expires_at"`
	Used      bool   `json:"used"`
}

func issueApprovalToken(account, action string, ttl time.Duration) (string, string, error) {
	if ttl <= 0 {
		return "", "", fmt.Errorf("ttl must be positive")
	}

	account = normalizeEmail(account)
	action = strings.ToLower(strings.TrimSpace(action))
	if account == "" || action == "" {
		return "", "", fmt.Errorf("account and action are required")
	}

	token, err := randomToken()
	if err != nil {
		return "", "", err
	}
	expiresAt := time.Now().UTC().Add(ttl)

	st := approvalTokenState{
		Token:     token,
		Account:   account,
		Action:    action,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}

	path, err := approvalTokenPath(token)
	if err != nil {
		return "", "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", "", fmt.Errorf("ensure approvals dir: %w", err)
	}

	b, err := json.Marshal(st)
	if err != nil {
		return "", "", fmt.Errorf("encode approval token: %w", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return "", "", fmt.Errorf("write approval token: %w", err)
	}

	return token, st.ExpiresAt, nil
}

func consumeApprovalToken(account, action, token string) error {
	account = normalizeEmail(account)
	action = strings.ToLower(strings.TrimSpace(action))
	token = strings.TrimSpace(token)
	if account == "" || action == "" || token == "" {
		return fmt.Errorf("approval token, account, and action are required")
	}

	path, err := approvalTokenPath(token)
	if err != nil {
		return err
	}

	b, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("approval token not found")
		}

		return fmt.Errorf("read approval token: %w", err)
	}

	var st approvalTokenState
	if err := json.Unmarshal(b, &st); err != nil {
		return fmt.Errorf("decode approval token: %w", err)
	}

	if st.Used {
		return fmt.Errorf("approval token already used")
	}
	if st.Account != account || st.Action != action {
		return fmt.Errorf("approval token does not match account/action")
	}

	exp, err := time.Parse(time.RFC3339, st.ExpiresAt)
	if err != nil || time.Now().UTC().After(exp) {
		return fmt.Errorf("approval token expired")
	}

	st.Used = true
	out, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("encode approval token: %w", err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("mark approval token used: %w", err)
	}

	return nil
}

func approvalTokenPath(token string) (string, error) {
	dir, err := config.EnsureDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}

	return filepath.Join(dir, "approvals", token+".json"), nil
}

func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate approval token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}
