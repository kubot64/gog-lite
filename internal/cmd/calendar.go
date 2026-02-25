package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/morikubo-takashi/gog-lite/internal/googleapi"
	"github.com/morikubo-takashi/gog-lite/internal/output"
)

// CalendarCmd groups Calendar subcommands.
type CalendarCmd struct {
	Calendars CalendarCalendarsCmd `cmd:"" help:"List all calendars."`
	List      CalendarListCmd      `cmd:"" help:"List calendar events."`
	Get       CalendarGetCmd       `cmd:"" help:"Get a calendar event by ID."`
	Create    CalendarCreateCmd    `cmd:"" help:"Create a calendar event."`
	Update    CalendarUpdateCmd    `cmd:"" help:"Update a calendar event."`
	Delete    CalendarDeleteCmd    `cmd:"" help:"Delete a calendar event."`
}

// CalendarCalendarsCmd lists all calendars.
type CalendarCalendarsCmd struct {
	Account string `name:"account" required:"" short:"a" help:"Google account email."`
}

func (c *CalendarCalendarsCmd) Run(ctx context.Context, _ *RootFlags) error {
	svc, err := googleapi.NewCalendarReadOnly(ctx, c.Account)
	if err != nil {
		return calendarAuthError(err)
	}

	resp, err := svc.CalendarList.List().Do()
	if err != nil {
		return writeGoogleAPIError("calendars_error", err)
	}

	type calendarInfo struct {
		ID          string `json:"id"`
		Summary     string `json:"summary"`
		Description string `json:"description,omitempty"`
		Primary     bool   `json:"primary,omitempty"`
		AccessRole  string `json:"access_role,omitempty"`
	}

	cals := make([]calendarInfo, 0, len(resp.Items))
	for _, cal := range resp.Items {
		cals = append(cals, calendarInfo{
			ID:          cal.Id,
			Summary:     cal.Summary,
			Description: cal.Description,
			Primary:     cal.Primary,
			AccessRole:  cal.AccessRole,
		})
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"calendars": cals,
	})
}

// CalendarListCmd lists calendar events.
type CalendarListCmd struct {
	Account    string `name:"account" required:"" short:"a" help:"Google account email."`
	CalendarID string `name:"calendar-id" default:"primary" help:"Calendar ID."`
	From       string `name:"from" help:"Start time in RFC3339 format."`
	To         string `name:"to" help:"End time in RFC3339 format."`
	Max        int64  `name:"max" default:"20" help:"Maximum results."`
	AllPages   bool   `name:"all-pages" help:"Fetch all pages of results."`
	Page       string `name:"page" help:"Page token for pagination."`
	Query      string `name:"query" short:"q" help:"Free text search query."`
}

func (c *CalendarListCmd) Run(ctx context.Context, _ *RootFlags) error {
	if err := enforceRateLimit("calendar.list", 120, time.Minute); err != nil {
		return output.WriteError(output.ExitCodeError, "rate_limited", err.Error())
	}

	if err := validateRFC3339Optional("--from", c.From); err != nil {
		return output.WriteError(output.ExitCodeError, "invalid_time", err.Error())
	}

	if err := validateRFC3339Optional("--to", c.To); err != nil {
		return output.WriteError(output.ExitCodeError, "invalid_time", err.Error())
	}

	svc, err := googleapi.NewCalendarReadOnly(ctx, c.Account)
	if err != nil {
		return calendarAuthError(err)
	}

	type eventRef struct {
		ID          string `json:"id"`
		Summary     string `json:"summary,omitempty"`
		Start       string `json:"start,omitempty"`
		End         string `json:"end,omitempty"`
		Description string `json:"description,omitempty"`
		Location    string `json:"location,omitempty"`
		Status      string `json:"status,omitempty"`
	}

	events, nextPageToken, err := collectAllPages(c.AllPages, func(pageToken string) (string, []eventRef, error) {
		req := svc.Events.List(c.CalendarID).
			MaxResults(c.Max).
			SingleEvents(true).
			OrderBy("startTime")

		if c.From != "" {
			req = req.TimeMin(c.From)
		}

		if c.To != "" {
			req = req.TimeMax(c.To)
		}

		if c.Query != "" {
			req = req.Q(c.Query)
		}

		if pageToken != "" {
			req = req.PageToken(pageToken)
		} else if c.Page != "" {
			req = req.PageToken(c.Page)
		}

		resp, err := req.Do()
		if err != nil {
			return "", nil, fmt.Errorf("calendar list: %w", err)
		}

		refs := make([]eventRef, 0, len(resp.Items))
		for _, e := range resp.Items {
			ref := eventRef{
				ID:          e.Id,
				Summary:     e.Summary,
				Description: e.Description,
				Location:    e.Location,
				Status:      e.Status,
			}

			if e.Start != nil {
				ref.Start = eventTimeString(e.Start)
			}

			if e.End != nil {
				ref.End = eventTimeString(e.End)
			}

			refs = append(refs, ref)
		}

		return resp.NextPageToken, refs, nil
	})

	if err != nil {
		return writeGoogleAPIError("list_error", err)
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"events":        events,
		"nextPageToken": nextPageToken,
	})
}

