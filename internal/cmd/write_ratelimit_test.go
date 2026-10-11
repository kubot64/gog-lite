package cmd

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteCommands_RateLimitedBeforeAPI(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("GOG_LITE_CLIENT_ID", "")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "")

	orig := writeRateLimitPerMinute
	writeRateLimitPerMinute = 1
	t.Cleanup(func() { writeRateLimitPerMinute = orig })

	// The limiter runs before approval, so a missing token on a default
	// destructive action still consumes the per-action slot.
	cmds := []struct {
		name string
		run  func() error
	}{
		{
			name: "calendar.create",
			run: func() error {
				return (&CalendarCreateCmd{
					Account: "a@example.com",
					Title:   "t",
					Start:   "2026-03-01T10:00:00Z",
					End:     "2026-03-01T11:00:00Z",
				}).Run(context.Background(), &RootFlags{})
			},
		},
		{
			name: "calendar.update",
			run: func() error {
				return (&CalendarUpdateCmd{
					Account: "a@example.com",
					EventID: "evt",
					Title:   "t",
				}).Run(context.Background(), &RootFlags{})
			},
		},
		{
			name: "calendar.delete",
			run: func() error {
				return (&CalendarDeleteCmd{
					Account:       "a@example.com",
					EventID:       "evt",
					ConfirmDelete: true,
				}).Run(context.Background(), &RootFlags{})
			},
		},
		{
			name: "docs.create",
			run: func() error {
				return (&DocsCreateCmd{
					Account: "a@example.com",
					Title:   "t",
				}).Run(context.Background(), &RootFlags{})
			},
		},
		{
			name: "docs.write",
			run: func() error {
				return (&DocsWriteCmd{
					Account: "a@example.com",
					DocID:   "doc",
					Content: "hello",
				}).Run(context.Background(), &RootFlags{})
			},
		},
		{
			name: "docs.find_replace",
			run: func() error {
				return (&DocsFindReplaceCmd{
					Account:            "a@example.com",
					DocID:              "doc",
					Find:               "a",
					Replace:            "b",
					ConfirmFindReplace: true,
				}).Run(context.Background(), &RootFlags{})
			},
		},
		{
			name: "docs.export",
			run: func() error {
				return (&DocsExportCmd{
					Account: "a@example.com",
					DocID:   "doc",
					Format:  "pdf",
					Output:  filepath.Join(t.TempDir(), "out.pdf"),
				}).Run(context.Background(), &RootFlags{})
			},
		},
		{
			name: "sheets.update",
			run: func() error {
				return (&SheetsUpdateCmd{
					Account:       "a@example.com",
					SpreadsheetID: "sp",
					Range:         "Sheet1!A1",
					Values:        `[["x"]]`,
					ConfirmWrite:  true,
				}).Run(context.Background(), &RootFlags{})
			},
		},
		{
			name: "sheets.append",
			run: func() error {
				return (&SheetsAppendCmd{
					Account:       "a@example.com",
					SpreadsheetID: "sp",
					Range:         "Sheet1",
					Values:        `[["x"]]`,
				}).Run(context.Background(), &RootFlags{})
			},
		},
		{
			name: "slides.write",
			run: func() error {
				return (&SlidesWriteCmd{
					Account:        "a@example.com",
					PresentationID: "pres",
					Find:           "a",
					Replace:        "b",
					ConfirmWrite:   true,
				}).Run(context.Background(), &RootFlags{})
			},
		},
	}

	for _, tc := range cmds {
		t.Run(tc.name, func(t *testing.T) {
			var first error
			firstStderr := captureStderr(t, func() {
				first = tc.run()
			})
			if first == nil {
				t.Fatal("expected an error before credentials exist")
			}
			if code := stderrErrorCode(t, firstStderr); code == "rate_limited" {
				t.Fatalf("first call consumed the limit before a real attempt: %s", firstStderr)
			}

			var second error
			secondStderr := captureStderr(t, func() {
				second = tc.run()
			})
			if second == nil {
				t.Fatal("expected rate limit error")
			}
			if code := stderrErrorCode(t, secondStderr); code != "rate_limited" {
				t.Fatalf("code = %q, want rate_limited (stderr %s)", code, secondStderr)
			}
		})
	}
}

