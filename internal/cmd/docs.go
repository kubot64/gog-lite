package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/morikubo-takashi/gog-lite/internal/googleapi"
	"github.com/morikubo-takashi/gog-lite/internal/output"
)

// DocsCmd groups Docs subcommands.
type DocsCmd struct {
	Info        DocsInfoCmd        `cmd:"" help:"Get document metadata."`
	Cat         DocsCatCmd         `cmd:"" help:"Print document text content."`
	Create      DocsCreateCmd      `cmd:"" help:"Create a new document."`
	Export      DocsExportCmd      `cmd:"" help:"Export a document to PDF/DOCX/TXT."`
	Write       DocsWriteCmd       `cmd:"" help:"Write content to a document."`
	FindReplace DocsFindReplaceCmd `cmd:"" help:"Find and replace text in a document."`
}

// DocsInfoCmd gets document metadata.
type DocsInfoCmd struct {
	Account string `name:"account" required:"" short:"a" help:"Google account email."`
	DocID   string `name:"doc-id" required:"" help:"Google Docs document ID."`
}

func (c *DocsInfoCmd) Run(ctx context.Context, _ *RootFlags) error {
	svc, err := googleapi.NewDocs(ctx, c.Account)
	if err != nil {
		return docsAuthError(err)
	}

	doc, err := svc.Documents.Get(c.DocID).Do()
	if err != nil {
		return writeGoogleAPIError("docs_info_error", err)
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"id":             doc.DocumentId,
		"title":          doc.Title,
		"revision_id":    doc.RevisionId,
		"document_style": doc.DocumentStyle,
	})
}

// DocsCatCmd prints document text content.
type DocsCatCmd struct {
	Account  string `name:"account" required:"" short:"a" help:"Google account email."`
	DocID    string `name:"doc-id" required:"" help:"Google Docs document ID."`
	MaxBytes int    `name:"max-bytes" default:"2000000" help:"Maximum bytes to return."`
}

func (c *DocsCatCmd) Run(ctx context.Context, _ *RootFlags) error {
	svc, err := googleapi.NewDocs(ctx, c.Account)
	if err != nil {
		return docsAuthError(err)
	}

	doc, err := svc.Documents.Get(c.DocID).Do()
	if err != nil {
		return writeGoogleAPIError("docs_cat_error", err)
	}

	text, truncated := truncateText(docsPlainText(doc), c.MaxBytes)

	return output.WriteJSON(os.Stdout, map[string]any{
		"id":        doc.DocumentId,
		"title":     doc.Title,
		"content":   text,
		"truncated": truncated,
	})
}

func truncateText(text string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text, false
	}

	return text[:maxBytes], true
}

// DocsCreateCmd creates a new document.
type DocsCreateCmd struct {
	Account      string `name:"account" required:"" short:"a" help:"Google account email."`
	Title        string `name:"title" required:"" help:"Document title."`
	Content      string `name:"content" help:"Initial document content."`
	ContentStdin bool   `name:"content-stdin" help:"Read initial content from stdin."`
}

