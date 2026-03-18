package cmd

import "github.com/sofq/jira-cli/cmd/generated"

// HandWrittenSchemaOps returns schema operations for hand-written commands
// (workflow transition, workflow assign) so they appear in schema discovery
// and batch operations.
func HandWrittenSchemaOps() []generated.SchemaOp {
	return []generated.SchemaOp{
		{
			Resource: "workflow",
			Verb:     "transition",
			Method:   "POST",
			Path:     "/rest/api/3/issue/{issueIdOrKey}/transitions",
			Summary:  "Transition an issue to a new status by name (resolves transition ID automatically)",
			HasBody:  false,
			Flags: []generated.SchemaFlag{
				{Name: "issue", Required: true, Type: "string", Description: "issue key (e.g. PROJ-123)", In: "custom"},
				{Name: "to", Required: true, Type: "string", Description: "target status name (case-insensitive match)", In: "custom"},
			},
		},
		{
			Resource: "workflow",
			Verb:     "assign",
			Method:   "PUT",
			Path:     "/rest/api/3/issue/{issueIdOrKey}/assignee",
			Summary:  "Assign an issue to a user (supports display name, email, 'me', 'none', or 'unassign')",
			HasBody:  false,
			Flags: []generated.SchemaFlag{
				{Name: "issue", Required: true, Type: "string", Description: "issue key (e.g. PROJ-123)", In: "custom"},
				{Name: "to", Required: true, Type: "string", Description: "assignee: display name, email, 'me', 'none', or 'unassign'", In: "custom"},
			},
		},
		{
			Resource: "workflow",
			Verb:     "comment",
			Method:   "POST",
			Path:     "/rest/api/3/issue/{issueIdOrKey}/comment",
			Summary:  "Add a plain-text comment to an issue (converted to ADF automatically)",
			HasBody:  false,
			Flags: []generated.SchemaFlag{
				{Name: "issue", Required: true, Type: "string", Description: "issue key (e.g. PROJ-123)", In: "custom"},
				{Name: "text", Required: true, Type: "string", Description: "comment text (plain text, converted to ADF)", In: "custom"},
			},
		},
		{
			Resource: "workflow",
			Verb:     "move",
			Method:   "POST",
			Path:     "/rest/api/3/issue/{issueIdOrKey}/transitions",
			Summary:  "Transition an issue and optionally reassign in one step",
			HasBody:  false,
			Flags: []generated.SchemaFlag{
				{Name: "issue", Required: true, Type: "string", Description: "issue key (e.g. PROJ-123)", In: "custom"},
				{Name: "to", Required: true, Type: "string", Description: "target status name (case-insensitive match)", In: "custom"},
				{Name: "assign", Required: false, Type: "string", Description: "assignee after transition: display name, email, 'me', 'none', or 'unassign'", In: "custom"},
			},
		},
		{
			Resource: "workflow",
			Verb:     "create",
			Method:   "POST",
			Path:     "/rest/api/3/issue",
			Summary:  "Create an issue from flags (no raw JSON needed)",
			HasBody:  false,
			Flags: []generated.SchemaFlag{
				{Name: "project", Required: true, Type: "string", Description: "project key (e.g. PROJ)", In: "custom"},
				{Name: "type", Required: true, Type: "string", Description: "issue type name (e.g. Bug, Story, Task)", In: "custom"},
				{Name: "summary", Required: true, Type: "string", Description: "issue summary/title", In: "custom"},
				{Name: "description", Required: false, Type: "string", Description: "issue description (plain text, converted to ADF)", In: "custom"},
				{Name: "assign", Required: false, Type: "string", Description: "assignee: display name, email, 'me', 'none', or 'unassign'", In: "custom"},
				{Name: "priority", Required: false, Type: "string", Description: "priority name (e.g. High, Medium, Low)", In: "custom"},
				{Name: "labels", Required: false, Type: "string", Description: "comma-separated list of labels", In: "custom"},
				{Name: "parent", Required: false, Type: "string", Description: "parent issue key (e.g. PROJ-100)", In: "custom"},
			},
		},
	}
}