func TestSheetsUpdateCmd_DryRunDoesNotConsumeRateLimit(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("GOG_LITE_CLIENT_ID", "")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "")

	orig := writeRateLimitPerMinute
	writeRateLimitPerMinute = 1
	t.Cleanup(func() { writeRateLimitPerMinute = orig })

	cmd := &SheetsUpdateCmd{
		Account:       "a@example.com",
		SpreadsheetID: "sp",
		Range:         "Sheet1!A1",
		Values:        `[["x"]]`,
		ConfirmWrite:  true,
	}
	for i := 0; i < 2; i++ {
		if err := cmd.Run(context.Background(), &RootFlags{DryRun: true}); err != nil {
			t.Fatalf("dry-run %d: %v", i, err)
		}
	}

	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{})
	})
	if err == nil {
		t.Fatal("expected an auth error, not success")
	}
	if code := stderrErrorCode(t, stderr); code == "rate_limited" {
		t.Fatalf("dry-run consumed the write rate limit: %s", stderr)
	}
}

func TestRateLimitDoesNotConsumeApprovalToken(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("GOG_LITE_CLIENT_ID", "")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "")

	orig := writeRateLimitPerMinute
	writeRateLimitPerMinute = 1
	t.Cleanup(func() { writeRateLimitPerMinute = orig })

	cases := []struct {
		rateAction     string
		approvalAction string
		run            func(token string) error
	}{
		{
			rateAction:     "sheets.update",
			approvalAction: "sheets.update",
			run: func(token string) error {
				return (&SheetsUpdateCmd{
					Account:       "a@example.com",
					SpreadsheetID: "sp",
					Range:         "Sheet1!A1",
					Values:        `[["x"]]`,
					ConfirmWrite:  true,
					ApprovalToken: token,
				}).Run(context.Background(), &RootFlags{})
			},
		},
		{
			rateAction:     "calendar.delete",
			approvalAction: "calendar.delete",
			run: func(token string) error {
				return (&CalendarDeleteCmd{
					Account:       "a@example.com",
					EventID:       "evt",
					ConfirmDelete: true,
					ApprovalToken: token,
				}).Run(context.Background(), &RootFlags{})
			},
		},
		{
			rateAction:     "docs.write",
			approvalAction: "docs.write.replace",
			run: func(token string) error {
				return (&DocsWriteCmd{
					Account:        "a@example.com",
					DocID:          "doc",
					Content:        "hello",
					Replace:        true,
					ConfirmReplace: true,
					ApprovalToken:  token,
				}).Run(context.Background(), &RootFlags{})
			},
		},
		{
			rateAction:     "docs.find_replace",
			approvalAction: "docs.find_replace",
			run: func(token string) error {
				return (&DocsFindReplaceCmd{
					Account:            "a@example.com",
					DocID:              "doc",
					Find:               "a",
					Replace:            "b",
					ConfirmFindReplace: true,
					ApprovalToken:      token,
				}).Run(context.Background(), &RootFlags{})
			},
		},
		{
			rateAction:     "slides.write",
			approvalAction: "slides.write",
			run: func(token string) error {
				return (&SlidesWriteCmd{
					Account:        "a@example.com",
					PresentationID: "pres",
					Find:           "a",
					Replace:        "b",
					ConfirmWrite:   true,
					ApprovalToken:  token,
				}).Run(context.Background(), &RootFlags{})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.approvalAction, func(t *testing.T) {
			if err := enforceWriteRateLimit(tc.rateAction); err != nil {
				t.Fatalf("seed rate limit: %v", err)
			}
			token, _, err := issueApprovalToken("a@example.com", tc.approvalAction, time.Minute)
			if err != nil {
				t.Fatalf("issueApprovalToken: %v", err)
			}

			var runErr error
			stderr := captureStderr(t, func() {
				runErr = tc.run(token)
			})
			if runErr == nil {
				t.Fatal("expected rate_limited")
			}
			if code := stderrErrorCode(t, stderr); code != "rate_limited" {
				t.Fatalf("code = %q, want rate_limited (%s)", code, stderr)
			}
			if err := consumeApprovalToken("a@example.com", tc.approvalAction, token); err != nil {
				t.Fatalf("rate limit consumed the approval token: %v", err)
			}
		})
	}
}

func stderrErrorCode(t *testing.T, stderr string) string {
	t.Helper()
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err, stderr)
	}
	return payload.Code
}
