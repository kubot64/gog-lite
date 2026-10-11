package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/docs/v1"

	"github.com/kubot64/gog-lite/internal/output"
)

func TestDocsPlainText_NilBody(t *testing.T) {
	doc := &docs.Document{}
	if got := docsPlainText(doc); got != "" {
		t.Errorf("want empty string, got %q", got)
	}
}

func TestDocsPlainText_SingleRun(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{Paragraph: &docs.Paragraph{
					Elements: []*docs.ParagraphElement{
						{TextRun: &docs.TextRun{Content: "Hello, World!\n"}},
					},
				}},
			},
		},
	}
	if got := docsPlainText(doc); got != "Hello, World!\n" {
		t.Errorf("got %q, want %q", got, "Hello, World!\n")
	}
}

func TestDocsPlainText_MultipleRuns(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{Paragraph: &docs.Paragraph{
					Elements: []*docs.ParagraphElement{
						{TextRun: &docs.TextRun{Content: "Bold"}},
						{TextRun: &docs.TextRun{Content: " text\n"}},
					},
				}},
			},
		},
	}
	if got := docsPlainText(doc); got != "Bold text\n" {
		t.Errorf("got %q, want %q", got, "Bold text\n")
	}
}

func TestDocsPlainText_MultipleParagraphs(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{Paragraph: &docs.Paragraph{
					Elements: []*docs.ParagraphElement{
						{TextRun: &docs.TextRun{Content: "Line 1\n"}},
					},
				}},
				// non-paragraph element (e.g. table) should be skipped
				{Paragraph: nil},
				{Paragraph: &docs.Paragraph{
					Elements: []*docs.ParagraphElement{
						{TextRun: &docs.TextRun{Content: "Line 2\n"}},
					},
				}},
			},
		},
	}
	want := "Line 1\nLine 2\n"
	if got := docsPlainText(doc); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDocsPlainText_SkipsNilTextRun(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{Paragraph: &docs.Paragraph{
					Elements: []*docs.ParagraphElement{
						{TextRun: nil}, // inline object or similar
						{TextRun: &docs.TextRun{Content: "text\n"}},
					},
				}},
			},
		},
	}
	if got := docsPlainText(doc); got != "text\n" {
		t.Errorf("got %q, want %q", got, "text\n")
	}
}

func TestDocBodyLength_NilBody(t *testing.T) {
	doc := &docs.Document{}
	if got := docBodyLength(doc); got != 1 {
		t.Errorf("want 1, got %d", got)
	}
}

func TestDocBodyLength_EmptyContent(t *testing.T) {
	doc := &docs.Document{Body: &docs.Body{Content: []*docs.StructuralElement{}}}
	if got := docBodyLength(doc); got != 1 {
		t.Errorf("want 1, got %d", got)
	}
}

func TestDocBodyLength_ReturnsLastEndIndex(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{EndIndex: 5},
				{EndIndex: 20},
				{EndIndex: 42},
			},
		},
	}
	if got := docBodyLength(doc); got != 42 {
		t.Errorf("want 42, got %d", got)
	}
}

func TestDocBodyLength_SingleElement(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{EndIndex: 100},
			},
		},
	}
	if got := docBodyLength(doc); got != 100 {
		t.Errorf("want 100, got %d", got)
	}
}

func TestTruncateText_NoLimit(t *testing.T) {
	got, truncated := truncateText("abcdef", 0)
	if got != "abcdef" {
		t.Errorf("got %q, want %q", got, "abcdef")
	}
	if truncated {
		t.Error("expected truncated=false")
	}
}

func TestTruncateText_ExactLimit(t *testing.T) {
	got, truncated := truncateText("abcdef", 6)
	if got != "abcdef" {
		t.Errorf("got %q, want %q", got, "abcdef")
	}
	if truncated {
		t.Error("expected truncated=false")
	}
}

func TestTruncateText_OverLimit(t *testing.T) {
	got, truncated := truncateText("abcdef", 4)
	if got != "abcd" {
		t.Errorf("got %q, want %q", got, "abcd")
	}
	if !truncated {
		t.Error("expected truncated=true")
	}
}

