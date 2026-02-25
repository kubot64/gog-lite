package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/gmail/v1"

	"github.com/morikubo-takashi/gog-lite/internal/googleauth"
)

const (
	scopeGmailReadonly    = "https://www.googleapis.com/auth/gmail.readonly"
	scopeGmailSend        = "https://www.googleapis.com/auth/gmail.send"
	scopeCalendarReadonly = "https://www.googleapis.com/auth/calendar.readonly"
	scopeCalendarWrite    = "https://www.googleapis.com/auth/calendar"
	scopeDocsReadonly     = "https://www.googleapis.com/auth/documents.readonly"
	scopeDocsWrite        = "https://www.googleapis.com/auth/documents"
	scopeDriveReadonly    = "https://www.googleapis.com/auth/drive.readonly"
)

// NewGmail creates an authenticated Gmail service client.
func NewGmail(ctx context.Context, email string) (*gmail.Service, error) {
	opts, err := optionsForEmail(ctx, googleauth.ServiceGmail, email)
	if err != nil {
		return nil, fmt.Errorf("gmail options: %w", err)
	}

	svc, err := gmail.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create gmail service: %w", err)
	}

	return svc, nil
}

func NewGmailReadOnly(ctx context.Context, email string) (*gmail.Service, error) {
	opts, err := optionsForEmailWithScopes(ctx, string(googleauth.ServiceGmail), email, normalizeScopes([]string{
		scopeGmailReadonly,
	}))
	if err != nil {
		return nil, fmt.Errorf("gmail options: %w", err)
	}

	svc, err := gmail.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create gmail service: %w", err)
	}

	return svc, nil
}

func NewGmailWrite(ctx context.Context, email string) (*gmail.Service, error) {
	opts, err := optionsForEmailWithScopes(ctx, string(googleauth.ServiceGmail), email, normalizeScopes([]string{
		scopeGmailSend,
	}))
	if err != nil {
		return nil, fmt.Errorf("gmail options: %w", err)
	}

	svc, err := gmail.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create gmail service: %w", err)
	}

	return svc, nil
}

// NewCalendar creates an authenticated Calendar service client.
func NewCalendar(ctx context.Context, email string) (*calendar.Service, error) {
	opts, err := optionsForEmail(ctx, googleauth.ServiceCalendar, email)
	if err != nil {
		return nil, fmt.Errorf("calendar options: %w", err)
	}

	svc, err := calendar.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create calendar service: %w", err)
	}

	return svc, nil
}

func NewCalendarReadOnly(ctx context.Context, email string) (*calendar.Service, error) {
	opts, err := optionsForEmailWithScopes(ctx, string(googleauth.ServiceCalendar), email, normalizeScopes([]string{
		scopeCalendarReadonly,
	}))
	if err != nil {
		return nil, fmt.Errorf("calendar options: %w", err)
	}

	svc, err := calendar.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create calendar service: %w", err)
	}

	return svc, nil
}

func NewCalendarWrite(ctx context.Context, email string) (*calendar.Service, error) {
	opts, err := optionsForEmailWithScopes(ctx, string(googleauth.ServiceCalendar), email, normalizeScopes([]string{
		scopeCalendarWrite,
	}))
	if err != nil {
		return nil, fmt.Errorf("calendar options: %w", err)
	}

	svc, err := calendar.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create calendar service: %w", err)
	}

	return svc, nil
}

// NewDocs creates an authenticated Docs service client.
func NewDocs(ctx context.Context, email string) (*docs.Service, error) {
	opts, err := optionsForEmail(ctx, googleauth.ServiceDocs, email)
	if err != nil {
		return nil, fmt.Errorf("docs options: %w", err)
	}

	svc, err := docs.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create docs service: %w", err)
	}

	return svc, nil
}

func NewDocsReadOnly(ctx context.Context, email string) (*docs.Service, error) {
	opts, err := optionsForEmailWithScopes(ctx, string(googleauth.ServiceDocs), email, normalizeScopes([]string{
		scopeDocsReadonly,
	}))
	if err != nil {
		return nil, fmt.Errorf("docs options: %w", err)
	}

	svc, err := docs.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create docs service: %w", err)
	}

	return svc, nil
}

func NewDocsWrite(ctx context.Context, email string) (*docs.Service, error) {
	opts, err := optionsForEmailWithScopes(ctx, string(googleauth.ServiceDocs), email, normalizeScopes([]string{
		scopeDocsWrite,
	}))
	if err != nil {
		return nil, fmt.Errorf("docs options: %w", err)
	}

	svc, err := docs.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create docs service: %w", err)
	}

	return svc, nil
}

// NewDrive creates an authenticated Drive service client (used by docs commands).
func NewDrive(ctx context.Context, email string) (*drive.Service, error) {
	opts, err := optionsForEmail(ctx, googleauth.ServiceDocs, email)
	if err != nil {
		return nil, fmt.Errorf("drive options: %w", err)
	}

	svc, err := drive.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create drive service: %w", err)
	}

	return svc, nil
}

func NewDriveReadOnly(ctx context.Context, email string) (*drive.Service, error) {
	opts, err := optionsForEmailWithScopes(ctx, string(googleauth.ServiceDocs), email, normalizeScopes([]string{
		scopeDriveReadonly,
	}))
	if err != nil {
		return nil, fmt.Errorf("drive options: %w", err)
	}

	svc, err := drive.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create drive service: %w", err)
	}

	return svc, nil
}
