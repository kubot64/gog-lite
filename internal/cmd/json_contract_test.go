package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
)

func TestAuthApprovalTokenDryRun_JSONContract(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &AuthApprovalTokenCmd{
		Account: "a@example.com",
		Action:  "calendar.delete",
		TTL:     "10m",
	}

	stdout := captureStdout(t, func() {
		if err := cmd.Run(context.Background(), &RootFlags{DryRun: true}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	payload := decodeJSONObject(t, stdout)
	assertJSONKeys(t, payload, "action", "account", "dry_run", "ok", "params", "requires_approval_token", "requires_confirmation", "resource_type", "validation_passed", "would_call_api")

	params, ok := payload["params"].(map[string]any)
	if !ok {
		t.Fatalf("params type = %T", payload["params"])
	}
	assertJSONKeys(t, params, "account", "action", "ttl")
}

func TestGmailGet_JSONContract(t *testing.T) {
	restoreDeps := setCommandDepsForTest(func(d *commandDeps) {
		d.newGmailReadOnlyService = func(ctx context.Context, _ string) (*gmail.Service, error) {
			return newTestGmailService(ctx, t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":       "msg-123",
					"threadId": "thread-123",
					"labelIds": []string{"INBOX"},
				})
			})
		}
	})
	t.Cleanup(restoreDeps)

	cmd := &GmailGetCmd{
		Account:   "you@gmail.com",
		MessageID: "msg-123",
		Format:    "full",
	}

	stdout := captureStdout(t, func() {
		if err := cmd.Run(context.Background(), &RootFlags{}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	payload := decodeJSONObject(t, stdout)
	assertJSONKeys(t, payload, "account", "id", "labelIds", "ok", "resource_type", "target", "threadId")
}

func TestGmailThread_JSONContract(t *testing.T) {
	restoreDeps := setCommandDepsForTest(func(d *commandDeps) {
		d.newGmailReadOnlyService = func(ctx context.Context, _ string) (*gmail.Service, error) {
			return newTestGmailService(ctx, t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "thread-123",
					"messages": []map[string]any{
						{"id": "msg-123"},
					},
				})
			})
		}
	})
	t.Cleanup(restoreDeps)

	cmd := &GmailThreadCmd{
		Account:  "you@gmail.com",
		ThreadID: "thread-123",
	}

	stdout := captureStdout(t, func() {
		if err := cmd.Run(context.Background(), &RootFlags{}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	payload := decodeJSONObject(t, stdout)
	assertJSONKeys(t, payload, "account", "id", "messages", "ok", "resource_type", "target")
}

func TestGmailSend_JSONContract(t *testing.T) {
	restoreDeps := setCommandDepsForTest(func(d *commandDeps) {
		d.newGmailWriteService = func(ctx context.Context, _ string) (*gmail.Service, error) {
			return newTestGmailService(ctx, t, func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Message struct {
						Raw string `json:"raw"`
					} `json:"message"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				rawBytes, err := base64.RawURLEncoding.DecodeString(req.Message.Raw)
				if err != nil {
					t.Fatalf("decode raw message: %v", err)
				}
				if !strings.Contains(string(rawBytes), "Subject: Hello\r\n") {
					t.Fatalf("raw message = %q", string(rawBytes))
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "draft-123",
					"message": map[string]any{
						"id":       "msg-123",
						"threadId": "thread-123",
					},
				})
			})
		}
	})
	t.Cleanup(restoreDeps)

	cmd := &GmailSendCmd{
		Account: "sender@example.com",
		To:      "alice@example.com",
		Subject: "Hello",
		Body:    "Body text",
	}

	stdout := captureStdout(t, func() {
		if err := cmd.Run(context.Background(), &RootFlags{}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	payload := decodeJSONObject(t, stdout)
	assertJSONKeys(t, payload, "action", "draft_id", "message_id", "ok", "resource_type", "saved", "target", "thread_id")
}

func TestApprovalTokenInvalidTTL_JSONContract(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	cmd := &AuthApprovalTokenCmd{
		Account: "a@example.com",
		Action:  "calendar.delete",
		TTL:     "not-a-duration",
	}

	stderr := captureStderr(t, func() {
		_ = cmd.Run(context.Background(), &RootFlags{})
	})

	payload := decodeJSONObject(t, stderr)
	assertJSONKeys(t, payload, "code", "error", "next_action", "retryable")
}

func TestCalendarDeleteMissingConfirmation_JSONContract(t *testing.T) {
	cmd := &CalendarDeleteCmd{
		Account:    "a@example.com",
		CalendarID: "primary",
		EventID:    "event-123",
	}

	stderr := captureStderr(t, func() {
		_ = cmd.Run(context.Background(), &RootFlags{})
	})

	payload := decodeJSONObject(t, stderr)
	assertJSONKeys(t, payload, "code", "error", "missing_flags", "next_action", "retryable")
}

func decodeJSONObject(t *testing.T, raw string) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		t.Fatalf("parse JSON: %v (got %q)", err, raw)
	}

	return payload
}

func assertJSONKeys(t *testing.T, payload map[string]any, want ...string) {
	t.Helper()

	got := make([]string, 0, len(payload))
	for key := range payload {
		got = append(got, key)
	}
	slices.Sort(got)
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
}
