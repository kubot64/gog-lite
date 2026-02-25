package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/morikubo-takashi/gog-lite/internal/googleapi"
	"github.com/morikubo-takashi/gog-lite/internal/output"
)

// GmailCmd groups Gmail subcommands.
type GmailCmd struct {
	Search GmailSearchCmd `cmd:"" help:"Search Gmail messages."`
	Get    GmailGetCmd    `cmd:"" help:"Get a Gmail message by ID."`
	Send   GmailSendCmd   `cmd:"" help:"Send an email."`
	Thread GmailThreadCmd `cmd:"" help:"Get a Gmail thread by ID."`
	Labels GmailLabelsCmd `cmd:"" help:"List Gmail labels."`
}

// GmailSearchCmd searches Gmail messages.
type GmailSearchCmd struct {
	Account  string `name:"account" required:"" short:"a" help:"Google account email."`
	Query    string `name:"query" required:"" short:"q" help:"Gmail search query (e.g. 'is:unread')."`
	Max      int64  `name:"max" default:"20" help:"Maximum results to return."`
	AllPages bool   `name:"all-pages" help:"Fetch all pages of results."`
	Page     string `name:"page" help:"Page token for pagination."`
}

func (c *GmailSearchCmd) Run(ctx context.Context, _ *RootFlags) error {
	svc, err := googleapi.NewGmail(ctx, c.Account)
	if err != nil {
		return gmailAuthError(err)
	}

	type messageRef struct {
		ID       string `json:"id"`
		ThreadID string `json:"thread_id"`
	}

	messages, nextPageToken, err := collectAllPages(c.AllPages, func(pageToken string) (string, []messageRef, error) {
		req := svc.Users.Messages.List("me").Q(c.Query).MaxResults(c.Max)
		if pageToken != "" {
			req = req.PageToken(pageToken)
		} else if c.Page != "" {
			req = req.PageToken(c.Page)
		}

		resp, err := req.Do()
		if err != nil {
			return "", nil, fmt.Errorf("gmail search: %w", err)
		}

		refs := make([]messageRef, 0, len(resp.Messages))
		for _, m := range resp.Messages {
			refs = append(refs, messageRef{ID: m.Id, ThreadID: m.ThreadId})
		}

		return resp.NextPageToken, refs, nil
	})

	if err != nil {
		return writeGoogleAPIError("search_error", err)
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"messages":      messages,
		"nextPageToken": nextPageToken,
	})
}

// GmailGetCmd fetches a Gmail message by ID.
type GmailGetCmd struct {
	Account   string `name:"account" required:"" short:"a" help:"Google account email."`
	MessageID string `name:"message-id" required:"" help:"Gmail message ID."`
	Format    string `name:"format" default:"full" help:"Message format: full, metadata, minimal, raw."`
}

func (c *GmailGetCmd) Run(ctx context.Context, _ *RootFlags) error {
	svc, err := googleapi.NewGmail(ctx, c.Account)
	if err != nil {
		return gmailAuthError(err)
	}

	msg, err := svc.Users.Messages.Get("me", c.MessageID).Format(c.Format).Do()
	if err != nil {
		return writeGoogleAPIError("get_error", err)
	}

	return output.WriteJSON(os.Stdout, msg)
}

// GmailSendCmd sends an email.
type GmailSendCmd struct {
	Account   string `name:"account" required:"" short:"a" help:"Google account email."`
	To        string `name:"to" required:"" help:"Recipient email address."`
	Subject   string `name:"subject" required:"" help:"Email subject."`
	Body      string `name:"body" help:"Email body."`
	BodyStdin bool   `name:"body-stdin" help:"Read email body from stdin."`
	CC        string `name:"cc" help:"CC email addresses (comma-separated)."`
	BCC       string `name:"bcc" help:"BCC email addresses (comma-separated)."`
}

func (c *GmailSendCmd) Run(ctx context.Context, _ *RootFlags) error {
	body := c.Body

	if c.BodyStdin {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return output.WriteError(output.ExitCodeError, "stdin_error", fmt.Sprintf("read stdin: %v", err))
		}

		body = string(b)
	}

	svc, err := googleapi.NewGmail(ctx, c.Account)
	if err != nil {
		return gmailAuthError(err)
	}

	var headers strings.Builder
	headers.WriteString("From: " + c.Account + "\r\n")
	headers.WriteString("To: " + c.To + "\r\n")

	if c.CC != "" {
		headers.WriteString("Cc: " + c.CC + "\r\n")
	}

	if c.BCC != "" {
		headers.WriteString("Bcc: " + c.BCC + "\r\n")
	}

	headers.WriteString("Subject: " + c.Subject + "\r\n")
	headers.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	headers.WriteString("\r\n")
	headers.WriteString(body)

	raw := base64.RawURLEncoding.EncodeToString([]byte(headers.String()))

	msg, err := svc.Users.Messages.Send("me", &gmail.Message{Raw: raw}).Do()
	if err != nil {
		return writeGoogleAPIError("send_error", err)
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"id":        msg.Id,
		"thread_id": msg.ThreadId,
		"sent":      true,
	})
}

// GmailThreadCmd fetches a Gmail thread.
type GmailThreadCmd struct {
	Account  string `name:"account" required:"" short:"a" help:"Google account email."`
	ThreadID string `name:"thread-id" required:"" help:"Gmail thread ID."`
	Format   string `name:"format" default:"full" help:"Message format: full, metadata, minimal."`
}

func (c *GmailThreadCmd) Run(ctx context.Context, _ *RootFlags) error {
	svc, err := googleapi.NewGmail(ctx, c.Account)
	if err != nil {
		return gmailAuthError(err)
	}

	thread, err := svc.Users.Threads.Get("me", c.ThreadID).Format(c.Format).Do()
	if err != nil {
		return writeGoogleAPIError("thread_error", err)
	}

	return output.WriteJSON(os.Stdout, thread)
}

// GmailLabelsCmd lists Gmail labels.
type GmailLabelsCmd struct {
	Account string `name:"account" required:"" short:"a" help:"Google account email."`
}

func (c *GmailLabelsCmd) Run(ctx context.Context, _ *RootFlags) error {
	svc, err := googleapi.NewGmail(ctx, c.Account)
	if err != nil {
		return gmailAuthError(err)
	}

	resp, err := svc.Users.Labels.List("me").Do()
	if err != nil {
		return writeGoogleAPIError("labels_error", err)
	}

	type labelInfo struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type,omitempty"`
	}

	labels := make([]labelInfo, 0, len(resp.Labels))
	for _, l := range resp.Labels {
		labels = append(labels, labelInfo{ID: l.Id, Name: l.Name, Type: l.Type})
	}

	return output.WriteJSON(os.Stdout, map[string]any{
		"labels": labels,
	})
}

func gmailAuthError(err error) error {
	var authErr *googleapi.AuthRequiredError
	if isAuthErr(err, &authErr) {
		return output.WriteError(output.ExitCodeAuth, "auth_required", err.Error())
	}

	return output.WriteError(output.ExitCodeError, "gmail_error", err.Error())
}
