package cmd

import (
	"os"
	"strings"
	"time"

	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/sofq/jira-cli/internal/watch"
	"github.com/spf13/cobra"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Poll Jira for changes and emit NDJSON events",
	Long: `Watch a JQL query or single issue for changes and stream events as NDJSON.

First poll emits all matching issues as "initial" events. Subsequent polls emit
"created", "updated", or "removed" events only for issues that changed.

Supports --preset, --jq, and --fields for output shaping. Each event is one
JSON line: {"type":"updated","issue":{...}}

Use Ctrl-C (SIGINT) to stop gracefully.`,
	RunE: runWatch,
}

func init() {
	watchCmd.Flags().String("jql", "", "JQL query to watch")
	watchCmd.Flags().String("issue", "", "single issue key to watch (e.g. PROJ-123)")
	watchCmd.Flags().Duration("interval", 30*time.Second, "poll interval (e.g. 10s, 1m, 5m)")
	watchCmd.Flags().Int("max-events", 0, "stop after N events (default: unlimited)")
	watchCmd.MarkFlagsMutuallyExclusive("jql", "issue")
}

func runWatch(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	jql, _ := cmd.Flags().GetString("jql")
	issue, _ := cmd.Flags().GetString("issue")
	interval, _ := cmd.Flags().GetDuration("interval")
	maxEvents, _ := cmd.Flags().GetInt("max-events")

	if jql == "" && issue == "" {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   "either --jql or --issue is required",
		}
		apiErr.WriteJSON(c.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	// Build fields from client's Fields setting (set by --fields or --preset).
	var fields []string
	if c.Fields != "" {
		fields = strings.Split(c.Fields, ",")
	}

	opts := watch.Options{
		JQL:       jql,
		Issue:     issue,
		Interval:  interval,
		Fields:    fields,
		MaxEvents: maxEvents,
	}

	if c.DryRun {
		watch.DryRunOutput(c.Stdout, c.BaseURL, opts)
		return nil
	}

	exitCode := watch.Run(cmd.Context(), c, opts)
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}
	return nil
}
