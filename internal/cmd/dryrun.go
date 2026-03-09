package cmd

type dryRunPayloadOptions struct {
	RequiresConfirmation  bool
	RequiresApprovalToken bool
	WouldCallAPI          bool
}

func newDryRunPayload(action string, params map[string]any, opts dryRunPayloadOptions) map[string]any {
	return map[string]any{
		"dry_run":                 true,
		"action":                  action,
		"params":                  params,
		"requires_confirmation":   opts.RequiresConfirmation,
		"requires_approval_token": opts.RequiresApprovalToken,
		"would_call_api":          opts.WouldCallAPI,
		"validation_passed":       true,
	}
}
