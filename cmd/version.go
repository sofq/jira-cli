package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags: -X github.com/sofq/jira-cli/cmd.Version=<ver>
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version as JSON",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		out, err := json.Marshal(map[string]string{"version": Version})
		if err != nil {
			return err
		}
		return schemaOutput(cmd, out)
	},
}
