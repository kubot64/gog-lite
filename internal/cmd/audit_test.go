package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	if !strings.Contains(s, `"hash_alg":"hmac-sha256"`) {
		t.Fatalf("missing hmac hash_alg in %q", s)
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
	if first.HashAlg != auditHashAlgHMAC || second.HashAlg != auditHashAlgHMAC {
		t.Fatalf("hash_alg = %q / %q, want %s", first.HashAlg, second.HashAlg, auditHashAlgHMAC)
	}
	if err := verifyAuditLog(path); err != nil {
		t.Fatalf("verifyAuditLog: %v", err)
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

	// Tamper: change the action field of the first entry (the stored hash is now stale).
	first.Action = "tampered"
	rewritten, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("encode tampered entry: %v", err)
	}
	tamperedLog := append(append(rewritten, '\n'), []byte(lines[1])...)
	if err := os.WriteFile(path, append(tamperedLog, '\n'), 0o600); err != nil {
		t.Fatalf("write tampered log: %v", err)
	}
	if err := verifyAuditLog(path); err == nil {
		t.Fatal("expected tampered chain to be rejected")
	}
	if err := appendAuditLog(path, auditEntry{Action: "third"}); err == nil {
		t.Fatal("expected append onto a tampered chain to be rejected")
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

func setupAuditHome(t *testing.T) string {
	t.Helper()
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("GOG_LITE_KEYRING_BACKEND", "file")
	t.Setenv("GOG_LITE_KEYRING_PASSWORD", "test-password")

	path, err := resolveAuditLogPath("")
	if err != nil {
		t.Fatalf("resolveAuditLogPath: %v", err)
	}

	return path
}

func TestAppendAuditLog_RequiresKeyring(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("GOG_LITE_KEYRING_BACKEND", "file")
	t.Setenv("GOG_LITE_KEYRING_PASSWORD", "")

	path, err := resolveAuditLogPath("")
	if err != nil {
		t.Fatalf("resolveAuditLogPath: %v", err)
	}
	if err := appendAuditLog(path, auditEntry{Action: "needs-keyring"}); err == nil {
		t.Fatal("expected append to fail without a keyring password")
	}
}

func TestAuditHashChain_LinksLegacyPrefix(t *testing.T) {
	path := setupAuditHome(t)

	legacy := auditEntry{
		Timestamp: "2020-01-02T03:04:05Z",
		Action:    "legacy.action",
		Account:   "old@example.com",
		Target:    "doc-1",
		DryRun:    false,
	}
	legacy.Hash = computeAuditHash(legacy)
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("encode legacy entry: %v", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatalf("write legacy log: %v", err)
	}

	if err := appendAuditLog(path, auditEntry{
		Action:  "new.action",
		Account: "old@example.com",
		Target:  "doc-1",
	}); err != nil {
		t.Fatalf("append onto legacy log: %v", err)
	}
	if err := verifyAuditLog(path); err != nil {
		t.Fatalf("verify mixed chain: %v", err)
	}

	entries, err := readAuditEntries(path)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	if entries[0].HashAlg != "" || entries[0].Hash != legacy.Hash {
		t.Fatalf("legacy entry = %+v", entries[0])
	}
	if entries[1].HashAlg != auditHashAlgHMAC || entries[1].PrevHash != entries[0].Hash {
		t.Fatalf("new entry = %+v", entries[1])
	}
	if computeAuditHash(entries[1]) == entries[1].Hash {
		t.Fatal("hmac entry matched a secret-free sha256")
	}

	entries[0].Action = "rewritten"
	entries[0].Hash = computeAuditHash(entries[0])
	if err := writeAuditEntries(path, entries); err != nil {
		t.Fatalf("rewrite legacy prefix: %v", err)
	}
	if err := verifyAuditLog(path); err == nil {
		t.Fatal("recomputed legacy prefix should break the hmac link")
	}
}

func TestAuditHashChain_RejectsRecomputedLegacy(t *testing.T) {
	path := setupAuditHome(t)
	if err := appendAuditLog(path, auditEntry{Action: "one", Account: "a@example.com"}); err != nil {
		t.Fatalf("append #1: %v", err)
	}
	if err := appendAuditLog(path, auditEntry{Action: "two", Account: "a@example.com"}); err != nil {
		t.Fatalf("append #2: %v", err)
	}

	entries, err := readAuditEntries(path)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	prev := ""
	for i := range entries {
		entries[i].HashAlg = ""
		entries[i].Action = "tampered"
		entries[i].PrevHash = prev
		entries[i].Hash = computeAuditHash(entries[i])
		prev = entries[i].Hash
	}
	if err := writeAuditEntries(path, entries); err != nil {
		t.Fatalf("rewrite log: %v", err)
	}

	err = verifyAuditLog(path)
	if err == nil {
		t.Fatal("recomputed sha256 chain should be rejected")
	}
	if !strings.Contains(err.Error(), "tail") {
		t.Fatalf("expected tail mismatch, got %v", err)
	}
	if err := appendAuditLog(path, auditEntry{Action: "three"}); err == nil {
		t.Fatal("expected append onto a recomputed chain to be rejected")
	}
}

func TestAuditHashChain_ConcurrentAppend(t *testing.T) {
	path := setupAuditHome(t)
	const n = 8

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- appendAuditLog(path, auditEntry{
				Action: "concurrent",
				Target: fmt.Sprintf("t-%02d", i),
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent append: %v", err)
		}
	}

	if err := verifyAuditLog(path); err != nil {
		t.Fatalf("verify concurrent chain: %v", err)
	}
	entries, err := readAuditEntries(path)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if len(entries) != n {
		t.Fatalf("want %d entries, got %d", n, len(entries))
	}

	seen := make(map[string]struct{}, n)
	prev := ""
	for i, entry := range entries {
		if entry.PrevHash != prev {
			t.Fatalf("entry %d prev_hash=%q want %q", i, entry.PrevHash, prev)
		}
		if entry.HashAlg != auditHashAlgHMAC {
			t.Fatalf("entry %d hash_alg=%q", i, entry.HashAlg)
		}
		seen[entry.Target] = struct{}{}
		prev = entry.Hash
	}
	if len(seen) != n {
		t.Fatalf("unique targets = %d, want %d", len(seen), n)
	}
}

func TestAuditHashChain_ConcurrentAppendStealsDeadLock(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("thread-level flock contention is Linux-specific")
	}
	path := setupAuditHome(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pid := unusedPID(t)
	if err := os.WriteFile(path+".lock", []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
		t.Fatalf("write dead lock: %v", err)
	}

	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- appendAuditLog(path, auditEntry{
				Action: "concurrent",
				Target: fmt.Sprintf("dead-lock-%02d", i),
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent append over dead lock: %v", err)
		}
	}

	if err := verifyAuditLog(path); err != nil {
		t.Fatalf("verify chain after dead lock: %v", err)
	}
	entries, err := readAuditEntries(path)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if len(entries) != n {
		t.Fatalf("want %d entries, got %d", n, len(entries))
	}
	prev := ""
	for i, entry := range entries {
		if entry.PrevHash != prev {
			t.Fatalf("entry %d prev_hash=%q want %q", i, entry.PrevHash, prev)
		}
		prev = entry.Hash
	}
}

func TestAuditHashChain_AcceptsSingleUncommittedTail(t *testing.T) {
	path := setupAuditHome(t)
	if err := appendAuditLog(path, auditEntry{Action: "one"}); err != nil {
		t.Fatalf("append #1: %v", err)
	}
	if err := appendAuditLog(path, auditEntry{Action: "two"}); err != nil {
		t.Fatalf("append #2: %v", err)
	}

	entries, err := readAuditEntries(path)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if err := storeAuditTail(path, entries[0].Hash); err != nil {
		t.Fatalf("rewind tail: %v", err)
	}
	if err := verifyAuditLog(path); err != nil {
		t.Fatalf("one uncommitted entry should still verify: %v", err)
	}
	if err := appendAuditLog(path, auditEntry{Action: "three"}); err != nil {
		t.Fatalf("append after uncommitted tail: %v", err)
	}

	entries, err = readAuditEntries(path)
	if err != nil {
		t.Fatalf("reread entries: %v", err)
	}
	tip, err := loadAuditTail(path)
	if err != nil {
		t.Fatalf("load tail: %v", err)
	}
	if tip != entries[len(entries)-1].Hash {
		t.Fatalf("tail=%q want %q", tip, entries[len(entries)-1].Hash)
	}
}

func TestAuditHashChain_RejectsTruncation(t *testing.T) {
	path := setupAuditHome(t)
	for _, action := range []string{"one", "two", "three"} {
		if err := appendAuditLog(path, auditEntry{Action: action}); err != nil {
			t.Fatalf("append %s: %v", action, err)
		}
	}

	entries, err := readAuditEntries(path)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if err := writeAuditEntries(path, entries[:1]); err != nil {
		t.Fatalf("truncate log: %v", err)
	}
	if err := verifyAuditLog(path); err == nil {
		t.Fatal("truncated log should be rejected")
	}
	if err := appendAuditLog(path, auditEntry{Action: "four"}); err == nil {
		t.Fatal("expected append onto a truncated log to be rejected")
	}
}

func TestAuditHashChain_RejectsDroppingLastEntry(t *testing.T) {
	path := setupAuditHome(t)
	for _, action := range []string{"one", "two", "three"} {
		if err := appendAuditLog(path, auditEntry{Action: action}); err != nil {
			t.Fatalf("append %s: %v", action, err)
		}
	}

	entries, err := readAuditEntries(path)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if err := writeAuditEntries(path, entries[:len(entries)-1]); err != nil {
		t.Fatalf("drop last entry: %v", err)
	}
	if err := verifyAuditLog(path); err == nil {
		t.Fatal("dropping the last entry should be rejected")
	}
}

func TestAuditHashChain_KeyStaysOutOfLog(t *testing.T) {
	path := setupAuditHome(t)
	if err := appendAuditLog(path, auditEntry{Action: "one"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	key, err := loadAuditMACKey()
	if err != nil {
		t.Fatalf("load key: %v", err)
	}
	again, err := loadAuditMACKey()
	if err != nil {
		t.Fatalf("reload key: %v", err)
	}
	if !bytes.Equal(key, again) {
		t.Fatal("hmac key changed between loads")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if bytes.Contains(raw, key) || bytes.Contains(raw, []byte(fmt.Sprintf("%x", key))) {
		t.Fatal("audit log contains the hmac key")
	}
}

func writeAuditEntries(path string, entries []auditEntry) error {
	var buf bytes.Buffer
	for _, entry := range entries {
		raw, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		buf.Write(raw)
		buf.WriteByte('\n')
	}

	return os.WriteFile(path, buf.Bytes(), 0o600)
}
