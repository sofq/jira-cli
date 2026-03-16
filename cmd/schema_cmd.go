package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sofq/jira-cli/cmd/generated"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/sofq/jira-cli/internal/jq"
	"github.com/spf13/cobra"
	"github.com/tidwall/pretty"
)

var schemaCmd = &cobra.Command{
	Use:   "schema [resource] [verb]",
	Short: "Discover available commands and flags (JSON output for agents)",
	Long: `Machine-readable command/flag discovery.

  jr schema --list          # list all resource names
  jr schema issue           # list operations for a resource
  jr schema issue get       # full schema for one operation`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		listFlag, _ := cmd.Flags().GetBool("list")
		compactFlag, _ := cmd.Flags().GetBool("compact")

		// Include hand-written ops alongside generated ones.
		allOps := generated.AllSchemaOps()
		allOps = append(allOps, HandWrittenSchemaOps()...)

		if compactFlag {
			compact := make(map[string][]string)
			for _, op := range allOps {
				compact[op.Resource] = append(compact[op.Resource], op.Verb)
			}
			data, _ := marshalNoEscape(compact)
			return schemaOutput(cmd, data)
		}

		if len(args) == 0 && !listFlag {
			compact := make(map[string][]string)
			for _, op := range allOps {
				compact[op.Resource] = append(compact[op.Resource], op.Verb)
			}
			data, _ := marshalNoEscape(compact)
			return schemaOutput(cmd, data)
		}

		if listFlag {
			// Include hand-written resources alongside generated ones.
			resources := generated.AllResources()
			seen := make(map[string]bool, len(resources))
			for _, r := range resources {
				seen[r] = true
			}
			for _, op := range allOps {
				if !seen[op.Resource] {
					resources = append(resources, op.Resource)
					seen[op.Resource] = true
				}
			}
			data, _ := marshalNoEscape(resources)
			return schemaOutput(cmd, data)
		}

		resource := args[0]

		if len(args) == 1 {
			var matching []generated.SchemaOp
			for _, op := range allOps {
				if op.Resource == resource {
					matching = append(matching, op)
				}
			}
			if len(matching) == 0 {
				apiErr := &jrerrors.APIError{
					ErrorType: "not_found",
					Message:   fmt.Sprintf("resource %q not found", resource),
				}
				apiErr.WriteJSON(os.Stderr)
				return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
			}
			data, _ := marshalNoEscape(matching)
			return schemaOutput(cmd, data)
		}

		verb := args[1]
		for _, op := range allOps {
			if op.Resource == resource && op.Verb == verb {
				data, _ := marshalNoEscape(op)
				return schemaOutput(cmd, data)
			}
		}
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   fmt.Sprintf("operation %s %s not found", resource, verb),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	},
}

// marshalNoEscape serializes v to JSON without HTML escaping.
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// schemaOutput applies --jq and --pretty flags to schema JSON output.
func schemaOutput(cmd *cobra.Command, data []byte) error {
	jqFilter, _ := cmd.Flags().GetString("jq")
	prettyFlag, _ := cmd.Flags().GetBool("pretty")

	if jqFilter != "" {
		filtered, err := jq.Apply(data, jqFilter)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "jq_error",
				Message:   "jq: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
		}
		data = filtered
	}

	if prettyFlag {
		data = pretty.Pretty(data)
	}

	fmt.Fprintf(os.Stdout, "%s\n", strings.TrimRight(string(data), "\n"))
	return nil
}

func init() {
	schemaCmd.Flags().Bool("list", false, "list all resource names")
	schemaCmd.Flags().Bool("compact", false, "compact output: resource → verbs mapping only")
	rootCmd.AddCommand(schemaCmd)
}