func (c *DocsCreateCmd) Run(ctx context.Context, root *RootFlags) error {
	content := c.Content

	if c.ContentStdin {
		s, err := readStdinWithLimit(maxStdinBytes)
		if err != nil {
			return output.WriteError(output.ExitCodeError, "stdin_error", fmt.Sprintf("read stdin: %v", err))
		}

		content = s
	}

	dryRun := root.DryRun
	if dryRun {
		if err := appendAuditLog(root.AuditLog, auditEntry{
			Action:  "docs.create",
			Account: normalizeEmail(c.Account),
			Target:  c.Title,
			DryRun:  true,
		}); err != nil {
			return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
		}
		return output.WriteJSON(os.Stdout, map[string]any{
			"dry_run": true,
			"action":  "docs.create",
			"params": map[string]any{
				"account":        c.Account,
				"title":          c.Title,
				"content_length": len(content),
			},
		})
	}

	docSvc, err := googleapi.NewDocs(ctx, c.Account)
	if err != nil {
		return docsAuthError(err)
	}

	doc := &docs.Document{Title: c.Title}

	created, err := docSvc.Documents.Create(doc).Do()
	if err != nil {
		return writeGoogleAPIError("docs_create_error", err)
	}

	// If initial content provided, insert it.
	if strings.TrimSpace(content) != "" {
		req := &docs.BatchUpdateDocumentRequest{
			Requests: []*docs.Request{
				{
					InsertText: &docs.InsertTextRequest{
						Text:     content,
						Location: &docs.Location{Index: 1},
					},
				},
			},
		}

		if _, err := docSvc.Documents.BatchUpdate(created.DocumentId, req).Do(); err != nil {
			return writeGoogleAPIError("docs_write_error", err)
		}
	}
	if err := appendAuditLog(root.AuditLog, auditEntry{
		Action:  "docs.create",
		Account: normalizeEmail(c.Account),
		Target:  created.DocumentId,
		DryRun:  false,
	}); err != nil {
		return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"id":    created.DocumentId,
		"title": created.Title,
		"url":   fmt.Sprintf("https://docs.google.com/document/d/%s/edit", created.DocumentId),
	})
}

// DocsExportCmd exports a document to PDF, DOCX, TXT, ODT, or HTML.
type DocsExportCmd struct {
	Account   string `name:"account" required:"" short:"a" help:"Google account email."`
	DocID     string `name:"doc-id" required:"" help:"Google Docs document ID."`
	Format    string `name:"format" required:"" help:"Export format: pdf, docx, txt, odt, html."`
	Output    string `name:"output" required:"" help:"Output file path."`
	Overwrite bool   `name:"overwrite" help:"Allow overwriting an existing output file (default: disabled)."`
}

var exportMIMETypes = map[string]string{
	"pdf":  "application/pdf",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"txt":  "text/plain",
	"odt":  "application/vnd.oasis.opendocument.text",
	"html": "text/html",
}

func (c *DocsExportCmd) Run(ctx context.Context, root *RootFlags) error {
	mimeType, ok := exportMIMETypes[strings.ToLower(c.Format)]
	if !ok {
		return output.WriteError(output.ExitCodeError, "invalid_format",
			fmt.Sprintf("unsupported format %q; use pdf, docx, txt, odt, or html", c.Format))
	}

	dryRun := root.DryRun
	if dryRun {
		if err := appendAuditLog(root.AuditLog, auditEntry{
			Action:  "docs.export",
			Account: normalizeEmail(c.Account),
			Target:  c.Output,
			DryRun:  true,
		}); err != nil {
			return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
		}
		return output.WriteJSON(os.Stdout, map[string]any{
			"dry_run": true,
			"action":  "docs.export",
			"params": map[string]any{
				"account":   c.Account,
				"doc_id":    c.DocID,
				"format":    c.Format,
				"output":    c.Output,
				"overwrite": c.Overwrite,
			},
		})
	}

	driveSvc, err := googleapi.NewDrive(ctx, c.Account)
	if err != nil {
		return docsAuthError(err)
	}

	resp, err := driveSvc.Files.Export(c.DocID, mimeType).Download()
	if err != nil {
		return writeGoogleAPIError("docs_export_error", err)
	}

	defer resp.Body.Close()

	written, err := writeFileAtomically(c.Output, resp.Body, c.Overwrite)
	if err != nil {
		return output.WriteError(output.ExitCodeError, "file_write_error", fmt.Sprintf("write output file: %v", err))
	}
	if err := appendAuditLog(root.AuditLog, auditEntry{
		Action:  "docs.export",
		Account: normalizeEmail(c.Account),
		Target:  c.Output,
		DryRun:  false,
	}); err != nil {
		return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"exported":      true,
		"doc_id":        c.DocID,
		"format":        c.Format,
		"output":        c.Output,
		"bytes_written": written,
	})
}

