package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/api/docs/v1"
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
