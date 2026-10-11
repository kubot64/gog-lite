package cmd

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kubot64/gog-lite/internal/config"
	"github.com/kubot64/gog-lite/internal/secrets"
)

const (
	auditHashAlgLegacy = "sha256"
	auditHashAlgHMAC   = "hmac-sha256"
	auditHMACKeyName   = "audit-hmac-key"
	auditHMACKeySize   = 32
	auditTailKeyPrefix = "audit-tail:"
)

type auditEntry struct {
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Account   string `json:"account,omitempty"`
	Target    string `json:"target,omitempty"`
	DryRun    bool   `json:"dry_run"`
	PrevHash  string `json:"prev_hash,omitempty"`
	HashAlg   string `json:"hash_alg,omitempty"`
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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("ensure audit log directory: %w", err)
	}

	key, err := loadAuditMACKey()
	if err != nil {
		return err
	}

	// Read the previous hash and append under one lock so concurrent writers
	// cannot fork prev_hash.
	return withFileLock(path, func() error {
		entries, err := readAuditEntries(path)
		if err != nil {
			return err
		}
		tip, err := loadAuditTail(path)
		if err != nil {
			return err
		}
		if err := validateAuditChain(entries, key, tip); err != nil {
			return err
		}

		prev := ""
		if n := len(entries); n > 0 {
			prev = entries[n-1].Hash
		}
		entry.PrevHash = prev
		entry.HashAlg = auditHashAlgHMAC
		entry.Hash = computeHMACAuditHash(entry, key)

		if err := writeAuditEntry(path, entry); err != nil {
			return err
		}

		return storeAuditTail(path, entry.Hash)
	})
}

func resolveAuditLogPath(path string) (string, error) {
	base, err := config.EnsureDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	baseResolved, err := resolvePathForContainment(base)
	if err != nil {
		return "", fmt.Errorf("resolve config dir absolute path: %w", err)
	}

	if strings.TrimSpace(path) == "" {
		return filepath.Join(baseResolved, "audit.log"), nil
	}

	candidate, err := resolvePathForContainment(path)
	if err != nil {
		return "", fmt.Errorf("resolve audit log path: %w", err)
	}
	if candidate == baseResolved {
		return "", fmt.Errorf("audit log path cannot be config directory itself")
	}

	prefix := baseResolved + string(os.PathSeparator)
	if !strings.HasPrefix(candidate, prefix) {
		return "", fmt.Errorf("audit log path must be under %s", baseResolved)
	}

	return candidate, nil
}

func verifyAuditLog(path string) error {
	path, err := resolveAuditLogPath(path)
	if err != nil {
		return err
	}
	key, err := loadAuditMACKey()
	if err != nil {
		return err
	}
	entries, err := readAuditEntries(path)
	if err != nil {
		return err
	}
	tip, err := loadAuditTail(path)
	if err != nil {
		return err
	}

	return validateAuditChain(entries, key, tip)
}

func readAuditEntries(path string) ([]auditEntry, error) {
	b, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read audit log: %w", err)
	}

	var entries []auditEntry
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry auditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("decode audit log line %d: %w", i+1, err)
		}
		entry.Hash = strings.TrimSpace(entry.Hash)
		entry.PrevHash = strings.TrimSpace(entry.PrevHash)
		entry.HashAlg = strings.TrimSpace(entry.HashAlg)
		entries = append(entries, entry)
	}

	return entries, nil
}