// CalendarGetCmd gets a calendar event.
type CalendarGetCmd struct {
	Account    string `name:"account" required:"" short:"a" help:"Google account email."`
	EventID    string `name:"event-id" required:"" help:"Calendar event ID."`
	CalendarID string `name:"calendar-id" default:"primary" help:"Calendar ID."`
}

func (c *CalendarGetCmd) Run(ctx context.Context, _ *RootFlags) error {
	svc, err := googleapi.NewCalendarReadOnly(ctx, c.Account)
	if err != nil {
		return calendarAuthError(err)
	}

	event, err := svc.Events.Get(c.CalendarID, c.EventID).Do()
	if err != nil {
		return writeGoogleAPIError("get_error", err)
	}

	return output.WriteJSON(os.Stdout, event)
}

// CalendarCreateCmd creates a calendar event.
type CalendarCreateCmd struct {
	Account     string `name:"account" required:"" short:"a" help:"Google account email."`
	CalendarID  string `name:"calendar-id" default:"primary" help:"Calendar ID."`
	Title       string `name:"title" required:"" help:"Event title."`
	Start       string `name:"start" required:"" help:"Start time in RFC3339 format."`
	End         string `name:"end" required:"" help:"End time in RFC3339 format."`
	Description string `name:"description" help:"Event description."`
	Location    string `name:"location" help:"Event location."`
}

func (c *CalendarCreateCmd) Run(ctx context.Context, root *RootFlags) error {
	if err := validateRFC3339("--start", c.Start); err != nil {
		return output.WriteError(output.ExitCodeError, "invalid_time", err.Error())
	}

	if err := validateRFC3339("--end", c.End); err != nil {
		return output.WriteError(output.ExitCodeError, "invalid_time", err.Error())
	}

	dryRun := root.DryRun

	if dryRun {
		if err := appendAuditLog(root.AuditLog, auditEntry{
			Action:  "calendar.create",
			Account: normalizeEmail(c.Account),
			Target:  c.CalendarID,
			DryRun:  true,
		}); err != nil {
			return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
		}
		return output.WriteJSON(os.Stdout, map[string]any{
			"dry_run": true,
			"action":  "calendar.create",
			"params": map[string]any{
				"account":     c.Account,
				"calendar_id": c.CalendarID,
				"title":       c.Title,
				"start":       c.Start,
				"end":         c.End,
				"description": c.Description,
				"location":    c.Location,
			},
		})
	}

	svc, err := googleapi.NewCalendarWrite(ctx, c.Account)
	if err != nil {
		return calendarAuthError(err)
	}

	event := &calendar.Event{
		Summary:     c.Title,
		Description: c.Description,
		Location:    c.Location,
		Start:       &calendar.EventDateTime{DateTime: c.Start},
		End:         &calendar.EventDateTime{DateTime: c.End},
	}

	created, err := svc.Events.Insert(c.CalendarID, event).Do()
	if err != nil {
		return writeGoogleAPIError("create_error", err)
	}
	if err := appendAuditLog(root.AuditLog, auditEntry{
		Action:  "calendar.create",
		Account: normalizeEmail(c.Account),
		Target:  created.Id,
		DryRun:  false,
	}); err != nil {
		return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"id":          created.Id,
		"summary":     created.Summary,
		"start":       eventTimeString(created.Start),
		"end":         eventTimeString(created.End),
		"html_link":   created.HtmlLink,
		"calendar_id": c.CalendarID,
	})
}

// CalendarUpdateCmd updates a calendar event.
type CalendarUpdateCmd struct {
	Account     string `name:"account" required:"" short:"a" help:"Google account email."`
	EventID     string `name:"event-id" required:"" help:"Calendar event ID."`
	CalendarID  string `name:"calendar-id" default:"primary" help:"Calendar ID."`
	Title       string `name:"title" help:"New event title."`
	Start       string `name:"start" help:"New start time in RFC3339 format."`
	End         string `name:"end" help:"New end time in RFC3339 format."`
	Description string `name:"description" help:"New event description."`
	Location    string `name:"location" help:"New event location."`
}

