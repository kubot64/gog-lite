package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"strings"
	"testing"
)

func TestAppendAuditLog_WritesJSONLine(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	path, err := resolveAuditLogPath("")
	if err != nil {
		t.Fatalf("resolveAuditLogPath: %v", err)
	}

	err = appendAuditLog(path, auditEntry{
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
	if !strings.Contains(s, `"hash":"`) {
		t.Fatalf("missing hash in %q", s)
	}
}

func TestAppendAuditLog_HashChain(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	path, err := resolveAuditLogPath("")
	if err != nil {
		t.Fatalf("resolveAuditLogPath: %v", err)
	}

	if err := appendAuditLog(path, auditEntry{Action: "one"}); err != nil {
		t.Fatalf("append #1: %v", err)
	}
	if err := appendAuditLog(path, auditEntry{Action: "two"}); err != nil {
		t.Fatalf("append #2: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}

	var first auditEntry
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("decode first line: %v", err)
	}
	var second auditEntry
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("decode second line: %v", err)
	}
	if second.PrevHash != first.Hash {
		t.Fatalf("prev_hash=%q want %q", second.PrevHash, first.Hash)
	}
}

func TestResolveAuditLogPath_RejectsOutsideConfigDir(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	_, err := resolveAuditLogPath("/tmp/outside.log")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAuditHashChain_TamperDetect(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	path, err := resolveAuditLogPath("")
	if err != nil {
		t.Fatalf("resolveAuditLogPath: %v", err)
	}

	if err := appendAuditLog(path, auditEntry{Action: "original"}); err != nil {
		t.Fatalf("append #1: %v", err)
	}
	if err := appendAuditLog(path, auditEntry{Action: "second"}); err != nil {
		t.Fatalf("append #2: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}

	var first auditEntry
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("decode first line: %v", err)
	}
	var second auditEntry
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("decode second line: %v", err)
	}

	// Tamper: change the action field of the first entry (the stored hash is now stale).
	tampered := first
	tampered.Action = "tampered"

	// The recomputed hash must differ from the stored hash → tampering is detectable.
	if computeAuditHash(tampered) == first.Hash {
		t.Fatal("tampered entry should produce a different hash")
	}
	// The second entry's prev_hash no longer matches the tampered first entry → chain is broken.
	if second.PrevHash == computeAuditHash(tampered) {
		t.Fatal("chain should be broken after tampering with first entry")
	}
}

func TestResolveAuditLogPath_AllowsPathUnderConfigDir(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	base, err := resolveAuditLogPath("")
	if err != nil {
		t.Fatalf("resolve base path: %v", err)
	}
	allowed := filepath.Join(filepath.Dir(base), "nested", "audit.log")
	if _, err := resolveAuditLogPath(allowed); err != nil {
		t.Fatalf("expected allowed path, got error: %v", err)
	}
}

func TestResolveAuditLogPath_RejectsSymlinkEscape(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	base, err := resolveAuditLogPath("")
	if err != nil {
		t.Fatalf("resolve base path: %v", err)
	}

	configDir := filepath.Dir(base)
	outsideDir := t.TempDir()
	linkPath := filepath.Join(configDir, "escape")
	if err := os.Symlink(outsideDir, linkPath); err != nil {
		t.Skipf("symlink not supported on this environment: %v", err)
	}

	escaped := filepath.Join(linkPath, "audit.log")
	if _, err := resolveAuditLogPath(escaped); err == nil {
		t.Fatal("expected symlink escape path to be rejected")
	}
}

func TestAppendAuditLog_ConcurrentHashChain(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	path, err := resolveAuditLogPath("")
	if err != nil {
		t.Fatalf("resolveAuditLogPath: %v", err)
	}

	const writers = 32
	var wg sync.WaitGroup
	errCh := make(chan error, writers)

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errCh <- appendAuditLog(path, auditEntry{
				Action:  "concurrent.write",
				Account: "user@example.com",
				Target:  filepath.Base(path),
				DryRun:  i%2 == 0,
			})
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("appendAuditLog: %v", err)
		}
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != writers {
		t.Fatalf("want %d lines, got %d", writers, len(lines))
	}

	var prevHash string
	for i, line := range lines {
		var entry auditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("decode line %d: %v; line=%q", i, err, line)
		}
		if entry.PrevHash != prevHash {
			t.Fatalf("line %d prev_hash=%q want %q", i, entry.PrevHash, prevHash)
		}
		if entry.Hash != computeAuditHash(entry) {
			t.Fatalf("line %d hash mismatch", i)
		}
		prevHash = entry.Hash
	}
}
