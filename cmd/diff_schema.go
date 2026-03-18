package cmd

import "github.com/sofq/jira-cli/cmd/generated"

// DiffSchemaOps returns schema operations for the diff command.
func DiffSchemaOps() []generated.SchemaOp {
	return []generated.SchemaOp{
		{
			Resource: "diff",
			Verb:     "diff",
			Method:   "GET",
			Path:     "/rest/api/3/issue/{issueIdOrKey}/changelog",
			Summary:  "Show the field-change history for an issue as structured JSON",
			HasBody:  false,
			Flags: []generated.SchemaFlag{
				{Name: "issue", Required: true, Type: "string", Description: "issue key (e.g. PROJ-123)", In: "custom"},
				{Name: "since", Required: false, Type: "string", Description: "filter changes since duration (e.g. 2h, 1d) or ISO date (e.g. 2025-01-01)", In: "custom"},
				{Name: "field", Required: false, Type: "string", Description: "filter to a specific field name", In: "custom"},
			},
		},
	}
}
