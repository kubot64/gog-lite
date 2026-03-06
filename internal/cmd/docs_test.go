package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

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

func TestWriteFileAtomically_FinalTargetSymlinkReplaced(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatalf("seed outside file: %v", err)
	}

	outputPath := filepath.Join(base, "export.txt")
	if err := os.Symlink(target, outputPath); err != nil {
		t.Skipf("symlink not supported on this environment: %v", err)
	}

	if _, err := writeFileAtomically(outputPath, bytes.NewBufferString("new-value"), true); err != nil {
		t.Fatalf("writeFileAtomically: %v", err)
	}

	info, err := os.Lstat(outputPath)
	if err != nil {
		t.Fatalf("lstat output: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("expected final target symlink to be replaced by a regular file")
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(got) != "new-value" {
		t.Fatalf("output = %q", string(got))
	}

	outsideGot, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read outside file: %v", err)
	}
	if string(outsideGot) != "outside" {
		t.Fatalf("outside file changed: %q", string(outsideGot))
	}
}

func TestWriteFileAtomically_ParentSymlinkEscapesWhenUnrestricted(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	linkDir := filepath.Join(base, "escape")
	if err := os.Symlink(outside, linkDir); err != nil {
		t.Skipf("symlink not supported on this environment: %v", err)
	}

	outputPath := filepath.Join(linkDir, "export.txt")
	if _, err := writeFileAtomically(outputPath, bytes.NewBufferString("escaped"), true); err != nil {
		t.Fatalf("writeFileAtomically: %v", err)
	}

	outsidePath := filepath.Join(outside, "export.txt")
	got, err := os.ReadFile(outsidePath)
	if err != nil {
		t.Fatalf("read escaped output: %v", err)
	}
	if string(got) != "escaped" {
		t.Fatalf("escaped output = %q", string(got))
	}
}

func TestDocsExportCmd_AllowedOutputDirRejectsFinalTargetSymlinkEscape(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	base := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "escaped.pdf")
	outputPath := filepath.Join(base, "export.pdf")
	if err := os.Symlink(target, outputPath); err != nil {
		t.Skipf("symlink not supported on this environment: %v", err)
	}

	cmd := &DocsExportCmd{
		Account: "a@example.com",
		DocID:   "doc-123",
		Format:  "pdf",
		Output:  outputPath,
	}
	var runErr error
	stderr := captureStderr(t, func() {
		runErr = cmd.Run(context.Background(), &RootFlags{AllowedOutputDir: base})
	})
	if runErr == nil {
		t.Fatal("expected error, got nil")
	}

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err, stderr)
	}
	if payload.Code != "output_not_allowed" {
		t.Fatalf("code = %q, want %q", payload.Code, "output_not_allowed")
	}
}

func TestDocsExportCmd_AllowedOutputDirRejectsParentSymlinkEscape(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	base := t.TempDir()
	outside := t.TempDir()
	linkDir := filepath.Join(base, "escape")
	if err := os.Symlink(outside, linkDir); err != nil {
		t.Skipf("symlink not supported on this environment: %v", err)
	}

	cmd := &DocsExportCmd{
		Account: "a@example.com",
		DocID:   "doc-123",
		Format:  "pdf",
		Output:  filepath.Join(linkDir, "export.pdf"),
	}
	var runErr error
	stderr := captureStderr(t, func() {
		runErr = cmd.Run(context.Background(), &RootFlags{AllowedOutputDir: base})
	})
	if runErr == nil {
		t.Fatal("expected error, got nil")
	}

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &payload); err != nil {
		t.Fatalf("parse stderr JSON: %v (got %q)", err, stderr)
	}
	if payload.Code != "output_not_allowed" {
		t.Fatalf("code = %q, want %q", payload.Code, "output_not_allowed")
	}
}

func TestDocsExportCmd_UnrestrictedFinalTargetSymlinkReplaced(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	base := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "escaped.pdf")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatalf("seed outside file: %v", err)
	}
	outputPath := filepath.Join(base, "export.pdf")
	if err := os.Symlink(target, outputPath); err != nil {
		t.Skipf("symlink not supported on this environment: %v", err)
	}

	restoreDeps := setCommandDepsForTest(func(d *commandDeps) {
		d.newDriveReadOnlyService = func(ctx context.Context, _ string) (*drive.Service, error) {
			return newTestDriveService(ctx, t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Fatalf("method = %s, want GET", r.Method)
				}
				if r.URL.Path != "/drive/v3/files/doc-123/export" {
					t.Fatalf("path = %q", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/pdf")
				_, _ = w.Write([]byte("pdf-data"))
			})
		}
	})
	t.Cleanup(restoreDeps)

	cmd := &DocsExportCmd{
		Account:   "a@example.com",
		DocID:     "doc-123",
		Format:    "pdf",
		Output:    outputPath,
		Overwrite: true,
	}
	stdout := captureStdout(t, func() {
		if err := cmd.Run(context.Background(), &RootFlags{}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var payload struct {
		Exported bool   `json:"exported"`
		Output   string `json:"output"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
	}
	if !payload.Exported || payload.Output != outputPath {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	info, err := os.Lstat(outputPath)
	if err != nil {
		t.Fatalf("lstat output: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("expected output symlink to be replaced by a regular file")
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(got) != "pdf-data" {
		t.Fatalf("output = %q", string(got))
	}

	outsideGot, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read outside file: %v", err)
	}
	if string(outsideGot) != "outside" {
		t.Fatalf("outside file changed: %q", string(outsideGot))
	}
}

func TestDocsExportCmd_UnrestrictedParentSymlinkWritesOutsideAllowedTree(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	base := t.TempDir()
	outside := t.TempDir()
	linkDir := filepath.Join(base, "escape")
	if err := os.Symlink(outside, linkDir); err != nil {
		t.Skipf("symlink not supported on this environment: %v", err)
	}

	restoreDeps := setCommandDepsForTest(func(d *commandDeps) {
		d.newDriveReadOnlyService = func(ctx context.Context, _ string) (*drive.Service, error) {
			return newTestDriveService(ctx, t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/pdf")
				_, _ = w.Write([]byte("pdf-data"))
			})
		}
	})
	t.Cleanup(restoreDeps)

	outputPath := filepath.Join(linkDir, "export.pdf")
	cmd := &DocsExportCmd{
		Account:   "a@example.com",
		DocID:     "doc-123",
		Format:    "pdf",
		Output:    outputPath,
		Overwrite: true,
	}
	captureStdout(t, func() {
		if err := cmd.Run(context.Background(), &RootFlags{}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	outsidePath := filepath.Join(outside, "export.pdf")
	got, err := os.ReadFile(outsidePath)
	if err != nil {
		t.Fatalf("read escaped output: %v", err)
	}
	if string(got) != "pdf-data" {
		t.Fatalf("escaped output = %q", string(got))
	}
}

func newTestDriveService(ctx context.Context, t *testing.T, handler http.HandlerFunc) (*drive.Service, error) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)

	return drive.NewService(ctx,
		option.WithHTTPClient(server.Client()),
		option.WithEndpoint(server.URL+"/drive/v3/"),
		option.WithoutAuthentication(),
	)
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
