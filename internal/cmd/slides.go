package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/api/slides/v1"

	"github.com/kubot64/gog-lite/internal/googleapi"
	"github.com/kubot64/gog-lite/internal/output"
)

// SlidesCmd groups Slides subcommands.
type SlidesCmd struct {
	Info  SlidesInfoCmd  `cmd:"" help:"Get presentation metadata."`
	Get   SlidesGetCmd   `cmd:"" help:"Get slide content as text."`
	Write SlidesWriteCmd `cmd:"" help:"Replace text in a presentation."`
}

// SlidesInfoCmd gets presentation metadata.
type SlidesInfoCmd struct {
	Account        string `name:"account" required:"" short:"a" help:"Google account email."`
	PresentationID string `name:"presentation-id" required:"" help:"Google Slides presentation ID."`
}

func (c *SlidesInfoCmd) Run(ctx context.Context, _ *RootFlags) error {
	svc, err := googleapi.NewSlidesReadOnly(ctx, c.Account)
	if err != nil {
		return slidesAuthError(err)
	}

	pres, err := svc.Presentations.Get(c.PresentationID).Do()
	if err != nil {
		return writeGoogleAPIError("slides_info_error", err)
	}

	type slideInfo struct {
		ObjectID    string `json:"object_id"`
		SlideNumber int    `json:"slide_number"`
	}

	slideInfos := make([]slideInfo, 0, len(pres.Slides))
	for i, s := range pres.Slides {
		slideInfos = append(slideInfos, slideInfo{
			ObjectID:    s.ObjectId,
			SlideNumber: i + 1,
		})
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"presentation_id": pres.PresentationId,
		"title":           pres.Title,
		"url":             fmt.Sprintf("https://docs.google.com/presentation/d/%s/edit", pres.PresentationId),
		"slide_count":     len(pres.Slides),
		"slides":          slideInfos,
	})
}

// SlidesGetCmd gets slide content as text.
type SlidesGetCmd struct {
	Account        string `name:"account" required:"" short:"a" help:"Google account email."`
	PresentationID string `name:"presentation-id" required:"" help:"Google Slides presentation ID."`
	PageID         string `name:"page-id" help:"Specific slide object ID to retrieve. Omit for all slides."`
}

func (c *SlidesGetCmd) Run(ctx context.Context, _ *RootFlags) error {
	if err := enforceRateLimit("slides.get", 120, time.Minute); err != nil {
		return output.WriteError(output.ExitCodeError, "rate_limited", err.Error())
	}

	svc, err := googleapi.NewSlidesReadOnly(ctx, c.Account)
	if err != nil {
		return slidesAuthError(err)
	}

	if c.PageID != "" {
		page, err := svc.Presentations.Pages.Get(c.PresentationID, c.PageID).Do()
		if err != nil {
			return writeGoogleAPIError("slides_get_error", err)
		}

		return output.WriteJSON(os.Stdout, map[string]any{
			"object_id": page.ObjectId,
			"texts":     extractPageTexts(page.PageElements),
		})
	}

	pres, err := svc.Presentations.Get(c.PresentationID).Do()
	if err != nil {
		return writeGoogleAPIError("slides_get_error", err)
	}

	type slideContent struct {
		ObjectID    string   `json:"object_id"`
		SlideNumber int      `json:"slide_number"`
		Texts       []string `json:"texts"`
	}

	slideContents := make([]slideContent, 0, len(pres.Slides))
	for i, s := range pres.Slides {
		slideContents = append(slideContents, slideContent{
			ObjectID:    s.ObjectId,
			SlideNumber: i + 1,
			Texts:       extractPageTexts(s.PageElements),
		})
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"presentation_id": pres.PresentationId,
		"slides":          slideContents,
	})
}

// extractPageTexts extracts text content from slide page elements.
func extractPageTexts(elements []*slides.PageElement) []string {
	var texts []string
	for _, elem := range elements {
		if elem.Shape == nil || elem.Shape.Text == nil {
			continue
		}
		for _, te := range elem.Shape.Text.TextElements {
			if te.TextRun != nil && te.TextRun.Content != "" {
				texts = append(texts, te.TextRun.Content)
			}
		}
	}
	return texts
}

// SlidesWriteCmd replaces text in a presentation.
type SlidesWriteCmd struct {
	Account        string `name:"account" required:"" short:"a" help:"Google account email."`
	PresentationID string `name:"presentation-id" required:"" help:"Google Slides presentation ID."`
	Find           string `name:"find" required:"" help:"Text to find."`
	Replace        string `name:"replace" required:"" help:"Replacement text."`
	MatchCase      bool   `name:"match-case" help:"Case-sensitive matching (default: false)."`
}

func (c *SlidesWriteCmd) Run(ctx context.Context, root *RootFlags) error {
	if err := enforceActionPolicy(c.Account, "slides.write"); err != nil {
		return output.WriteError(output.ExitCodePermission, "policy_denied", err.Error())
	}

	if root.DryRun {
		if err := appendAuditLog(root.AuditLog, auditEntry{
			Action:  "slides.write",
			Account: normalizeEmail(c.Account),
			Target:  c.PresentationID,
			DryRun:  true,
		}); err != nil {
			return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
		}
		return output.WriteJSON(os.Stdout, map[string]any{
			"dry_run": true,
			"action":  "slides.write",
			"params": map[string]any{
				"account":         c.Account,
				"presentation_id": c.PresentationID,
				"find":            c.Find,
				"replace":         c.Replace,
				"match_case":      c.MatchCase,
			},
		})
	}

	svc, err := googleapi.NewSlidesWrite(ctx, c.Account)
	if err != nil {
		return slidesAuthError(err)
	}

	req := &slides.BatchUpdatePresentationRequest{
		Requests: []*slides.Request{
			{
				ReplaceAllText: &slides.ReplaceAllTextRequest{
					ContainsText: &slides.SubstringMatchCriteria{
						Text:      c.Find,
						MatchCase: c.MatchCase,
					},
					ReplaceText: c.Replace,
				},
			},
		},
	}

	resp, err := svc.Presentations.BatchUpdate(c.PresentationID, req).Do()
	if err != nil {
		return writeGoogleAPIError("slides_write_error", err)
	}

	if err := appendAuditLog(root.AuditLog, auditEntry{
		Action:  "slides.write",
		Account: normalizeEmail(c.Account),
		Target:  c.PresentationID,
		DryRun:  false,
	}); err != nil {
		return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
	}

	occurrences := int64(0)
	if resp != nil && len(resp.Replies) > 0 && resp.Replies[0].ReplaceAllText != nil {
		occurrences = resp.Replies[0].ReplaceAllText.OccurrencesChanged
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"presentation_id":    c.PresentationID,
		"occurrences_changed": occurrences,
	})
}

func slidesAuthError(err error) error {
	var authErr *googleapi.AuthRequiredError
	if isAuthErr(err, &authErr) {
		return output.WriteError(output.ExitCodeAuth, "auth_required", err.Error())
	}
	return output.WriteError(output.ExitCodeError, "slides_error", err.Error())
}
