package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/kubot64/gog-lite/internal/config"
	"github.com/kubot64/gog-lite/internal/googleauth"
	"github.com/kubot64/gog-lite/internal/secrets"
)

func TestAuthLoginCmd_TwoStepHappyPath(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("GOG_LITE_KEYRING_BACKEND", "file")
	t.Setenv("GOG_LITE_KEYRING_PASSWORD", "test-password")

	restoreDeps := setCommandDepsForTest(func(d *commandDeps) {
		d.readCredentials = func() (config.ClientCredentials, error) {
			return config.ClientCredentials{ClientID: "client-id", ClientSecret: "client-secret"}, nil
		}
		d.authStep1 = func(_ context.Context, _ config.ClientCredentials, opts googleauth.AuthorizeOptions) (googleauth.Step1Result, error) {
			if len(opts.Scopes) == 0 {
				t.Fatal("expected scopes for auth step1")
			}
			return googleauth.Step1Result{
				AuthURL:  "https://accounts.example.com/auth?state=test-state",
				NextStep: "Open the URL and paste the redirect URL.",
			}, nil
		}
		d.authStep2 = func(_ context.Context, _ config.ClientCredentials, opts googleauth.AuthorizeOptions, authURL string) (googleauth.Step2Result, error) {
			if len(opts.Scopes) == 0 {
				t.Fatal("expected scopes for auth step2")
			}
			if !strings.Contains(authURL, "code=test-code") {
				t.Fatalf("unexpected auth URL %q", authURL)
			}
			return googleauth.Step2Result{
				Email:        "you@gmail.com",
				RefreshToken: "refresh-token",
			}, nil
		}
	})
	t.Cleanup(restoreDeps)

	step1Cmd := &AuthLoginCmd{
		Account:  "you@gmail.com",
		Services: "gmail,calendar,docs",
	}
	step1Stdout := captureStdout(t, func() {
		if err := step1Cmd.Run(context.Background(), &RootFlags{}); err != nil {
			t.Fatalf("step1 run: %v", err)
		}
	})

	var step1Payload struct {
		AuthURL  string `json:"auth_url"`
		NextStep string `json:"next_step"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(step1Stdout)), &step1Payload); err != nil {
		t.Fatalf("parse step1 JSON: %v (got %q)", err, step1Stdout)
	}
	if step1Payload.AuthURL == "" || step1Payload.NextStep == "" {
		t.Fatalf("unexpected step1 payload: %+v", step1Payload)
	}

	step2Cmd := &AuthLoginCmd{
		Account:  "you@gmail.com",
		Services: "gmail,calendar,docs",
		AuthURL:  "http://127.0.0.1:8080/oauth2/callback?code=test-code&state=test-state",
	}
	step2Stdout := captureStdout(t, func() {
		if err := step2Cmd.Run(context.Background(), &RootFlags{}); err != nil {
			t.Fatalf("step2 run: %v", err)
		}
	})

	var step2Payload struct {
		Stored   bool     `json:"stored"`
		Email    string   `json:"email"`
		Services []string `json:"services"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(step2Stdout)), &step2Payload); err != nil {
		t.Fatalf("parse step2 JSON: %v (got %q)", err, step2Stdout)
	}
	if !step2Payload.Stored || step2Payload.Email != "you@gmail.com" || len(step2Payload.Services) != 3 {
		t.Fatalf("unexpected step2 payload: %+v", step2Payload)
	}

	store, err := secrets.OpenDefault()
	if err != nil {
		t.Fatalf("open secrets store: %v", err)
	}
	tok, err := store.GetToken("you@gmail.com")
	if err != nil {
		t.Fatalf("get stored token: %v", err)
	}
	if tok.RefreshToken != "refresh-token" {
		t.Fatalf("refresh_token = %q, want %q", tok.RefreshToken, "refresh-token")
	}

	listStdout := captureStdout(t, func() {
		if err := (&AuthListCmd{}).Run(context.Background(), &RootFlags{}); err != nil {
			t.Fatalf("auth list run: %v", err)
		}
	})

	var listPayload struct {
		Accounts []struct {
			Email string `json:"email"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(listStdout)), &listPayload); err != nil {
		t.Fatalf("parse auth list JSON: %v (got %q)", err, listStdout)
	}
	if len(listPayload.Accounts) != 1 || listPayload.Accounts[0].Email != "you@gmail.com" {
		t.Fatalf("unexpected auth list payload: %+v", listPayload)
	}
}

func TestGmailSendCmd_HappyPath(t *testing.T) {
	var gotRaw string
	restoreDeps := setCommandDepsForTest(func(d *commandDeps) {
		d.newGmailWriteService = func(ctx context.Context, _ string) (*gmail.Service, error) {
			return newTestGmailService(ctx, t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Fatalf("method = %s, want POST", r.Method)
				}
				if r.URL.Path != "/gmail/v1/users/me/drafts" {
					t.Fatalf("path = %q", r.URL.Path)
				}

				var req struct {
					Message struct {
						Raw string `json:"raw"`
					} `json:"message"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				gotRaw = req.Message.Raw

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

	rawBytes, err := base64.RawURLEncoding.DecodeString(gotRaw)
	if err != nil {
		t.Fatalf("decode raw message: %v", err)
	}
	rawMessage := string(rawBytes)
	if !strings.Contains(rawMessage, "To: alice@example.com\r\n") ||
		!strings.Contains(rawMessage, "Subject: Hello\r\n") ||
		!strings.Contains(rawMessage, "\r\n\r\nBody text") {
		t.Fatalf("unexpected raw message: %q", rawMessage)
	}

	var payload struct {
		DraftID   string `json:"draft_id"`
		MessageID string `json:"message_id"`
		ThreadID  string `json:"thread_id"`
		Saved     bool   `json:"saved"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
	}
	if !payload.Saved || payload.DraftID != "draft-123" || payload.MessageID != "msg-123" || payload.ThreadID != "thread-123" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payloadMap); err != nil {
		t.Fatalf("parse stdout JSON as map: %v", err)
	}
	if _, ok := payloadMap["dry_run"]; ok {
		t.Fatalf("did not expect dry_run in non-dry-run gmail send response: %+v", payloadMap)
	}
}

func TestGmailGetCmd_HappyPathAddsMetadata(t *testing.T) {
	restoreDeps := setCommandDepsForTest(func(d *commandDeps) {
		d.newGmailReadOnlyService = func(ctx context.Context, _ string) (*gmail.Service, error) {
			return newTestGmailService(ctx, t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Fatalf("method = %s, want GET", r.Method)
				}
				if r.URL.Path != "/gmail/v1/users/me/messages/msg-123" {
					t.Fatalf("path = %q", r.URL.Path)
				}

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

	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
	}
	if payload["ok"] != true || payload["resource_type"] != "message" || payload["id"] != "msg-123" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if _, ok := payload["dry_run"]; ok {
		t.Fatalf("did not expect dry_run for read-only payload: %+v", payload)
	}
}

func TestCalendarCreateCmd_HappyPath(t *testing.T) {
	var reqBody []byte
	restoreDeps := setCommandDepsForTest(func(d *commandDeps) {
		d.newCalendarWriteService = func(ctx context.Context, _ string) (*calendar.Service, error) {
			return newTestCalendarService(ctx, t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Fatalf("method = %s, want POST", r.Method)
				}
				if r.URL.Path != "/calendar/v3/calendars/primary/events" {
					t.Fatalf("path = %q", r.URL.Path)
				}

				var err error
				reqBody, err = io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":       "event-123",
					"summary":  "Team MTG",
					"htmlLink": "https://calendar.google.com/event?eid=event-123",
				})
			})
		}
	})
	t.Cleanup(restoreDeps)

	cmd := &CalendarCreateCmd{
		Account:    "you@gmail.com",
		CalendarID: "primary",
		Title:      "Team MTG",
		Start:      "2026-03-01T10:00:00+09:00",
		End:        "2026-03-01T11:00:00+09:00",
	}
	stdout := captureStdout(t, func() {
		if err := cmd.Run(context.Background(), &RootFlags{}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	if !strings.Contains(string(reqBody), `"summary":"Team MTG"`) ||
		!strings.Contains(string(reqBody), `"dateTime":"2026-03-01T10:00:00+09:00"`) ||
		!strings.Contains(string(reqBody), `"dateTime":"2026-03-01T11:00:00+09:00"`) {
		t.Fatalf("unexpected calendar request body: %s", string(reqBody))
	}

	var payload struct {
		ID         string `json:"id"`
		Summary    string `json:"summary"`
		HTMLLink   string `json:"html_link"`
		CalendarID string `json:"calendar_id"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
	}
	if payload.ID != "event-123" || payload.Summary != "Team MTG" || payload.HTMLLink == "" || payload.CalendarID != "primary" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payloadMap); err != nil {
		t.Fatalf("parse stdout JSON as map: %v", err)
	}
	if _, ok := payloadMap["dry_run"]; ok {
		t.Fatalf("did not expect dry_run in non-dry-run calendar create response: %+v", payloadMap)
	}
}

