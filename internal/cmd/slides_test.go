package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/api/slides/v1"

	"github.com/kubot64/gog-lite/internal/config"
	"github.com/kubot64/gog-lite/internal/output"
)

func TestExtractPageTexts_Empty(t *testing.T) {
	texts := extractPageTexts(nil)
	if len(texts) != 0 {
		t.Errorf("expected empty, got %v", texts)
	}
}

func TestExtractPageTexts_SkipsNilShape(t *testing.T) {
	elems := []*slides.PageElement{
		{Shape: nil},
	}
	texts := extractPageTexts(elems)
	if len(texts) != 0 {
		t.Errorf("expected empty for nil shape, got %v", texts)
	}
}

func TestExtractPageTexts_SkipsNilText(t *testing.T) {
	elems := []*slides.PageElement{
		{Shape: &slides.Shape{Text: nil}},
	}
	texts := extractPageTexts(elems)
	if len(texts) != 0 {
		t.Errorf("expected empty for nil text, got %v", texts)
	}
}

func TestExtractPageTexts_CollectsTextRuns(t *testing.T) {
	elems := []*slides.PageElement{
		{Shape: &slides.Shape{Text: &slides.TextContent{
			TextElements: []*slides.TextElement{
				{TextRun: &slides.TextRun{Content: "Hello "}},
				{TextRun: nil}, // should be skipped
				{TextRun: &slides.TextRun{Content: "World"}},
				{TextRun: &slides.TextRun{Content: ""}}, // empty, should be skipped
			},
		}}},
	}
	texts := extractPageTexts(elems)
	if len(texts) != 2 {
		t.Fatalf("expected 2 texts, got %d: %v", len(texts), texts)
	}
	if texts[0] != "Hello " {
		t.Errorf("texts[0] = %q, want %q", texts[0], "Hello ")
	}
	if texts[1] != "World" {
		t.Errorf("texts[1] = %q, want %q", texts[1], "World")
	}
}

func TestSlidesWriteCmd_RequiresConfirmation(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &SlidesWriteCmd{
		Account:        "a@example.com",
		PresentationID: "pres-123",
		Find:           "old",
		Replace:        "new",
		ConfirmWrite:   false, // missing confirmation
	}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{DryRun: false})
	})
	if err == nil {
		t.Fatal("expected error when --confirm-write is not set")
	}
	if output.ExitCode(err) != output.ExitCodeError {
		t.Fatalf("expected ExitCodeError, got %d", output.ExitCode(err))
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if payload.Code != "write_requires_confirmation" {
		t.Errorf("code = %q, want %q", payload.Code, "write_requires_confirmation")
	}
}

func TestSlidesWriteCmd_PolicyDenied(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if err := config.WritePolicy(config.PolicyFile{
		AllowedActions: []string{"gmail.search"},
	}); err != nil {
		t.Fatalf("WritePolicy: %v", err)
	}

	cmd := &SlidesWriteCmd{
		Account:        "a@example.com",
		PresentationID: "pres-123",
		Find:           "old",
		Replace:        "new",
		ConfirmWrite:   true,
	}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{DryRun: false})
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if output.ExitCode(err) != output.ExitCodePermission {
		t.Fatalf("expected ExitCodePermission, got %d", output.ExitCode(err))
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if payload.Code != "policy_denied" {
		t.Errorf("code = %q, want %q", payload.Code, "policy_denied")
	}
}

func TestSlidesWriteCmd_DryRun(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &SlidesWriteCmd{
		Account:        "a@example.com",
		PresentationID: "pres-123",
		Find:           "{{NAME}}",
		Replace:        "Alice",
	}
	var stdout string
	var err error
	stdout = captureStdout(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{DryRun: true})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload struct {
		DryRun bool   `json:"dry_run"`
		Action string `json:"action"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err2 != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err2, stdout)
	}
	if !payload.DryRun {
		t.Error("expected dry_run=true")
	}
	if payload.Action != "slides.write" {
		t.Errorf("action = %q, want %q", payload.Action, "slides.write")
	}
}

func TestActionRequiresApproval_SlidesWrite(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	// No policy file → uses defaultApprovalActions which includes slides.write.
	required, err := actionRequiresApproval("slides.write")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !required {
		t.Fatal("slides.write should require approval by default")
	}
}
