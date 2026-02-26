package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/gmail/v1"

	"github.com/kubot64/gog-lite/internal/googleauth"
)

const (
	scopeGmailReadonly    = "https://www.googleapis.com/auth/gmail.readonly"
	scopeGmailCompose     = "https://www.googleapis.com/auth/gmail.compose"
	scopeCalendarReadonly = "https://www.googleapis.com/auth/calendar.readonly"
	scopeCalendarWrite    = "https://www.googleapis.com/auth/calendar"
	scopeDocsReadonly     = "https://www.googleapis.com/auth/documents.readonly"
	scopeDocsWrite        = "https://www.googleapis.com/auth/documents"
	scopeDriveReadonly    = "https://www.googleapis.com/auth/drive.readonly"
)

// Legacy constructors — delegate to read-only variants for least privilege.
func NewGmail(ctx context.Context, email string) (*gmail.Service, error) {
	return NewGmailReadOnly(ctx, email)
}
func NewCalendar(ctx context.Context, email string) (*calendar.Service, error) {
	return NewCalendarReadOnly(ctx, email)
}
func NewDocs(ctx context.Context, email string) (*docs.Service, error) {
	return NewDocsReadOnly(ctx, email)
}
func NewDrive(ctx context.Context, email string) (*drive.Service, error) {
	return NewDriveReadOnly(ctx, email)
}

func NewGmailReadOnly(ctx context.Context, email string) (*gmail.Service, error) {
	opts, err := optionsForEmailWithScopes(ctx, string(googleauth.ServiceGmail), email, []string{scopeGmailReadonly})
	if err != nil {
		return nil, fmt.Errorf("gmail options: %w", err)
	}
	return gmail.NewService(ctx, opts...)
}

func NewGmailWrite(ctx context.Context, email string) (*gmail.Service, error) {
	opts, err := optionsForEmailWithScopes(ctx, string(googleauth.ServiceGmail), email, []string{scopeGmailCompose})
	if err != nil {
		return nil, fmt.Errorf("gmail options: %w", err)
	}
	return gmail.NewService(ctx, opts...)
}

func NewCalendarReadOnly(ctx context.Context, email string) (*calendar.Service, error) {
	opts, err := optionsForEmailWithScopes(ctx, string(googleauth.ServiceCalendar), email, []string{scopeCalendarReadonly})
	if err != nil {
		return nil, fmt.Errorf("calendar options: %w", err)
	}
	return calendar.NewService(ctx, opts...)
}

func NewCalendarWrite(ctx context.Context, email string) (*calendar.Service, error) {
	opts, err := optionsForEmailWithScopes(ctx, string(googleauth.ServiceCalendar), email, []string{scopeCalendarWrite})
	if err != nil {
		return nil, fmt.Errorf("calendar options: %w", err)
	}
	return calendar.NewService(ctx, opts...)
}

func NewDocsReadOnly(ctx context.Context, email string) (*docs.Service, error) {
	opts, err := optionsForEmailWithScopes(ctx, string(googleauth.ServiceDocs), email, []string{scopeDocsReadonly})
	if err != nil {
		return nil, fmt.Errorf("docs options: %w", err)
	}
	return docs.NewService(ctx, opts...)
}

func NewDocsWrite(ctx context.Context, email string) (*docs.Service, error) {
	opts, err := optionsForEmailWithScopes(ctx, string(googleauth.ServiceDocs), email, []string{scopeDocsWrite})
	if err != nil {
		return nil, fmt.Errorf("docs options: %w", err)
	}
	return docs.NewService(ctx, opts...)
}

func NewDriveReadOnly(ctx context.Context, email string) (*drive.Service, error) {
	opts, err := optionsForEmailWithScopes(ctx, string(googleauth.ServiceDocs), email, []string{scopeDriveReadonly})
	if err != nil {
		return nil, fmt.Errorf("drive options: %w", err)
	}
	return drive.NewService(ctx, opts...)
}
