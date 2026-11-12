package cmd

import (
	"context"
	"sync"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/gmail/v1"

	"github.com/kubot64/gog-lite/internal/config"
	"github.com/kubot64/gog-lite/internal/googleapi"
	"github.com/kubot64/gog-lite/internal/googleauth"
)

type commandDeps struct {
	readCredentials            func() (config.ClientCredentials, error)
	authStep1                  func(context.Context, config.ClientCredentials, googleauth.AuthorizeOptions) (googleauth.Step1Result, error)
	authStep2                  func(context.Context, config.ClientCredentials, googleauth.AuthorizeOptions, string) (googleauth.Step2Result, error)
	newGmailReadOnlyService    func(context.Context, string) (*gmail.Service, error)
	newGmailWriteService       func(context.Context, string) (*gmail.Service, error)
	newCalendarReadOnlyService func(context.Context, string) (*calendar.Service, error)
	newCalendarWriteService    func(context.Context, string) (*calendar.Service, error)
	newDocsWriteService        func(context.Context, string) (*docs.Service, error)
	newDriveReadOnlyService    func(context.Context, string) (*drive.Service, error)
}

var (
	commandDepsMu     sync.RWMutex
	commandDepsTestMu sync.Mutex
	deps              = commandDeps{
		readCredentials:            config.ReadCredentials,
		authStep1:                  googleauth.Step1,
		authStep2:                  googleauth.Step2,
		newGmailReadOnlyService:    googleapi.NewGmailReadOnly,
		newGmailWriteService:       googleapi.NewGmailWrite,
		newCalendarReadOnlyService: googleapi.NewCalendarReadOnly,
		newCalendarWriteService:    googleapi.NewCalendarWrite,
		newDocsWriteService:        googleapi.NewDocsWrite,
		newDriveReadOnlyService:    googleapi.NewDriveReadOnly,
	}
)

func currentCommandDeps() commandDeps {
	commandDepsMu.RLock()
	defer commandDepsMu.RUnlock()

	return deps
}

func setCommandDepsForTest(override func(*commandDeps)) func() {
	commandDepsTestMu.Lock()
	commandDepsMu.Lock()
	prev := deps
	override(&deps)
	commandDepsMu.Unlock()

	return func() {
		commandDepsMu.Lock()
		deps = prev
		commandDepsMu.Unlock()
		commandDepsTestMu.Unlock()
	}
}
