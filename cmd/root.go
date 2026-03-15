package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/sofq/jira-cli/cmd/generated"
	"github.com/sofq/jira-cli/internal/client"
	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

// skipClientCommands are command names that do not require a configured client.
var skipClientCommands = map[string]bool{
	"configure":  true,
	"version":    true,
	"completion": true,
	"help":       true,
	"schema":     true,
}

var rootCmd = &cobra.Command{
	Use:          "jr",
	Short:        "Agent-friendly Jira CLI",
	SilenceUsage: true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip client injection for built-in / setup commands.
		name := cmd.Name()
		if skipClientCommands[name] {
			return nil
		}
		// Also skip for subcommands of skipped commands (e.g., completion bash)
		if cmd.Parent() != nil && skipClientCommands[cmd.Parent().Name()] {
			return nil
		}

		// Read flag overrides.
		baseURL, _ := cmd.Flags().GetString("base-url")
		authType, _ := cmd.Flags().GetString("auth-type")
		authUser, _ := cmd.Flags().GetString("auth-user")
		authToken, _ := cmd.Flags().GetString("auth-token")

		profileName, _ := cmd.Flags().GetString("profile")
		jqFilter, _ := cmd.Flags().GetString("jq")
		pretty, _ := cmd.Flags().GetBool("pretty")
		noPaginate, _ := cmd.Flags().GetBool("no-paginate")
		verbose, _ := cmd.Flags().GetBool("verbose")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		fields, _ := cmd.Flags().GetString("fields")
		cacheTTL, _ := cmd.Flags().GetDuration("cache")

		flags := &config.FlagOverrides{
			BaseURL:  baseURL,
			AuthType: authType,
			Username: authUser,
			Token:    authToken,
		}

		resolved, err := config.Resolve(config.DefaultPath(), profileName, flags)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "config_error",
				Status:    0,
				Message:   "failed to resolve config: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return err
		}

		if resolved.BaseURL == "" {
			apiErr := &jrerrors.APIError{
				ErrorType: "config_error",
				Status:    0,
				Message:   "base_url is not set; run `jr configure --base-url <url> --token <token>` or set JR_BASE_URL",
			}
			apiErr.WriteJSON(os.Stderr)
			return fmt.Errorf("base_url is required")
		}

		c := &client.Client{
			BaseURL:    resolved.BaseURL,
			Auth:       resolved.Auth,
			HTTPClient: http.DefaultClient,
			Stdout:     os.Stdout,
			Stderr:     os.Stderr,
			JQFilter:   jqFilter,
			Paginate:   !noPaginate,
			DryRun:     dryRun,
			Verbose:    verbose,
			Pretty:     pretty,
			Fields:     fields,
			CacheTTL:   cacheTTL,
		}

		cmd.SetContext(client.NewContext(cmd.Context(), c))
		return nil
	},
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringP("profile", "p", "", "config profile to use")
	pf.String("base-url", "", "Jira base URL (overrides config)")
	pf.String("auth-type", "", "auth type: basic or bearer (overrides config)")
	pf.String("auth-user", "", "username for basic auth (overrides config)")
	pf.String("auth-token", "", "API token or bearer token (overrides config)")
	pf.String("jq", "", "jq filter expression to apply to the response")
	pf.Bool("pretty", false, "pretty-print JSON output")
	pf.Bool("no-paginate", false, "disable automatic pagination")
	pf.Bool("verbose", false, "log HTTP request/response details to stderr")
	pf.Bool("dry-run", false, "print the request as JSON without executing it")
	pf.String("fields", "", "comma-separated list of fields to return (GET only)")
	pf.Duration("cache", 0, "cache GET responses for this duration (e.g. 5m, 1h)")

	rootCmd.AddCommand(configureCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(rawCmd)
	rootCmd.AddCommand(workflowCmd)
	generated.RegisterAll(rootCmd)
}

// Execute runs the root command and returns an exit code.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		enc := json.NewEncoder(os.Stderr)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(map[string]string{
			"error_type": "command_error",
			"message":    err.Error(),
		})
		return jrerrors.ExitError
	}
	return jrerrors.ExitOK
}
