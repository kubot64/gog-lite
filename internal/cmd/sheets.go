package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"google.golang.org/api/sheets/v4"

	"github.com/kubot64/gog-lite/internal/googleapi"
	"github.com/kubot64/gog-lite/internal/output"
)

// SheetsCmd groups Sheets subcommands.
type SheetsCmd struct {
	Info   SheetsInfoCmd   `cmd:"" help:"Get spreadsheet metadata."`
	Get    SheetsGetCmd    `cmd:"" help:"Get cell values from a range."`
	Update SheetsUpdateCmd `cmd:"" help:"Update cell values in a range."`
	Append SheetsAppendCmd `cmd:"" help:"Append rows to a sheet."`
}

// SheetsInfoCmd gets spreadsheet metadata.
type SheetsInfoCmd struct {
	Account       string `name:"account" required:"" short:"a" help:"Google account email."`
	SpreadsheetID string `name:"spreadsheet-id" required:"" help:"Google Sheets spreadsheet ID."`
}

func (c *SheetsInfoCmd) Run(ctx context.Context, _ *RootFlags) error {
	svc, err := googleapi.NewSheetsReadOnly(ctx, c.Account)
	if err != nil {
		return sheetsAuthError(err)
	}

	sp, err := svc.Spreadsheets.Get(c.SpreadsheetID).Do()
	if err != nil {
		return writeGoogleAPIError("sheets_info_error", err)
	}

	type sheetInfo struct {
		SheetID     int64  `json:"sheet_id"`
		Title       string `json:"title"`
		RowCount    int64  `json:"row_count"`
		ColumnCount int64  `json:"column_count"`
	}

	sheetInfos := make([]sheetInfo, 0, len(sp.Sheets))
	for _, s := range sp.Sheets {
		var info sheetInfo
		if s.Properties != nil {
			info.Title = s.Properties.Title
			info.SheetID = s.Properties.SheetId
			if s.Properties.GridProperties != nil {
				info.RowCount = s.Properties.GridProperties.RowCount
				info.ColumnCount = s.Properties.GridProperties.ColumnCount
			}
		}
		sheetInfos = append(sheetInfos, info)
	}

	title := ""
	if sp.Properties != nil {
		title = sp.Properties.Title
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"spreadsheet_id": sp.SpreadsheetId,
		"title":          title,
		"url":            fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit", sp.SpreadsheetId),
		"sheets":         sheetInfos,
	})
}

// SheetsGetCmd gets cell values from a range.
type SheetsGetCmd struct {
	Account       string `name:"account" required:"" short:"a" help:"Google account email."`
	SpreadsheetID string `name:"spreadsheet-id" required:"" help:"Google Sheets spreadsheet ID."`
	Range         string `name:"range" required:"" help:"Cell range (e.g. Sheet1!A1:C10)."`
}

func (c *SheetsGetCmd) Run(ctx context.Context, _ *RootFlags) error {
	if err := enforceRateLimit("sheets.get", 120, time.Minute); err != nil {
		return output.WriteError(output.ExitCodeError, "rate_limited", err.Error())
	}

	svc, err := googleapi.NewSheetsReadOnly(ctx, c.Account)
	if err != nil {
		return sheetsAuthError(err)
	}

	resp, err := svc.Spreadsheets.Values.Get(c.SpreadsheetID, c.Range).Do()
	if err != nil {
		return writeGoogleAPIError("sheets_get_error", err)
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"spreadsheet_id": c.SpreadsheetID,
		"range":          resp.Range,
		"values":         resp.Values,
	})
}

// SheetsUpdateCmd updates cell values in a range.
type SheetsUpdateCmd struct {
	Account       string `name:"account" required:"" short:"a" help:"Google account email."`
	SpreadsheetID string `name:"spreadsheet-id" required:"" help:"Google Sheets spreadsheet ID."`
	Range         string `name:"range" required:"" help:"Cell range to update (e.g. Sheet1!A1:B2)."`
	Values        string `name:"values" help:"JSON array of rows (e.g. [[\"Alice\",30]])."`
	ValuesStdin   bool   `name:"values-stdin" help:"Read values JSON from stdin."`
}

