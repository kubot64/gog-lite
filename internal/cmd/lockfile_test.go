package cmd

import (
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func setLockAcquireTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	prev := lockAcquireTimeout
	lockAcquireTimeout = d
	t.Cleanup(func() { lockAcquireTimeout = prev })
}

func TestWithFileLock_DoesNotStealAgedLiveLock(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "resource")
	started := make(chan struct{})
	release := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := withFileLock(target, func() error {
			close(started)
			<-release
			return nil
		})
		if err != nil {
			t.Errorf("holder: %v", err)
		}
	}()

	<-started
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(target+".lock", old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	setLockAcquireTimeout(t, 200*time.Millisecond)
	entered := false
	err := withFileLock(target, func() error {
		entered = true
		return nil
	})
	if entered {
		t.Fatal("entered critical section while the holder was alive")
	}
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("err = %v, want timeout", err)
	}
	if _, statErr := os.Stat(target + ".lock"); statErr != nil {
		t.Fatalf("live lock file should remain: %v", statErr)
	}

	close(release)
	wg.Wait()

	if err := withFileLock(target, func() error { return nil }); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
}

func TestWithFileLock_DoesNotStealOtherLiveProcess(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "resource")
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleeper: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	lockPath := target + ".lock"
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	setLockAcquireTimeout(t, 200*time.Millisecond)
	entered := false
	err := withFileLock(target, func() error {
		entered = true
		return nil
	})
	if entered {
		t.Fatal("stole a lock held by a live process")
	}
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("err = %v, want timeout", err)
	}
}

func TestWithFileLock_StealsDeadPIDWithoutWaitingForAge(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "resource")
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleeper: %v", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill sleeper: %v", err)
	}
	_, _ = cmd.Process.Wait()

	lockPath := target + ".lock"
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	setLockAcquireTimeout(t, 500*time.Millisecond)
	entered := false
	err := withFileLock(target, func() error {
		entered = true
		return nil
	})
	if err != nil {
		t.Fatalf("acquire dead lock: %v", err)
	}
	if !entered {
		t.Fatal("expected critical section to run")
	}
}

func TestWithFileLock_LegacyEmptyLockUsesAge(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "resource")
	lockPath := target + ".lock"
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	setLockAcquireTimeout(t, 200*time.Millisecond)
	entered := false
	err := withFileLock(target, func() error {
		entered = true
		return nil
	})
	if entered {
		t.Fatal("stole a fresh lock file that has no PID")
	}
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("fresh legacy lock err = %v, want timeout", err)
	}

	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	entered = false
	if err := withFileLock(target, func() error {
		entered = true
		return nil
	}); err != nil {
		t.Fatalf("acquire aged legacy lock: %v", err)
	}
	if !entered {
		t.Fatal("expected aged legacy lock to be taken")
	}
}

func TestWithFileLock_ReacquiresAfterHolderExits(t *testing.T) {
	if os.Getenv("GOG_LITE_LOCK_HELPER") == "1" {
		// SIGKILL skips the unlock defer, leaving the PID behind as a crash would.
		_ = withFileLock(os.Getenv("GOG_LITE_LOCK_TARGET"), func() error {
			_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
			return nil
		})
		os.Exit(1)
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "resource")
	cmd := exec.Command(os.Args[0], "-test.run=^TestWithFileLock_ReacquiresAfterHolderExits$")
	cmd.Env = append(os.Environ(),
		"GOG_LITE_LOCK_HELPER=1",
		"GOG_LITE_LOCK_TARGET="+target,
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("helper should have been killed, output: %s", out)
	}

	lockPath := target + ".lock"
	pidText, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read leftover lock: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidText)))
	if err != nil {
		t.Fatalf("parse leftover pid %q: %v", pidText, err)
	}
	if processAlive(pid) {
		t.Fatalf("helper pid %d still looks alive", pid)
	}

	// The file is young. A dead holder must still be replaceable.
	if err := withFileLock(target, func() error { return nil }); err != nil {
		t.Fatalf("reacquire: %v", err)
	}
}