func TestCalendarGetCmd_HappyPathAddsMetadata(t *testing.T) {
	restoreDeps := setCommandDepsForTest(func(d *commandDeps) {
		d.newCalendarReadOnlyService = func(ctx context.Context, _ string) (*calendar.Service, error) {
			return newTestCalendarService(ctx, t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Fatalf("method = %s, want GET", r.Method)
				}
				if r.URL.Path != "/calendar/v3/calendars/primary/events/event-123" {
					t.Fatalf("path = %q", r.URL.Path)
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":       "event-123",
					"summary":  "Team MTG",
					"htmlLink": "https://calendar.google.com/event?eid=event-123",
				})
			})
		}
	})
	t.Cleanup(restoreDeps)

	cmd := &CalendarGetCmd{
		Account:    "you@gmail.com",
		EventID:    "event-123",
		CalendarID: "primary",
	}
	stdout := captureStdout(t, func() {
		if err := cmd.Run(context.Background(), &RootFlags{}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
	}
	if payload["ok"] != true || payload["resource_type"] != "event" || payload["id"] != "event-123" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if _, ok := payload["dry_run"]; ok {
		t.Fatalf("did not expect dry_run for read-only payload: %+v", payload)
	}
}

func TestDocsWriteCmd_HappyPath(t *testing.T) {
	var reqBody []byte
	restoreDeps := setCommandDepsForTest(func(d *commandDeps) {
		d.newDocsWriteService = func(ctx context.Context, _ string) (*docs.Service, error) {
			return newTestDocsService(ctx, t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Fatalf("method = %s, want POST", r.Method)
				}
				if r.URL.Path != "/v1/documents/doc-123:batchUpdate" {
					t.Fatalf("path = %q", r.URL.Path)
				}

				var err error
				reqBody, err = io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"documentId": "doc-123",
					"replies":    []any{},
				})
			})
		}
	})
	t.Cleanup(restoreDeps)

	cmd := &DocsWriteCmd{
		Account: "you@gmail.com",
		DocID:   "doc-123",
		Content: "Updated content",
	}
	stdout := captureStdout(t, func() {
		if err := cmd.Run(context.Background(), &RootFlags{}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	if !strings.Contains(string(reqBody), `"insertText"`) || !strings.Contains(string(reqBody), `"text":"Updated content"`) {
		t.Fatalf("unexpected docs request body: %s", string(reqBody))
	}

	var payload struct {
		Written bool   `json:"written"`
		DocID   string `json:"doc_id"`
		Replace bool   `json:"replace"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse stdout JSON: %v (got %q)", err, stdout)
	}
	if !payload.Written || payload.DocID != "doc-123" || payload.Replace {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payloadMap); err != nil {
		t.Fatalf("parse stdout JSON as map: %v", err)
	}
	if _, ok := payloadMap["dry_run"]; ok {
		t.Fatalf("did not expect dry_run in non-dry-run docs write response: %+v", payloadMap)
	}
}

func newTestGmailService(ctx context.Context, t *testing.T, handler http.HandlerFunc) (*gmail.Service, error) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)

	return gmail.NewService(ctx,
		option.WithHTTPClient(server.Client()),
		option.WithEndpoint(server.URL+"/"),
		option.WithoutAuthentication(),
	)
}

func newTestCalendarService(ctx context.Context, t *testing.T, handler http.HandlerFunc) (*calendar.Service, error) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)

	return calendar.NewService(ctx,
		option.WithHTTPClient(server.Client()),
		option.WithEndpoint(server.URL+"/calendar/v3/"),
		option.WithoutAuthentication(),
	)
}

func newTestDocsService(ctx context.Context, t *testing.T, handler http.HandlerFunc) (*docs.Service, error) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)

	return docs.NewService(ctx,
		option.WithHTTPClient(server.Client()),
		option.WithEndpoint(server.URL+"/"),
		option.WithoutAuthentication(),
	)
}
