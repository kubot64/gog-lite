package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/morikubo-takashi/gog-lite/internal/config"
)

type auditEntry struct {
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Account   string `json:"account,omitempty"`
	Target    string `json:"target,omitempty"`
	DryRun    bool   `json:"dry_run"`
	PrevHash  string `json:"prev_hash,omitempty"`
	Hash      string `json:"hash"`
}

func appendAuditLog(path string, entry auditEntry) error {
	path, err := resolveAuditLogPath(path)
	if err != nil {
		return err
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	prevHash, err := lastAuditHash(path)
	if err != nil {
		return err
	}
	entry.PrevHash = prevHash
	entry.Hash = computeAuditHash(entry)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("ensure audit log directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()
	_ = os.Chmod(path, 0o600)

	b, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode audit log: %w", err)
	}

	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	return nil
}

func resolveAuditLogPath(path string) (string, error) {
	base, err := config.EnsureDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve config dir absolute path: %w", err)
	}

	if strings.TrimSpace(path) == "" {
		return filepath.Join(baseAbs, "audit.log"), nil
	}

	candidate, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve audit log path: %w", err)
	}
	if candidate == baseAbs {
		return "", fmt.Errorf("audit log path cannot be config directory itself")
	}

	prefix := baseAbs + string(os.PathSeparator)
	if !strings.HasPrefix(candidate, prefix) {
		return "", fmt.Errorf("audit log path must be under %s", baseAbs)
	}

	return candidate, nil
}

func lastAuditHash(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}

		return "", fmt.Errorf("read audit log: %w", err)
	}

	lines := strings.Split(string(b), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		var e auditEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return "", fmt.Errorf("decode audit log tail: %w", err)
		}

		return strings.TrimSpace(e.Hash), nil
	}

	return "", nil
}

func computeAuditHash(entry auditEntry) string {
	data := strings.Join([]string{
		entry.Timestamp,
		entry.Action,
		entry.Account,
		entry.Target,
		fmt.Sprintf("%t", entry.DryRun),
		entry.PrevHash,
	}, "|")
	sum := sha256.Sum256([]byte(data))

	return hex.EncodeToString(sum[:])
}
