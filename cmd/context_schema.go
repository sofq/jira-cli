package cmd

import "github.com/sofq/jira-cli/cmd/generated"

// ContextSchemaOps returns schema operations for the context command.
func ContextSchemaOps() []generated.SchemaOp {
	return []generated.SchemaOp{
		{
			Resource: "context",
			Verb:     "context",
			Method:   "GET",
			Path:     "/rest/api/3/issue/{issueIdOrKey}",
			Summary:  "Get full context for an issue (fields, comments, links, changelog) in a single call",
			HasBody:  false,
			Flags:    []generated.SchemaFlag{},
		},
	}
}
