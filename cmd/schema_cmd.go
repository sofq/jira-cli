package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sofq/jira-cli/cmd/generated"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
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

		if compactFlag {
			allOps := generated.AllSchemaOps()
			compact := make(map[string][]string)
			for _, op := range allOps {
				compact[op.Resource] = append(compact[op.Resource], op.Verb)
			}
			data, _ := json.Marshal(compact)
			fmt.Println(string(data))
			return nil
		}

		if listFlag || len(args) == 0 {
			data, _ := json.Marshal(generated.AllResources())
			fmt.Println(string(data))
			return nil
		}

		resource := args[0]
		allOps := generated.AllSchemaOps()

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
				os.Exit(jrerrors.ExitNotFound)
			}
			data, _ := json.Marshal(matching)
			fmt.Println(string(data))
			return nil
		}

		verb := args[1]
		for _, op := range allOps {
			if op.Resource == resource && op.Verb == verb {
				data, _ := json.Marshal(op)
				fmt.Println(string(data))
				return nil
			}
		}
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   fmt.Sprintf("operation %s %s not found", resource, verb),
		}
		apiErr.WriteJSON(os.Stderr)
		os.Exit(jrerrors.ExitNotFound)
		return nil
	},
}

func init() {
	schemaCmd.Flags().Bool("list", false, "list all resource names")
	schemaCmd.Flags().Bool("compact", false, "compact output: resource → verbs mapping only")
	rootCmd.AddCommand(schemaCmd)
}
