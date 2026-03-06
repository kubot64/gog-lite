package cmd

import (
	"context"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/gmail/v1"

	"github.com/kubot64/gog-lite/internal/config"
	"github.com/kubot64/gog-lite/internal/googleapi"
	"github.com/kubot64/gog-lite/internal/googleauth"
)

var (
	readCredentials         = config.ReadCredentials
	authStep1               = googleauth.Step1
	authStep2               = googleauth.Step2
	newGmailWriteService    = googleapi.NewGmailWrite
	newCalendarWriteService = googleapi.NewCalendarWrite
	newDocsWriteService     = googleapi.NewDocsWrite
	newDriveReadOnlyService = googleapi.NewDriveReadOnly
)

type gmailWriteServiceFactory func(context.Context, string) (*gmail.Service, error)
type calendarWriteServiceFactory func(context.Context, string) (*calendar.Service, error)
type docsWriteServiceFactory func(context.Context, string) (*docs.Service, error)
type driveReadOnlyServiceFactory func(context.Context, string) (*drive.Service, error)
