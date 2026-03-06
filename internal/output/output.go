package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var Stderr io.Writer = os.Stderr

const (
	ExitCodeOK         = 0
	ExitCodeError      = 1
	ExitCodeAuth       = 2
	ExitCodeNotFound   = 3
	ExitCodePermission = 4
)

// ExitCodeError wraps an error with a specific exit code.
type ExitCodeErr struct {
	Code int
	Err  error
}

func (e *ExitCodeErr) Error() string {
	return e.Err.Error()
}

func (e *ExitCodeErr) Unwrap() error {
	return e.Err
}

// NewError creates an ExitCodeErr with the given code and error.
func NewError(code int, err error) *ExitCodeErr {
	return &ExitCodeErr{Code: code, Err: err}
}

// ExitCode walks the error chain and returns the exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitCodeOK
	}

	var e *ExitCodeErr
	if errors.As(err, &e) {
		return e.Code
	}

	return ExitCodeError
}

// errorPayload is the JSON structure written to stderr on errors.
type errorPayload struct {
	Error         string   `json:"error"`
	Code          string   `json:"code"`
	Retryable     bool     `json:"retryable"`
	NextAction    string   `json:"next_action,omitempty"`
	CommandHint   string   `json:"command_hint,omitempty"`
	MissingFlags  []string `json:"missing_flags,omitempty"`
	MissingTokens []string `json:"missing_tokens,omitempty"`
}

// WriteError writes a JSON error to stderr and returns an ExitCodeErr.
// The returned error should be returned from Run() to trigger os.Exit.
func WriteError(code int, codeStr, msg string) error {
	payload := errorPayload{
		Error:         msg,
		Code:          codeStr,
		Retryable:     inferRetryable(codeStr),
		NextAction:    inferNextAction(codeStr),
		CommandHint:   inferCommandHint(codeStr, msg),
		MissingFlags:  inferMissingFlags(codeStr),
		MissingTokens: inferMissingTokens(codeStr),
	}
	enc := json.NewEncoder(Stderr)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	_ = enc.Encode(payload)

	return &ExitCodeErr{Code: code, Err: fmt.Errorf("%s", msg)}
}

// Errorf writes a JSON error to stderr and returns an ExitCodeErr.
func Errorf(code int, codeStr, format string, args ...any) error {
	return WriteError(code, codeStr, fmt.Sprintf(format, args...))
}

func WriteJSON(w io.Writer, val any) error {
	if payload, ok, err := normalizeJSONObject(val); err == nil && ok {
		val = augmentSuccessPayload(payload)
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	return enc.Encode(val)
}

func augmentSuccessPayload(payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload)+6)
	for k, v := range payload {
		out[k] = v
	}

	if _, ok := out["ok"]; !ok {
		out["ok"] = true
	}

	if next, ok := out["nextPageToken"].(string); ok {
		if _, exists := out["next_page_token"]; !exists {
			out["next_page_token"] = next
		}
		if _, exists := out["has_more"]; !exists {
			out["has_more"] = strings.TrimSpace(next) != ""
		}
	}

	if _, ok := out["resource_type"]; !ok {
		if resourceType := inferResourceType(out); resourceType != "" {
			out["resource_type"] = resourceType
		}
	}

	params, _ := out["params"].(map[string]any)
	if _, ok := out["account"]; !ok && params != nil {
		if account, ok := params["account"]; ok {
			out["account"] = account
		}
	}
	if _, ok := out["target"]; !ok {
		if target := inferTarget(out, params); target != nil {
			out["target"] = target
		}
	}
	if _, ok := out["action"]; !ok {
		if action, ok := inferAction(out, params); ok {
			out["action"] = action
		}
	}

	return out
}

func normalizeJSONObject(val any) (map[string]any, bool, error) {
	if payload, ok := val.(map[string]any); ok {
		out := make(map[string]any, len(payload))
		for k, v := range payload {
			out[k] = v
		}
		return out, true, nil
	}

	b, err := json.Marshal(val)
	if err != nil {
		return nil, false, err
	}

	var payload any
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, false, err
	}

	m, ok := payload.(map[string]any)
	return m, ok, nil
}

