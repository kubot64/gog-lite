package cmd

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEnforceRateLimit(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	nowUTC = func() time.Time { return base }
	t.Cleanup(func() { nowUTC = func() time.Time { return time.Now().UTC() } })

	if err := enforceRateLimit("gmail.send", 2, time.Minute); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if err := enforceRateLimit("gmail.send", 2, time.Minute); err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if err := enforceRateLimit("gmail.send", 2, time.Minute); err == nil {
		t.Fatal("expected rate limit error, got nil")
	}
}

func TestEnforceRateLimit_ConcurrentRequestsRespectLimit(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	nowUTC = func() time.Time { return base }
	t.Cleanup(func() { nowUTC = func() time.Time { return time.Now().UTC() } })

	start := make(chan struct{})
	var wg sync.WaitGroup
	var successCount atomic.Int32
	errs := make(chan error, 8)

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := enforceRateLimit("gmail.send", 3, time.Minute); err == nil {
				successCount.Add(1)
			} else {
				errs <- err
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errs)

	if got := successCount.Load(); got != 3 {
		t.Fatalf("successCount = %d, want 3", got)
	}

	var failureCount int
	for err := range errs {
		failureCount++
		if !strings.Contains(err.Error(), "rate limit exceeded for gmail.send") {
			t.Fatalf("unexpected loser error: %v", err)
		}
	}
	if failureCount != 5 {
		t.Fatalf("failureCount = %d, want 5", failureCount)
	}

	path, err := rateLimitPath("gmail.send")
	if err != nil {
		t.Fatalf("rateLimitPath: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rate limit state: %v", err)
	}

	var st rateLimitState
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatalf("parse rate limit state: %v", err)
	}
	if got := len(st.Timestamps); got != 3 {
		t.Fatalf("persisted timestamps = %d, want 3", got)
	}
}

func TestEnforceRateLimit_InvalidStateFailsClosed(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	path, err := rateLimitPath("gmail.send")
	if err != nil {
		t.Fatalf("rateLimitPath: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write invalid state: %v", err)
	}

	if err := enforceRateLimit("gmail.send", 1, time.Minute); err == nil {
		t.Fatal("expected invalid state to fail")
	}
}