// DocsWriteCmd writes content to a document.
type DocsWriteCmd struct {
	Account        string `name:"account" required:"" short:"a" help:"Google account email."`
	DocID          string `name:"doc-id" required:"" help:"Google Docs document ID."`
	Content        string `name:"content" help:"Content to write."`
	ContentStdin   bool   `name:"content-stdin" help:"Read content from stdin."`
	Replace        bool   `name:"replace" help:"Replace all existing content."`
	ConfirmReplace bool   `name:"confirm-replace" help:"Required confirmation flag when using --replace."`
}

func (c *DocsWriteCmd) Run(ctx context.Context, root *RootFlags) error {
	content := c.Content

	if c.ContentStdin {
		s, err := readStdinWithLimit(maxStdinBytes)
		if err != nil {
			return output.WriteError(output.ExitCodeError, "stdin_error", fmt.Sprintf("read stdin: %v", err))
		}

		content = s
	}

	dryRun := root.DryRun
	if c.Replace && !c.ConfirmReplace {
		return output.WriteError(output.ExitCodeError, "replace_requires_confirmation",
			"--replace requires --confirm-replace to reduce destructive mistakes")
	}

	if dryRun {
		if err := appendAuditLog(root.AuditLog, auditEntry{
			Action:  "docs.write",
			Account: normalizeEmail(c.Account),
			Target:  c.DocID,
			DryRun:  true,
		}); err != nil {
			return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
		}
		return output.WriteJSON(os.Stdout, map[string]any{
			"dry_run": true,
			"action":  "docs.write",
			"params": map[string]any{
				"account":         c.Account,
				"doc_id":          c.DocID,
				"content_length":  len(content),
				"replace":         c.Replace,
				"confirm_replace": c.ConfirmReplace,
			},
		})
	}

	docSvc, err := googleapi.NewDocs(ctx, c.Account)
	if err != nil {
		return docsAuthError(err)
	}

	var requests []*docs.Request

	if c.Replace {
		// Get document length to delete all content.
		doc, err := docSvc.Documents.Get(c.DocID).Do()
		if err != nil {
			return writeGoogleAPIError("docs_get_error", err)
		}

		docLen := docBodyLength(doc)

		if docLen > 1 {
			requests = append(requests, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{
						StartIndex: 1,
						EndIndex:   docLen,
					},
				},
			})
		}
	}

	if strings.TrimSpace(content) != "" {
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Text:     content,
				Location: &docs.Location{Index: 1},
			},
		})
	}

	if len(requests) == 0 {
		return output.WriteJSON(os.Stdout, map[string]any{
			"written": false,
			"doc_id":  c.DocID,
			"reason":  "no content to write",
		})
	}

	if _, err := docSvc.Documents.BatchUpdate(c.DocID, &docs.BatchUpdateDocumentRequest{
		Requests: requests,
	}).Do(); err != nil {
		return writeGoogleAPIError("docs_write_error", err)
	}
	if err := appendAuditLog(root.AuditLog, auditEntry{
		Action:  "docs.write",
		Account: normalizeEmail(c.Account),
		Target:  c.DocID,
		DryRun:  false,
	}); err != nil {
		return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"written": true,
		"doc_id":  c.DocID,
		"replace": c.Replace,
	})
}

// DocsFindReplaceCmd performs find-and-replace in a document.
type DocsFindReplaceCmd struct {
	Account   string `name:"account" required:"" short:"a" help:"Google account email."`
	DocID     string `name:"doc-id" required:"" help:"Google Docs document ID."`
	Find      string `name:"find" required:"" help:"Text to find."`
	Replace   string `name:"replace" required:"" help:"Replacement text."`
	MatchCase bool   `name:"match-case" help:"Case-sensitive matching."`
}

