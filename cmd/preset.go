package cmd

import (
	"fmt"
	"os"
	"strings"

	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/sofq/jira-cli/internal/jq"
	"github.com/sofq/jira-cli/internal/preset"
	"github.com/spf13/cobra"
	"github.com/tidwall/pretty"
)

var presetCmd = &cobra.Command{
	Use:   "preset",
	Short: "Manage output presets",
}

var presetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available output presets",
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := preset.List()
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "config_error",
				Message:   "failed to list presets: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
		}

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
	},
}

func init() {
	presetCmd.AddCommand(presetListCmd)
	rootCmd.AddCommand(presetCmd)
}