func TestWithFileLock_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "resource")
	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := os.Symlink(secret, target+".lock"); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	entered := false
	err := withFileLock(target, func() error {
		entered = true
		return nil
	})
	if entered {
		t.Fatal("followed lock symlink")
	}
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err = %v, want symlink", err)
	}
	got, err := os.ReadFile(secret)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	if string(got) != "keep" {
		t.Fatalf("secret = %q", got)
	}
	if _, err := os.Lstat(target + ".lock"); err != nil {
		t.Fatalf("symlink removed: %v", err)
	}
}

func TestWithFileLock_OnlyOneWaiterStealsStaleLock(t *testing.T) {
	// Linux flock is per open-file description, so goroutines contend the same
	// way processes do. BSD flock is per process and would not show the race.
	if runtime.GOOS != "linux" {
		t.Skip("thread-level flock contention is Linux-specific")
	}
	t.Run("dead pid", func(t *testing.T) {
		pid := unusedPID(t)
		assertOneStealer(t, func(lockPath string) {
			if err := os.WriteFile(lockPath, []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
				t.Fatalf("write lock: %v", err)
			}
		})
	})
	t.Run("aged legacy", func(t *testing.T) {
		assertOneStealer(t, func(lockPath string) {
			if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
				t.Fatalf("write lock: %v", err)
			}
			old := time.Now().Add(-time.Hour)
			if err := os.Chtimes(lockPath, old, old); err != nil {
				t.Fatalf("chtimes: %v", err)
			}
		})
	})
}

// assertOneStealer starts every waiter on one stale lock. The hold is long
// enough that overlapping entry is visible, and far below the acquire timeout.
func assertOneStealer(t *testing.T, plant func(lockPath string)) {
	t.Helper()
	const (
		n        = 16
		hold     = 5 * time.Millisecond
		attempts = 5
	)
	for attempt := 0; attempt < attempts; attempt++ {
		dir := t.TempDir()
		target := filepath.Join(dir, "resource")
		plant(target + ".lock")

		var current, maxSeen, entered atomic.Int32
		errCh := make(chan error, n)
		var ready, wg sync.WaitGroup
		start := make(chan struct{})
		ready.Add(n)
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				ready.Done()
				<-start
				err := withFileLock(target, func() error {
					c := current.Add(1)
					for {
						old := maxSeen.Load()
						if c <= old || maxSeen.CompareAndSwap(old, c) {
							break
						}
					}
					time.Sleep(hold)
					current.Add(-1)
					entered.Add(1)
					return nil
				})
				errCh <- err
			}()
		}
		ready.Wait()
		close(start)
		wg.Wait()
		close(errCh)

		for err := range errCh {
			if err != nil {
				t.Fatalf("attempt %d acquire: %v", attempt, err)
			}
		}
		if got := entered.Load(); got != n {
			t.Fatalf("attempt %d entered = %d, want %d", attempt, got, n)
		}
		if got := maxSeen.Load(); got != 1 {
			t.Fatalf("attempt %d max in section = %d, want 1", attempt, got)
		}
	}
}

func unusedPID(t *testing.T) int {
	t.Helper()
	candidates := []int{math.MaxInt32}
	if b, err := os.ReadFile("/proc/sys/kernel/pid_max"); err == nil {
		n, convErr := strconv.Atoi(strings.TrimSpace(string(b)))
		if convErr == nil && n > 1 {
			candidates = append([]int{n}, candidates...)
		}
	}
	for _, pid := range candidates {
		if pid > 1 && !processAlive(pid) {
			return pid
		}
	}
	t.Fatal("no unused pid for a dead lock")
	return 0
}

func TestWithFileLock_SerializesCriticalSections(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "resource")
	var current atomic.Int32
	var maxSeen atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := withFileLock(target, func() error {
				n := current.Add(1)
				for {
					old := maxSeen.Load()
					if n <= old || maxSeen.CompareAndSwap(old, n) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				current.Add(-1)
				return nil
			})
			if err != nil {
				t.Errorf("lock: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := maxSeen.Load(); got != 1 {
		t.Fatalf("max in section = %d, want 1", got)
	}
}

func TestWithFileLock_ReturnsFnErrorAndReleases(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "resource")
	want := errors.New("boom")
	err := withFileLock(target, func() error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(target + ".lock"); !os.IsNotExist(statErr) {
		t.Fatalf("lock file remains: %v", statErr)
	}
	if err := withFileLock(target, func() error { return nil }); err != nil {
		t.Fatalf("second acquire: %v", err)
	}
}
