package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/morikubo-takashi/gog-lite/internal/config"
)

var nowUTC = func() time.Time { return time.Now().UTC() }

type rateLimitState struct {
	Timestamps []string `json:"timestamps"`
}

func enforceRateLimit(action string, limit int, window time.Duration) error {
	if limit <= 0 || window <= 0 {
		return nil
	}
	action = strings.TrimSpace(action)
	if action == "" {
		return nil
	}

	path, err := rateLimitPath(action)
	if err != nil {
		return err
	}

	state, err := loadRateLimitState(path)
	if err != nil {
		return err
	}

	now := nowUTC()
	cutoff := now.Add(-window)
	kept := make([]string, 0, len(state.Timestamps)+1)
	for _, ts := range state.Timestamps {
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		if !t.Before(cutoff) {
			kept = append(kept, ts)
		}
	}

	if len(kept) >= limit {
		return fmt.Errorf("rate limit exceeded for %s: max %d per %s", action, limit, window)
	}

	kept = append(kept, now.Format(time.RFC3339))
	state.Timestamps = kept

	return saveRateLimitState(path, state)
}

func rateLimitPath(action string) (string, error) {
	base, err := config.EnsureDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}

	dir := filepath.Join(base, "ratelimit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure ratelimit dir: %w", err)
	}

	safe := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(action)
	if safe == "" {
		safe = "default"
	}

	return filepath.Join(dir, safe+".json"), nil
}

func loadRateLimitState(path string) (rateLimitState, error) {
	b, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return rateLimitState{}, nil
		}

		return rateLimitState{}, fmt.Errorf("read rate limit state: %w", err)
	}

	var st rateLimitState
	if err := json.Unmarshal(b, &st); err != nil {
		return rateLimitState{}, nil
	}

	return st, nil
}

func saveRateLimitState(path string, st rateLimitState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("encode rate limit state: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write rate limit state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit rate limit state: %w", err)
	}

	return nil
}