func (c *CalendarUpdateCmd) Run(ctx context.Context, root *RootFlags) error {
	if c.Start != "" {
		if err := validateRFC3339("--start", c.Start); err != nil {
			return output.WriteError(output.ExitCodeError, "invalid_time", err.Error())
		}
	}

	if c.End != "" {
		if err := validateRFC3339("--end", c.End); err != nil {
			return output.WriteError(output.ExitCodeError, "invalid_time", err.Error())
		}
	}

	dryRun := root.DryRun

	if dryRun {
		if err := appendAuditLog(root.AuditLog, auditEntry{
			Action:  "calendar.update",
			Account: normalizeEmail(c.Account),
			Target:  c.EventID,
			DryRun:  true,
		}); err != nil {
			return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
		}
		return output.WriteJSON(os.Stdout, map[string]any{
			"dry_run": true,
			"action":  "calendar.update",
			"params": map[string]any{
				"account":     c.Account,
				"event_id":    c.EventID,
				"calendar_id": c.CalendarID,
				"title":       c.Title,
				"start":       c.Start,
				"end":         c.End,
				"description": c.Description,
				"location":    c.Location,
			},
		})
	}

	svc, err := googleapi.NewCalendarWrite(ctx, c.Account)
	if err != nil {
		return calendarAuthError(err)
	}

	// Fetch existing event.
	event, err := svc.Events.Get(c.CalendarID, c.EventID).Do()
	if err != nil {
		return writeGoogleAPIError("get_error", err)
	}

	if c.Title != "" {
		event.Summary = c.Title
	}

	if c.Description != "" {
		event.Description = c.Description
	}

	if c.Location != "" {
		event.Location = c.Location
	}

	if c.Start != "" {
		event.Start = &calendar.EventDateTime{DateTime: c.Start}
	}

	if c.End != "" {
		event.End = &calendar.EventDateTime{DateTime: c.End}
	}

	updated, err := svc.Events.Update(c.CalendarID, c.EventID, event).Do()
	if err != nil {
		return writeGoogleAPIError("update_error", err)
	}
	if err := appendAuditLog(root.AuditLog, auditEntry{
		Action:  "calendar.update",
		Account: normalizeEmail(c.Account),
		Target:  c.EventID,
		DryRun:  false,
	}); err != nil {
		return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"id":      updated.Id,
		"summary": updated.Summary,
		"start":   eventTimeString(updated.Start),
		"end":     eventTimeString(updated.End),
		"updated": true,
	})
}

// CalendarDeleteCmd deletes a calendar event.
type CalendarDeleteCmd struct {
	Account       string `name:"account" required:"" short:"a" help:"Google account email."`
	EventID       string `name:"event-id" required:"" help:"Calendar event ID."`
	CalendarID    string `name:"calendar-id" default:"primary" help:"Calendar ID."`
	ConfirmDelete bool   `name:"confirm-delete" help:"Required confirmation flag for delete operations."`
}

func (c *CalendarDeleteCmd) Run(ctx context.Context, root *RootFlags) error {
	dryRun := root.DryRun
	if !dryRun && !c.ConfirmDelete {
		return output.WriteError(output.ExitCodeError, "delete_requires_confirmation",
			"calendar delete requires --confirm-delete")
	}

	if dryRun {
		if err := appendAuditLog(root.AuditLog, auditEntry{
			Action:  "calendar.delete",
			Account: normalizeEmail(c.Account),
			Target:  c.EventID,
			DryRun:  true,
		}); err != nil {
			return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
		}
		return output.WriteJSON(os.Stdout, map[string]any{
			"dry_run": true,
			"action":  "calendar.delete",
			"params": map[string]any{
				"account":     c.Account,
				"event_id":    c.EventID,
				"calendar_id": c.CalendarID,
			},
		})
	}

	svc, err := googleapi.NewCalendarWrite(ctx, c.Account)
	if err != nil {
		return calendarAuthError(err)
	}

	if err := svc.Events.Delete(c.CalendarID, c.EventID).Do(); err != nil {
		return writeGoogleAPIError("delete_error", err)
	}
	if err := appendAuditLog(root.AuditLog, auditEntry{
		Action:  "calendar.delete",
		Account: normalizeEmail(c.Account),
		Target:  c.EventID,
		DryRun:  false,
	}); err != nil {
		return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"deleted":  true,
		"event_id": c.EventID,
	})
}

// eventTimeString extracts a string from a calendar EventDateTime.
func eventTimeString(edt *calendar.EventDateTime) string {
	if edt == nil {
		return ""
	}

	if edt.DateTime != "" {
		return edt.DateTime
	}

	return edt.Date
}

// validateRFC3339 validates that a string is a valid RFC3339 datetime.
func validateRFC3339(flag, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", flag)
	}

	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("%s %q is not a valid RFC3339 datetime (e.g. 2026-03-01T10:00:00Z): %v", flag, value, err)
	}

	return nil
}

// validateRFC3339Optional validates that a string is a valid RFC3339 datetime if non-empty.
func validateRFC3339Optional(flag, value string) error {
	if value == "" {
		return nil
	}

	return validateRFC3339(flag, value)
}

func calendarAuthError(err error) error {
	var authErr *googleapi.AuthRequiredError
	if isAuthErr(err, &authErr) {
		return output.WriteError(output.ExitCodeAuth, "auth_required", err.Error())
	}

	return output.WriteError(output.ExitCodeError, "calendar_error", err.Error())
}
