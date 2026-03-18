package cmd

import (
	"fmt"
	"net/url"
	"os"

	"github.com/sofq/jira-cli/internal/changelog"
	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show what changed on an issue (changelog as structured JSON)",
	Long: `Fetches the changelog for an issue and outputs field-level changes as structured JSON.

Filter by time range (--since) or specific field name (--field).
Supports --jq and --preset for output shaping.

Examples:
  jr diff --issue PROJ-123
  jr diff --issue PROJ-123 --since 2h
  jr diff --issue PROJ-123 --since 2025-01-01
  jr diff --issue PROJ-123 --field status`,
	RunE: runDiff,
}

func init() {
	diffCmd.Flags().String("issue", "", "issue key (e.g. PROJ-123)")
	_ = diffCmd.MarkFlagRequired("issue")
	diffCmd.Flags().String("since", "", "filter changes since duration (e.g. 2h, 1d) or ISO date (e.g. 2025-01-01)")
	diffCmd.Flags().String("field", "", "filter to a specific field name")
}

func runDiff(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	issueKey, _ := cmd.Flags().GetString("issue")
	since, _ := cmd.Flags().GetString("since")
	field, _ := cmd.Flags().GetString("field")

	changelogPath := fmt.Sprintf("/rest/api/3/issue/%s/changelog", url.PathEscape(issueKey))

	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "GET",
			"url":    c.BaseURL + changelogPath,
			"note":   fmt.Sprintf("would fetch changelog for %s", issueKey),
		})
		if code := c.WriteOutput(out); code != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: code}
		}
		return nil
	}

	body, exitCode := c.Fetch(cmd.Context(), "GET", changelogPath, nil)
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}

	opts := changelog.Options{
		Since: since,
		Field: field,
	}

	result, err := changelog.Parse(issueKey, body, opts)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	out, _ := marshalNoEscape(result)
	if exitCode := c.WriteOutput(out); exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}
	return nil
}
