package cmd

import "github.com/sofq/jira-cli/cmd/generated"

// TemplateSchemaOps returns schema operations for the template commands.
func TemplateSchemaOps() []generated.SchemaOp {
	return []generated.SchemaOp{
		{
			Resource: "template",
			Verb:     "list",
			Method:   "",
			Path:     "",
			Summary:  "List all available templates (builtin + user-defined)",
			HasBody:  false,
			Flags:    []generated.SchemaFlag{},
		},
		{
			Resource: "template",
			Verb:     "show",
			Method:   "",
			Path:     "",
			Summary:  "Show a template's full definition including variables and fields",
			HasBody:  false,
			Flags: []generated.SchemaFlag{
				{Name: "name", Required: true, Type: "string", Description: "template name (positional argument)", In: "custom"},
			},
		},
		{
			Resource: "template",
			Verb:     "apply",
			Method:   "POST",
			Path:     "/rest/api/3/issue",
			Summary:  "Create an issue from a template with variable substitution",
			HasBody:  false,
			Flags: []generated.SchemaFlag{
				{Name: "name", Required: true, Type: "string", Description: "template name (positional argument)", In: "custom"},
				{Name: "project", Required: true, Type: "string", Description: "project key (e.g. PROJ)", In: "custom"},
				{Name: "var", Required: false, Type: "string", Description: "variable value in key=value format (repeatable)", In: "custom"},
				{Name: "assign", Required: false, Type: "string", Description: "assignee: display name, email, 'me', 'none', or 'unassign'", In: "custom"},
			},
		},
		{
			Resource: "template",
			Verb:     "create",
			Method:   "GET",
			Path:     "/rest/api/3/issue/{issueIdOrKey}",
			Summary:  "Create a user-defined template (optionally from an existing issue)",
			HasBody:  false,
			Flags: []generated.SchemaFlag{
				{Name: "name", Required: true, Type: "string", Description: "template name (positional argument)", In: "custom"},
				{Name: "from", Required: false, Type: "string", Description: "existing issue key to create template from (e.g. PROJ-123)", In: "custom"},
				{Name: "overwrite", Required: false, Type: "boolean", Description: "overwrite existing template with the same name", In: "custom"},
			},
		},
	}
}
