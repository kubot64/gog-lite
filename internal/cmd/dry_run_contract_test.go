package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestWriteCommandsDryRunContract(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	outputDir := t.TempDir()

	testedCommands := map[string]struct{}{}
	for _, tt := range []struct {
		name   string
		key    string
		run    func() (string, error)
		action string
	}{
		{
			name: "calendar create",
			key:  "calendar create",
			run: func() (string, error) {
				cmd := &CalendarCreateCmd{
					Account: "a@example.com",
					Title:   "Team sync",
					Start:   "2026-03-01T10:00:00+09:00",
					End:     "2026-03-01T11:00:00+09:00",
				}

				var err error
				stdout := captureStdout(t, func() {
					err = cmd.Run(context.Background(), &RootFlags{DryRun: true})
				})

				return stdout, err
			},
			action: "calendar.create",
		},
		{
			name: "docs create",
			key:  "docs create",
			run: func() (string, error) {
				cmd := &DocsCreateCmd{
					Account: "a@example.com",
					Title:   "Draft",
					Content: "hello",
				}

				var err error
				stdout := captureStdout(t, func() {
					err = cmd.Run(context.Background(), &RootFlags{DryRun: true})
				})

				return stdout, err
			},
			action: "docs.create",
		},
		{
			name: "docs write",
			key:  "docs write",
			run: func() (string, error) {
				cmd := &DocsWriteCmd{
					Account: "a@example.com",
					DocID:   "doc-123",
					Content: "updated",
				}

				var err error
				stdout := captureStdout(t, func() {
					err = cmd.Run(context.Background(), &RootFlags{DryRun: true})
				})

				return stdout, err
			},
			action: "docs.write",
		},
		{
			name: "docs export",
			key:  "docs export",
			run: func() (string, error) {
				cmd := &DocsExportCmd{
					Account: "a@example.com",
					DocID:   "doc-123",
					Format:  "pdf",
					Output:  filepath.Join(outputDir, "doc.pdf"),
				}

				var err error
				stdout := captureStdout(t, func() {
					err = cmd.Run(context.Background(), &RootFlags{
						DryRun:          true,
						AllowedOutputDir: outputDir,
					})
				})

				return stdout, err
			},
			action: "docs.export",
		},
		{
			name: "sheets update",
			key:  "sheets update",
			run: func() (string, error) {
				cmd := &SheetsUpdateCmd{
					Account:       "a@example.com",
					SpreadsheetID: "sp-123",
					Range:         "Sheet1!A1:B1",
					Values:        `[["Alice",30]]`,
				}

				var err error
				stdout := captureStdout(t, func() {
					err = cmd.Run(context.Background(), &RootFlags{DryRun: true})
				})

				return stdout, err
			},
			action: "sheets.update",
		},
		{
			name: "slides write",
			key:  "slides write",
			run: func() (string, error) {
				cmd := &SlidesWriteCmd{
					Account:        "a@example.com",
					PresentationID: "pres-123",
					Find:           "{{NAME}}",
					Replace:        "Alice",
				}

				var err error
				stdout := captureStdout(t, func() {
					err = cmd.Run(context.Background(), &RootFlags{DryRun: true})
				})

				return stdout, err
			},
			action: "slides.write",
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			stdout, err := tt.run()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var payload struct {
				DryRun bool           `json:"dry_run"`
				Action string         `json:"action"`
				Params map[string]any `json:"params"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
				t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
			}
			if !payload.DryRun {
				t.Fatalf("expected dry_run=true for %s", tt.key)
			}
			if payload.Action != tt.action {
				t.Fatalf("action = %q, want %q", payload.Action, tt.action)
			}
			if len(payload.Params) == 0 {
				t.Fatalf("expected params in dry-run payload for %s", tt.key)
			}

			testedCommands[tt.key] = struct{}{}
		})
	}

	readmeDryRuns, err := readmeDryRunCommands()
	if err != nil {
		t.Fatalf("read README dry-run commands: %v", err)
	}

	for _, cmd := range readmeDryRuns {
		if _, ok := testedCommands[cmd]; !ok {
			t.Fatalf("README dry-run command %q is not covered by this contract test", cmd)
		}
	}
}

func readmeDryRunCommands() ([]string, error) {
	b, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`(?m)^gog-lite --dry-run ([a-z-]+) ([a-z-]+)\b`)
	matches := re.FindAllStringSubmatch(string(b), -1)
	if len(matches) == 0 {
		return nil, nil
	}

	seen := map[string]struct{}{}
	commands := make([]string, 0, len(matches))
	for _, match := range matches {
		cmd := match[1] + " " + match[2]
		if _, ok := seen[cmd]; ok {
			continue
		}
		seen[cmd] = struct{}{}
		commands = append(commands, cmd)
	}

	slices.Sort(commands)

	return commands, nil
}