func inferResourceType(payload map[string]any) string {
	for _, candidate := range []struct {
		key          string
		resourceType string
	}{
		{key: "accounts", resourceType: "account"},
		{key: "events", resourceType: "event"},
		{key: "calendars", resourceType: "calendar"},
		{key: "messages", resourceType: "message"},
		{key: "labels", resourceType: "label"},
		{key: "spreadsheets", resourceType: "spreadsheet"},
	} {
		if _, ok := payload[candidate.key]; ok {
			return candidate.resourceType
		}
	}

	if _, ok := payload["threadId"]; ok {
		if _, hasLabels := payload["labelIds"]; hasLabels {
			return "message"
		}
		return "thread"
	}
	if _, ok := payload["htmlLink"]; ok {
		if _, hasSummary := payload["summary"]; hasSummary {
			return "event"
		}
	}

	for _, candidate := range []struct {
		key          string
		resourceType string
	}{
		{key: "draft_id", resourceType: "draft"},
		{key: "doc_id", resourceType: "document"},
		{key: "spreadsheet", resourceType: "spreadsheet"},
		{key: "spreadsheet_id", resourceType: "spreadsheet"},
		{key: "presentation_id", resourceType: "presentation"},
		{key: "event_id", resourceType: "event"},
		{key: "calendar_id", resourceType: "calendar"},
		{key: "thread_id", resourceType: "thread"},
		{key: "message_id", resourceType: "message"},
		{key: "email", resourceType: "account"},
		{key: "output", resourceType: "file"},
		{key: "id", resourceType: "resource"},
	} {
		if _, ok := payload[candidate.key]; ok {
			return candidate.resourceType
		}
	}

	if action, ok := payload["action"].(string); ok && action != "" {
		parts := strings.Split(action, ".")
		return strings.ReplaceAll(parts[0], "_", " ")
	}

	return ""
}

func inferTarget(payload map[string]any, params map[string]any) any {
	for _, key := range []string{"target", "output", "doc_id", "spreadsheet_id", "presentation_id", "event_id", "calendar_id", "draft_id", "email", "id"} {
		if value, ok := payload[key]; ok {
			return value
		}
	}
	if params != nil {
		for _, key := range []string{"output", "doc_id", "spreadsheet_id", "presentation_id", "event_id", "calendar_id", "title"} {
			if value, ok := params[key]; ok {
				return value
			}
		}
	}

	return nil
}

func inferAction(payload map[string]any, params map[string]any) (any, bool) {
	if saved, ok := payload["saved"].(bool); ok && saved {
		return "gmail.draft", true
	}
	if exported, ok := payload["exported"].(bool); ok && exported {
		return "docs.export", true
	}
	if written, ok := payload["written"].(bool); ok && written {
		switch inferResourceType(payload) {
		case "document":
			return "docs.write", true
		}
	}
	if _, ok := payload["calendar_id"]; ok {
		if _, ok := payload["summary"]; ok {
			return "calendar.create", true
		}
	}

	return nil, false
}

func inferRetryable(code string) bool {
	return code == "rate_limited"
}

func inferNextAction(code string) string {
	switch code {
	case "auth_required":
		return "authenticate_account"
	case "credentials_missing", "credentials_error":
		return "configure_credentials"
	case "approval_required":
		return "request_approval_token"
	case "delete_requires_confirmation", "replace_requires_confirmation", "find_replace_requires_confirmation", "write_requires_confirmation":
		return "retry_with_required_flags"
	case "invalid_action", "invalid_arguments", "invalid_format", "invalid_header", "invalid_recipient", "invalid_services", "invalid_time", "invalid_ttl", "invalid_values":
		return "fix_arguments"
	case "policy_denied", "permission_denied":
		return "adjust_policy_or_permissions"
	case "not_found":
		return "verify_target"
	case "rate_limited":
		return "retry_later"
	default:
		return ""
	}
}

func inferCommandHint(code, msg string) string {
	if code != "auth_required" {
		return ""
	}

	if idx := strings.Index(msg, "run: "); idx >= 0 {
		return strings.TrimSpace(msg[idx+len("run: "):])
	}

	return ""
}

func inferMissingFlags(code string) []string {
	switch code {
	case "delete_requires_confirmation":
		return []string{"confirm-delete"}
	case "replace_requires_confirmation":
		return []string{"confirm-replace"}
	case "find_replace_requires_confirmation":
		return []string{"confirm-find-replace"}
	case "write_requires_confirmation":
		return []string{"confirm-write"}
	default:
		return nil
	}
}

func inferMissingTokens(code string) []string {
	if code == "approval_required" {
		return []string{"approval-token"}
	}

	return nil
}