func writeAuditEntry(path string, entry auditEntry) error {
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

func validateAuditChain(entries []auditEntry, key []byte, tip string) error {
	if err := validateAuditEntries(entries, key); err != nil {
		return err
	}

	tip = strings.TrimSpace(tip)
	if len(entries) == 0 {
		if tip != "" {
			return fmt.Errorf("audit log is empty but a tail hash is stored")
		}

		return nil
	}

	last := entries[len(entries)-1].Hash
	if tip == "" || hashEqual(tip, last) {
		return nil
	}
	// One newer entry than the stored tail is the crash window between append
	// and the tail update. A forged entry cannot satisfy the HMAC check above.
	if len(entries) >= 2 && hashEqual(tip, entries[len(entries)-2].Hash) {
		return nil
	}

	return fmt.Errorf("audit log tail does not match the stored hash")
}

func validateAuditEntries(entries []auditEntry, key []byte) error {
	seenHMAC := false
	prev := ""
	for i, entry := range entries {
		if !hashEqual(entry.PrevHash, prev) {
			return fmt.Errorf("audit log chain break at entry %d", i+1)
		}

		alg := entry.HashAlg
		if alg == "" {
			alg = auditHashAlgLegacy
		}
		if seenHMAC && alg != auditHashAlgHMAC {
			return fmt.Errorf("audit log legacy hash after hmac at entry %d", i+1)
		}

		want, err := expectedAuditHash(entry, alg, key)
		if err != nil {
			return err
		}
		if !hashEqual(want, entry.Hash) {
			return fmt.Errorf("audit log hash mismatch at entry %d", i+1)
		}
		if alg == auditHashAlgHMAC {
			seenHMAC = true
		}
		prev = entry.Hash
	}

	return nil
}

func expectedAuditHash(entry auditEntry, alg string, key []byte) (string, error) {
	switch alg {
	case auditHashAlgLegacy:
		return computeAuditHash(entry), nil
	case auditHashAlgHMAC:
		if len(key) != auditHMACKeySize {
			return "", fmt.Errorf("audit hmac key is unavailable")
		}

		return computeHMACAuditHash(entry, key), nil
	default:
		return "", fmt.Errorf("unsupported audit hash algorithm %q", alg)
	}
}

func canonicalAuditMaterial(entry auditEntry) string {
	return strings.Join([]string{
		entry.Timestamp,
		entry.Action,
		entry.Account,
		entry.Target,
		fmt.Sprintf("%t", entry.DryRun),
		entry.PrevHash,
	}, "|")
}

func computeAuditHash(entry auditEntry) string {
	sum := sha256.Sum256([]byte(canonicalAuditMaterial(entry)))

	return hex.EncodeToString(sum[:])
}

func computeHMACAuditHash(entry auditEntry, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(canonicalAuditMaterial(entry)))

	return hex.EncodeToString(mac.Sum(nil))
}

func hashEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}

func loadAuditMACKey() ([]byte, error) {
	dir, err := config.EnsureDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}

	var key []byte
	err = withFileLock(filepath.Join(dir, "audit-hmac"), func() error {
		store, err := secrets.OpenDefault()
		if err != nil {
			return fmt.Errorf("open keyring for audit log: %w", err)
		}
		key, err = store.GetOrCreateSecret(auditHMACKeyName, auditHMACKeySize)
		if err != nil {
			return fmt.Errorf("load audit hmac key: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return key, nil
}

func auditTailSecretName(path string) string {
	sum := sha256.Sum256([]byte(path))

	return auditTailKeyPrefix + hex.EncodeToString(sum[:])
}

func loadAuditTail(path string) (string, error) {
	store, err := secrets.OpenDefault()
	if err != nil {
		return "", fmt.Errorf("open keyring for audit log: %w", err)
	}
	data, err := store.GetSecret(auditTailSecretName(path))
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			return "", nil
		}

		return "", fmt.Errorf("read audit tail: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

func storeAuditTail(path, hash string) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return fmt.Errorf("audit tail hash is empty")
	}
	store, err := secrets.OpenDefault()
	if err != nil {
		return fmt.Errorf("open keyring for audit log: %w", err)
	}
	if err := store.SetSecret(auditTailSecretName(path), []byte(hash)); err != nil {
		return fmt.Errorf("store audit tail: %w", err)
	}

	return nil
}
