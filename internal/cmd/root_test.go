package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/kubot64/gog-lite/internal/output"
)

func TestCollectAllPages_SinglePage(t *testing.T) {
	calls := 0
	items, next, err := collectAllPages[string](false, 0, func(token string) (string, []string, error) {
		calls++
		return "next-token", []string{"a", "b"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("want 2 items, got %d", len(items))
	}
	if next != "next-token" {
		t.Errorf("want next-token, got %q", next)
	}
	if calls != 1 {
		t.Errorf("want 1 call when allPages=false, got %d", calls)
	}
}

func TestCollectAllPages_AllPages(t *testing.T) {
	page := 0
	items, next, err := collectAllPages[int](true, defaultMaxPages, func(token string) (string, []int, error) {
		page++
		switch page {
		case 1:
			if token != "" {
				t.Errorf("page 1: want empty token, got %q", token)
			}
			return "p2", []int{1, 2}, nil
		case 2:
			if token != "p2" {
				t.Errorf("page 2: want %q, got %q", "p2", token)
			}
			return "p3", []int{3}, nil
		case 3:
			return "", []int{4, 5}, nil
		default:
			t.Errorf("unexpected call %d", page)
			return "", nil, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 5 {
		t.Errorf("want 5 items, got %d: %v", len(items), items)
	}
	for i, v := range items {
		if v != i+1 {
			t.Errorf("items[%d] = %d, want %d", i, v, i+1)
		}
	}
	if next != "" {
		t.Errorf("want empty nextPageToken, got %q", next)
	}
}

func TestCollectAllPages_EmptyResult(t *testing.T) {
	items, next, err := collectAllPages[string](true, defaultMaxPages, func(_ string) (string, []string, error) {
		return "", nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("want 0 items, got %d", len(items))
	}
	if next != "" {
		t.Errorf("want empty next, got %q", next)
	}
}

func TestCollectAllPages_Error(t *testing.T) {
	wantErr := errors.New("api error")
	_, _, err := collectAllPages[string](true, defaultMaxPages, func(_ string) (string, []string, error) {
		return "", nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("want %v, got %v", wantErr, err)
	}
}

func TestCollectAllPages_ErrorMidway(t *testing.T) {
	call := 0
	wantErr := errors.New("second page error")
	_, _, err := collectAllPages[int](true, defaultMaxPages, func(_ string) (string, []int, error) {
		call++
		if call == 2 {
			return "", nil, wantErr
		}
		return "next", []int{1}, nil
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("want %v, got %v", wantErr, err)
	}
}

func TestCollectAllPages_StopsAtMaxPages(t *testing.T) {
	calls := 0
	items, next, err := collectAllPages[int](true, 2, func(token string) (string, []int, error) {
		calls++
		switch calls {
		case 1:
			if token != "" {
				t.Errorf("page 1: want empty token, got %q", token)
			}
			return "p2", []int{1}, nil
		case 2:
			if token != "p2" {
				t.Errorf("page 2: want %q, got %q", "p2", token)
			}
			return "p3", []int{2}, nil
		default:
			t.Errorf("unexpected call %d", calls)
			return "", nil, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("want 2 calls, got %d", calls)
	}
	if len(items) != 2 || items[0] != 1 || items[1] != 2 {
		t.Fatalf("items = %v, want [1 2]", items)
	}
	if next != "p3" {
		t.Errorf("want nextPageToken %q, got %q", "p3", next)
	}
}

func TestCollectAllPages_CapEqualToAvailablePages(t *testing.T) {
	items, next, err := collectAllPages[int](true, 1, func(token string) (string, []int, error) {
		if token != "" {
			t.Errorf("want empty token, got %q", token)
		}
		return "", []int{7}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0] != 7 {
		t.Fatalf("items = %v, want [7]", items)
	}
	if next != "" {
		t.Errorf("want empty nextPageToken, got %q", next)
	}
}

func TestCollectAllPages_RejectsPageCap(t *testing.T) {
	for _, maxPages := range []int{0, -1, maxPagesLimit + 1} {
		calls := 0
		_, _, err := collectAllPages[int](true, maxPages, func(string) (string, []int, error) {
			calls++
			return "next", []int{1}, nil
		})
		if err == nil {
			t.Fatalf("maxPages=%d: expected error", maxPages)
		}
		if calls != 0 {
			t.Fatalf("maxPages=%d: fn called %d times", maxPages, calls)
		}
	}
}

func TestMaxPagesFlagDocumentsLimits(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(GmailSearchCmd{}),
		reflect.TypeOf(CalendarListCmd{}),
	} {
		field, ok := typ.FieldByName("MaxPages")
		if !ok {
			t.Fatalf("%s missing MaxPages", typ.Name())
		}
		if got := field.Tag.Get("default"); got != strconv.Itoa(defaultMaxPages) {
			t.Errorf("%s default = %q, want %d", typ.Name(), got, defaultMaxPages)
		}
		help := field.Tag.Get("help")
		if !strings.Contains(help, strconv.Itoa(maxPagesLimit)) {
			t.Errorf("%s help %q does not mention limit %d", typ.Name(), help, maxPagesLimit)
		}
		if field.Tag.Get("name") != "max-pages" {
			t.Errorf("%s name = %q, want max-pages", typ.Name(), field.Tag.Get("name"))
		}
	}
}

func TestSearchCommands_InvalidMaxPages(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cases := []struct {
		name   string
		action string
		run    func(maxPages int) error
	}{
		{
			name:   "gmail.search",
			action: "gmail.search",
			run: func(maxPages int) error {
				cmd := &GmailSearchCmd{
					Account:  "a@example.com",
					Query:    "is:unread",
					AllPages: true,
					MaxPages: maxPages,
				}
				return cmd.Run(context.Background(), &RootFlags{})
			},
		},
		{
			name:   "calendar.list",
			action: "calendar.list",
			run: func(maxPages int) error {
				cmd := &CalendarListCmd{
					Account:    "a@example.com",
					CalendarID: "primary",
					AllPages:   true,
					MaxPages:   maxPages,
				}
				return cmd.Run(context.Background(), &RootFlags{})
			},
		},
	}

	for _, tc := range cases {
		for _, maxPages := range []int{0, -1, maxPagesLimit + 1} {
			t.Run(tc.name+"/"+strconv.Itoa(maxPages), func(t *testing.T) {
				var runErr error
				stderr := captureStderr(t, func() {
					runErr = tc.run(maxPages)
				})
				if runErr == nil {
					t.Fatal("expected error")
				}
				if output.ExitCode(runErr) != output.ExitCodeError {
					t.Fatalf("exit code = %d, want %d", output.ExitCode(runErr), output.ExitCodeError)
				}
				var payload struct {
					Code  string `json:"code"`
					Error string `json:"error"`
				}
				if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
					t.Fatalf("parse stderr JSON: %v (got %q)", err, stderr)
				}
				if payload.Code != "invalid_arguments" {
					t.Fatalf("code = %q, want invalid_arguments", payload.Code)
				}
				if !strings.Contains(payload.Error, strconv.Itoa(maxPagesLimit)) {
					t.Fatalf("error = %q, want limit %d", payload.Error, maxPagesLimit)
				}

				path, err := rateLimitPath(tc.action)
				if err != nil {
					t.Fatalf("rateLimitPath: %v", err)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("invalid --max-pages consumed rate limit, stat err=%v", err)
				}
			})
		}
	}
}

func TestExecute_VersionReturnsJSON(t *testing.T) {
	prevArgs := os.Args
	defer func() { os.Args = prevArgs }()
	os.Args = []string{"gog-lite", "--version"}

	stdout := captureStdout(t, func() {
		if err := Execute(context.Background(), "v1.2.3"); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	var payload struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
	}
	if payload.Version != "v1.2.3" {
		t.Fatalf("version = %q, want %q", payload.Version, "v1.2.3")
	}
}

func TestExecute_HelpReturnsJSON(t *testing.T) {
	prevArgs := os.Args
	defer func() { os.Args = prevArgs }()
	os.Args = []string{"gog-lite", "--help"}

	stdout := captureStdout(t, func() {
		if err := Execute(context.Background(), "dev"); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	var payload struct {
		Help string `json:"help"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
	}
	if !strings.Contains(payload.Help, "Usage:") {
		t.Fatalf("help payload missing usage: %q", payload.Help)
	}
}

func TestExecute_SearchHelpIncludesMaxPages(t *testing.T) {
	cases := [][]string{
		{"gog-lite", "gmail", "search", "--help"},
		{"gog-lite", "calendar", "list", "--help"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			prevArgs := os.Args
			os.Args = args
			t.Cleanup(func() { os.Args = prevArgs })

			stdout := captureStdout(t, func() {
				if err := Execute(context.Background(), "dev"); err != nil {
					t.Fatalf("Execute: %v", err)
				}
			})

			var payload struct {
				Help string `json:"help"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
				t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
			}
			if !strings.Contains(payload.Help, "--max-pages") || !strings.Contains(payload.Help, "1-50") {
				t.Fatalf("help missing page cap: %q", payload.Help)
			}
		})
	}
}

func TestExecute_ParseErrorReturnsJSON(t *testing.T) {
	prevArgs := os.Args
	defer func() { os.Args = prevArgs }()
	os.Args = []string{"gog-lite", "calendar", "delete"}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = Execute(context.Background(), "dev")
	})
	if runErr == nil {
		t.Fatal("expected parse error")
	}
	if output.ExitCode(runErr) != output.ExitCodeError {
		t.Fatalf("exit code = %d, want %d", output.ExitCode(runErr), output.ExitCodeError)
	}

	var payload struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err, stderr)
	}
	if payload.Code != "invalid_arguments" {
		t.Fatalf("code = %q, want %q", payload.Code, "invalid_arguments")
	}
	if payload.Error == "" {
		t.Fatal("expected non-empty error message")
	}
}