func TestWriteFileAtomically_NoOverwriteWhenFileExists(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(out, []byte("existing"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, err := writeFileAtomically(out, bytes.NewBufferString("new"), false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestWriteFileAtomically_Overwrite(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(out, []byte("existing"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	written, err := writeFileAtomically(out, bytes.NewBufferString("new-value"), true)
	if err != nil {
		t.Fatalf("writeFileAtomically: %v", err)
	}
	if written != int64(len("new-value")) {
		t.Fatalf("written=%d", written)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(got) != "new-value" {
		t.Fatalf("got %q", string(got))
	}
}

func TestEnsureWithinAllowedOutputDir(t *testing.T) {
	base := t.TempDir()
	okPath := filepath.Join(base, "sub", "out.txt")
	if err := ensureWithinAllowedOutputDir(okPath, base); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	badPath := filepath.Join(t.TempDir(), "other.txt")
	if err := ensureWithinAllowedOutputDir(badPath, base); err == nil {
		t.Fatal("expected error for path outside allowed dir")
	}
}

func TestEnsureWithinAllowedOutputDir_RejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	linkPath := filepath.Join(base, "escape")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink not supported on this environment: %v", err)
	}

	escapedPath := filepath.Join(linkPath, "out.txt")
	if err := ensureWithinAllowedOutputDir(escapedPath, base); err == nil {
		t.Fatal("expected symlink escape path to be rejected")
	}
}

func TestDocsWriteReplaceRequiresConfirmation(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &DocsWriteCmd{
		Account:        "a@example.com",
		DocID:          "doc-123",
		Content:        "hello",
		Replace:        true,
		ConfirmReplace: false, // missing confirmation
	}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{DryRun: false})
	})
	if err == nil {
		t.Fatal("expected error when --replace is set without --confirm-replace")
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
	if payload.Code != "replace_requires_confirmation" {
		t.Errorf("code = %q, want %q", payload.Code, "replace_requires_confirmation")
	}
}

func TestDocsWriteReplace_DryRunSkipsConfirmAndApproval(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &DocsWriteCmd{
		Account: "a@example.com",
		DocID:   "doc-123",
		Content: "hello",
		Replace: true,
	}
	var stdout string
	var err error
	stdout = captureStdout(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{DryRun: true})
	})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	var payload struct {
		DryRun bool   `json:"dry_run"`
		Action string `json:"action"`
		Params struct {
			ContentLength  int  `json:"content_length"`
			Replace        bool `json:"replace"`
			ConfirmReplace bool `json:"confirm_replace"`
		} `json:"params"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err2 != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err2, stdout)
	}
	if !payload.DryRun || payload.Action != "docs.write" {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Params.ContentLength != len("hello") || !payload.Params.Replace || payload.Params.ConfirmReplace {
		t.Fatalf("params = %+v", payload.Params)
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = cmd.Run(context.Background(), &RootFlags{})
	})
	if runErr == nil {
		t.Fatal("expected confirmation error after dry-run")
	}
	if output.ExitCode(runErr) != output.ExitCodeError {
		t.Fatalf("expected ExitCodeError, got %d", output.ExitCode(runErr))
	}
	var errPayload struct {
		Code string `json:"code"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errPayload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if errPayload.Code != "replace_requires_confirmation" {
		t.Fatalf("code = %q, want replace_requires_confirmation (dry-run must not consume approval)", errPayload.Code)
	}
}

func TestDocsWriteReplace_DryRunDoesNotConsumeApprovalToken(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("GOG_LITE_CLIENT_ID", "")
	t.Setenv("GOG_LITE_CLIENT_SECRET", "")

	token, _, err := issueApprovalToken("a@example.com", "docs.write.replace", time.Minute)
	if err != nil {
		t.Fatalf("issueApprovalToken: %v", err)
	}

	cmd := &DocsWriteCmd{
		Account:        "a@example.com",
		DocID:          "doc-123",
		Content:        "hello",
		Replace:        true,
		ConfirmReplace: true,
		ApprovalToken:  token,
	}
	if err := cmd.Run(context.Background(), &RootFlags{DryRun: true}); err != nil {
		t.Fatalf("dry-run: %v", err)
	}

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = cmd.Run(context.Background(), &RootFlags{})
	})
	if runErr == nil {
		t.Fatal("expected an error after the token is accepted")
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if payload.Code == "approval_required" {
		t.Fatalf("dry-run consumed the approval token: %s", stderr)
	}

	stderr = captureStderr(t, func() {
		runErr = cmd.Run(context.Background(), &RootFlags{})
	})
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
	}
	if payload.Code != "approval_required" {
		t.Fatalf("code = %q, want approval_required after the token is used", payload.Code)
	}
}

func TestDocsExport_OutputNotAllowedUsesPermissionExit(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	allowed := t.TempDir()
	outside := filepath.Join(t.TempDir(), "out.pdf")
	cmd := &DocsExportCmd{
		Account: "a@example.com",
		DocID:   "doc-123",
		Format:  "pdf",
		Output:  outside,
	}
	root := &RootFlags{AllowedOutputDir: allowed}

	for _, dryRun := range []bool{true, false} {
		root.DryRun = dryRun
		var err error
		stderr := captureStderr(t, func() {
			err = cmd.Run(context.Background(), root)
		})
		if err == nil {
			t.Fatalf("dry_run=%v: expected error", dryRun)
		}
		if output.ExitCode(err) != output.ExitCodePermission {
			t.Fatalf("dry_run=%v: exit code = %d, want %d", dryRun, output.ExitCode(err), output.ExitCodePermission)
		}
		var payload struct {
			Code string `json:"code"`
		}
		if err2 := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err2 != nil {
			t.Fatalf("parse stderr JSON: %v (got %q)", err2, stderr)
		}
		if payload.Code != "output_not_allowed" {
			t.Fatalf("dry_run=%v: code = %q, want output_not_allowed", dryRun, payload.Code)
		}
	}
}

func TestDocsExport_DryRunAllowedOutput(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	allowed := t.TempDir()
	cmd := &DocsExportCmd{
		Account: "a@example.com",
		DocID:   "doc-123",
		Format:  "pdf",
		Output:  filepath.Join(allowed, "out.pdf"),
	}
	var stdout string
	var err error
	stdout = captureStdout(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{DryRun: true, AllowedOutputDir: allowed})
	})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	var payload struct {
		DryRun bool   `json:"dry_run"`
		Action string `json:"action"`
	}
	if err2 := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err2 != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err2, stdout)
	}
	if !payload.DryRun || payload.Action != "docs.export" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestDocsFindReplaceRequiresConfirmation(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &DocsFindReplaceCmd{
		Account:            "a@example.com",
		DocID:              "doc-123",
		Find:               "old",
		Replace:            "new",
		ConfirmFindReplace: false, // missing confirmation
	}
	var err error
	stderr := captureStderr(t, func() {
		err = cmd.Run(context.Background(), &RootFlags{DryRun: false})
	})
	if err == nil {
		t.Fatal("expected error when --confirm-find-replace is not set")
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
	if payload.Code != "find_replace_requires_confirmation" {
		t.Errorf("code = %q, want %q", payload.Code, "find_replace_requires_confirmation")
	}
}
