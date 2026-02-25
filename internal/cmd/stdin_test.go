package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestReadStdinWithLimit_OK(t *testing.T) {
	got, err := withStdin(t, "hello", func() (string, error) {
		return readStdinWithLimit(10)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
}

func TestReadStdinWithLimit_Exceeds(t *testing.T) {
	_, err := withStdin(t, strings.Repeat("a", 11), func() (string, error) {
		return readStdinWithLimit(10)
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// withStdinAsync writes input to a pipe in a goroutine to avoid blocking on large payloads.
func withStdinAsync(t *testing.T, input string, fn func() (string, error)) (string, error) {
	t.Helper()

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	go func() {
		_, _ = w.WriteString(input)
		_ = w.Close()
	}()

	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()
	defer func() { _ = r.Close() }()

	return fn()
}

func TestReadStdinWithLimit_AtMaxStdinBytes(t *testing.T) {
	input := strings.Repeat("a", int(maxStdinBytes))
	got, err := withStdinAsync(t, input, func() (string, error) {
		return readStdinWithLimit(maxStdinBytes)
	})
	if err != nil {
		t.Fatalf("unexpected error at maxStdinBytes: %v", err)
	}
	if got != input {
		t.Fatalf("got %d bytes, want %d", len(got), len(input))
	}
}

func TestReadStdinWithLimit_OverMaxStdinBytes(t *testing.T) {
	input := strings.Repeat("a", int(maxStdinBytes)+1)
	_, err := withStdinAsync(t, input, func() (string, error) {
		return readStdinWithLimit(maxStdinBytes)
	})
	if err == nil {
		t.Fatal("expected error for input exceeding maxStdinBytes, got nil")
	}
}

func TestReadStdinWithLimit_InvalidLimit(t *testing.T) {
	_, err := readStdinWithLimit(0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func withStdin(t *testing.T, input string, fn func() (string, error)) (string, error) {
	t.Helper()

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	if _, err := w.WriteString(input); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = w.Close()

	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()
	defer func() { _ = r.Close() }()

	return fn()
}
