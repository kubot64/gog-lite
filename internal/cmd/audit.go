package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type auditEntry struct {
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Account   string `json:"account,omitempty"`
	Target    string `json:"target,omitempty"`
	DryRun    bool   `json:"dry_run"`
}

func appendAuditLog(path string, entry auditEntry) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}

	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("ensure audit log directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()

	b, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode audit log: %w", err)
	}

	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	return nil
}