func (c *SheetsUpdateCmd) Run(ctx context.Context, root *RootFlags) error {
	if err := enforceActionPolicy(c.Account, "sheets.update"); err != nil {
		return output.WriteError(output.ExitCodePermission, "policy_denied", err.Error())
	}

	valuesJSON := c.Values
	if c.ValuesStdin {
		s, err := readStdinWithLimit(maxStdinBytes)
		if err != nil {
			return output.WriteError(output.ExitCodeError, "stdin_error", fmt.Sprintf("read stdin: %v", err))
		}
		valuesJSON = s
	}

	var values [][]any
	if err := json.Unmarshal([]byte(valuesJSON), &values); err != nil {
		return output.WriteError(output.ExitCodeError, "invalid_values", fmt.Sprintf("parse values JSON: %v", err))
	}

	if root.DryRun {
		if err := appendAuditLog(root.AuditLog, auditEntry{
			Action:  "sheets.update",
			Account: normalizeEmail(c.Account),
			Target:  c.SpreadsheetID,
			DryRun:  true,
		}); err != nil {
			return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
		}
		return output.WriteJSON(os.Stdout, map[string]any{
			"dry_run": true,
			"action":  "sheets.update",
			"params": map[string]any{
				"account":        c.Account,
				"spreadsheet_id": c.SpreadsheetID,
				"range":          c.Range,
				"row_count":      len(values),
			},
		})
	}

	svc, err := googleapi.NewSheetsWrite(ctx, c.Account)
	if err != nil {
		return sheetsAuthError(err)
	}

	valueRange := &sheets.ValueRange{Values: toInterfaceSlice(values)}
	resp, err := svc.Spreadsheets.Values.Update(c.SpreadsheetID, c.Range, valueRange).
		ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return writeGoogleAPIError("sheets_update_error", err)
	}

	if err := appendAuditLog(root.AuditLog, auditEntry{
		Action:  "sheets.update",
		Account: normalizeEmail(c.Account),
		Target:  c.SpreadsheetID,
		DryRun:  false,
	}); err != nil {
		return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"spreadsheet_id":  c.SpreadsheetID,
		"updated_range":   resp.UpdatedRange,
		"updated_rows":    resp.UpdatedRows,
		"updated_columns": resp.UpdatedColumns,
		"updated_cells":   resp.UpdatedCells,
	})
}

// SheetsAppendCmd appends rows to a sheet.
type SheetsAppendCmd struct {
	Account       string `name:"account" required:"" short:"a" help:"Google account email."`
	SpreadsheetID string `name:"spreadsheet-id" required:"" help:"Google Sheets spreadsheet ID."`
	Range         string `name:"range" required:"" help:"Sheet or range to append to (e.g. Sheet1)."`
	Values        string `name:"values" help:"JSON array of rows (e.g. [[\"Bob\",25]])."`
	ValuesStdin   bool   `name:"values-stdin" help:"Read values JSON from stdin."`
}

func (c *SheetsAppendCmd) Run(ctx context.Context, root *RootFlags) error {
	if err := enforceActionPolicy(c.Account, "sheets.append"); err != nil {
		return output.WriteError(output.ExitCodePermission, "policy_denied", err.Error())
	}

	valuesJSON := c.Values
	if c.ValuesStdin {
		s, err := readStdinWithLimit(maxStdinBytes)
		if err != nil {
			return output.WriteError(output.ExitCodeError, "stdin_error", fmt.Sprintf("read stdin: %v", err))
		}
		valuesJSON = s
	}

	var values [][]any
	if err := json.Unmarshal([]byte(valuesJSON), &values); err != nil {
		return output.WriteError(output.ExitCodeError, "invalid_values", fmt.Sprintf("parse values JSON: %v", err))
	}

	if root.DryRun {
		if err := appendAuditLog(root.AuditLog, auditEntry{
			Action:  "sheets.append",
			Account: normalizeEmail(c.Account),
			Target:  c.SpreadsheetID,
			DryRun:  true,
		}); err != nil {
			return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
		}
		return output.WriteJSON(os.Stdout, map[string]any{
			"dry_run": true,
			"action":  "sheets.append",
			"params": map[string]any{
				"account":        c.Account,
				"spreadsheet_id": c.SpreadsheetID,
				"range":          c.Range,
				"row_count":      len(values),
			},
		})
	}

	svc, err := googleapi.NewSheetsWrite(ctx, c.Account)
	if err != nil {
		return sheetsAuthError(err)
	}

	valueRange := &sheets.ValueRange{Values: toInterfaceSlice(values)}
	resp, err := svc.Spreadsheets.Values.Append(c.SpreadsheetID, c.Range, valueRange).
		ValueInputOption("USER_ENTERED").InsertDataOption("INSERT_ROWS").Do()
	if err != nil {
		return writeGoogleAPIError("sheets_append_error", err)
	}

	if err := appendAuditLog(root.AuditLog, auditEntry{
		Action:  "sheets.append",
		Account: normalizeEmail(c.Account),
		Target:  c.SpreadsheetID,
		DryRun:  false,
	}); err != nil {
		return output.WriteError(output.ExitCodeError, "audit_error", err.Error())
	}

	updatedRange := ""
	updatedRows := int64(0)
	tableRange := ""
	if resp.Updates != nil {
		updatedRange = resp.Updates.UpdatedRange
		updatedRows = resp.Updates.UpdatedRows
	}
	if resp.TableRange != "" {
		tableRange = resp.TableRange
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"spreadsheet_id": c.SpreadsheetID,
		"table_range":    tableRange,
		"updated_range":  updatedRange,
		"updated_rows":   updatedRows,
	})
}

func sheetsAuthError(err error) error {
	var authErr *googleapi.AuthRequiredError
	if isAuthErr(err, &authErr) {
		return output.WriteError(output.ExitCodeAuth, "auth_required", err.Error())
	}
	return output.WriteError(output.ExitCodeError, "sheets_error", err.Error())
}

// toInterfaceSlice converts [][]any to [][]interface{} for the Sheets API.
func toInterfaceSlice(rows [][]any) [][]interface{} {
	result := make([][]interface{}, len(rows))
	for i, row := range rows {
		irow := make([]interface{}, len(row))
		for j, v := range row {
			irow[j] = v
		}
		result[i] = irow
	}
	return result
}
