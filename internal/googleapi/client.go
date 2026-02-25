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
