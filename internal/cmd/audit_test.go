package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendAuditLog_WritesJSONLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	err := appendAuditLog(path, auditEntry{
		Action:  "docs.write",
		Account: "you@example.com",
		Target:  "doc-123",
		DryRun:  false,
	})
	if err != nil {
		t.Fatalf("appendAuditLog: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"action":"docs.write"`) {
		t.Fatalf("missing action in %q", s)
	}
	if !strings.Contains(s, `"account":"you@example.com"`) {
		t.Fatalf("missing account in %q", s)
	}
}