func (c *DocsFindReplaceCmd) Run(ctx context.Context, root *RootFlags) error {
	dryRun := root.DryRun

	if dryRun {
		if err := appendAuditLog(root.AuditLog, auditEntry{
			Action:  "docs.find_replace",
			Account: normalizeEmail(c.Account),
			Target:  c.DocID,
			DryRun:  true,
		}); err != nil {
			return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
		}
		return output.WriteJSON(os.Stdout, map[string]any{
			"dry_run": true,
			"action":  "docs.find_replace",
			"params": map[string]any{
				"account":    c.Account,
				"doc_id":     c.DocID,
				"find":       c.Find,
				"replace":    c.Replace,
				"match_case": c.MatchCase,
			},
		})
	}

	docSvc, err := googleapi.NewDocs(ctx, c.Account)
	if err != nil {
		return docsAuthError(err)
	}

	req := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{
			{
				ReplaceAllText: &docs.ReplaceAllTextRequest{
					ContainsText: &docs.SubstringMatchCriteria{
						Text:      c.Find,
						MatchCase: c.MatchCase,
					},
					ReplaceText: c.Replace,
				},
			},
		},
	}

	resp, err := docSvc.Documents.BatchUpdate(c.DocID, req).Do()
	if err != nil {
		return writeGoogleAPIError("docs_find_replace_error", err)
	}
	if err := appendAuditLog(root.AuditLog, auditEntry{
		Action:  "docs.find_replace",
		Account: normalizeEmail(c.Account),
		Target:  c.DocID,
		DryRun:  false,
	}); err != nil {
		return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
	}

	occurrences := int64(0)
	if resp != nil && len(resp.Replies) > 0 && resp.Replies[0].ReplaceAllText != nil {
		occurrences = resp.Replies[0].ReplaceAllText.OccurrencesChanged
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"replaced":    true,
		"doc_id":      c.DocID,
		"find":        c.Find,
		"replace":     c.Replace,
		"occurrences": occurrences,
	})
}

// docsPlainText extracts plain text from a Google Docs document.
func docsPlainText(doc *docs.Document) string {
	if doc.Body == nil {
		return ""
	}

	var sb strings.Builder

	for _, elem := range doc.Body.Content {
		if elem.Paragraph == nil {
			continue
		}

		for _, pe := range elem.Paragraph.Elements {
			if pe.TextRun != nil {
				sb.WriteString(pe.TextRun.Content)
			}
		}
	}

	return sb.String()
}

// docBodyLength returns the index of the last character in the document body.
func docBodyLength(doc *docs.Document) int64 {
	if doc.Body == nil || len(doc.Body.Content) == 0 {
		return 1
	}

	last := doc.Body.Content[len(doc.Body.Content)-1]

	return last.EndIndex
}

func writeFileAtomically(outputPath string, src io.Reader, overwrite bool) (int64, error) {
	if strings.TrimSpace(outputPath) == "" {
		return 0, fmt.Errorf("output path is empty")
	}

	if !overwrite {
		if _, err := os.Stat(outputPath); err == nil {
			return 0, fmt.Errorf("output file already exists; pass --overwrite to replace it")
		} else if !os.IsNotExist(err) {
			return 0, fmt.Errorf("check output file: %w", err)
		}
	}

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, fmt.Errorf("ensure output dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".gog-lite-export-*")
	if err != nil {
		return 0, fmt.Errorf("create temp file: %w", err)
	}

	tmpPath := tmp.Name()
	cleanupTmp := true
	defer func() {
		_ = tmp.Close()
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	written, err := io.Copy(tmp, src)
	if err != nil {
		return 0, err
	}
	if err := tmp.Sync(); err != nil {
		return 0, fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("close temp file: %w", err)
	}

	if overwrite {
		if err := os.Rename(tmpPath, outputPath); err != nil {
			return 0, fmt.Errorf("replace output file: %w", err)
		}
		cleanupTmp = false
		return written, nil
	}

	if _, err := os.Stat(outputPath); err == nil {
		return 0, fmt.Errorf("output file already exists; pass --overwrite to replace it")
	} else if !os.IsNotExist(err) {
		return 0, fmt.Errorf("check output file: %w", err)
	}

	if err := os.Rename(tmpPath, outputPath); err != nil {
		return 0, fmt.Errorf("commit output file: %w", err)
	}
	cleanupTmp = false

	return written, nil
}

func docsAuthError(err error) error {
	var authErr *googleapi.AuthRequiredError
	if isAuthErr(err, &authErr) {
		return output.WriteError(output.ExitCodeAuth, "auth_required", err.Error())
	}

	return output.WriteError(output.ExitCodeError, "docs_error", err.Error())
}
