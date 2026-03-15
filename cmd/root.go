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

// errAlreadyWritten is a sentinel error indicating that the JSON error has
// already been written to stderr by PersistentPreRunE, so Execute() should
// not write a second one.
type errAlreadyWritten struct{ code int }

func (e *errAlreadyWritten) Error() string { return "error already written" }

var rootCmd = &cobra.Command{
	Use:           "jr",
	Short:         "Agent-friendly Jira CLI",
	SilenceUsage:  true,
	SilenceErrors: true,
	Version:       Version,
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
			return &errAlreadyWritten{code: jrerrors.ExitError}
		}

		if resolved.BaseURL == "" {
			apiErr := &jrerrors.APIError{
				ErrorType: "config_error",
				Status:    0,
				Message:   "base_url is not set; run `jr configure --base-url <url> --token <token>` or set JR_BASE_URL",
			}
			apiErr.WriteJSON(os.Stderr)
			return &errAlreadyWritten{code: jrerrors.ExitError}
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

	// Override --version template to output JSON.
	rootCmd.SetVersionTemplate(`{"version":"{{.Version}}"}` + "\n")

	// Register generated commands first, then replace version/workflow parents
	// with hand-written ones while preserving generated subcommands.
	generated.RegisterAll(rootCmd)

	// Merge hand-written version/workflow with their generated subcommands.
	mergeCommand(rootCmd, versionCmd)
	mergeCommand(rootCmd, workflowCmd)

	rootCmd.AddCommand(configureCmd)
	rootCmd.AddCommand(rawCmd)
}

// Execute runs the root command and returns an exit code.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		// If error was already written to stderr, don't write again.
		if aw, ok := err.(*errAlreadyWritten); ok {
			return aw.code
		}
		if aw, ok := err.(*jrerrors.AlreadyWrittenError); ok {
			return aw.Code
		}
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

// mergeCommand replaces a generated parent command on root with a hand-written
// one, while preserving any generated subcommands that are not already present
// on the hand-written command.
func mergeCommand(root *cobra.Command, handWritten *cobra.Command) {
	name := handWritten.Name()
	// Find existing generated command.
	for _, c := range root.Commands() {
		if c.Name() == name {
			// Copy generated subcommands to hand-written command.
			existingSubs := make(map[string]bool)
			for _, sub := range handWritten.Commands() {
				existingSubs[sub.Name()] = true
			}
			for _, sub := range c.Commands() {
				if !existingSubs[sub.Name()] {
					handWritten.AddCommand(sub)
				}
			}
			root.RemoveCommand(c)
			break
		}
	}
	root.AddCommand(handWritten)
}

func init() {
	// Override cobra's default help output so that "jr" with no args and
	// "jr help <resource>" emit JSON errors to stderr instead of plain text
	// to stdout. This preserves the JSON-only stdout contract.
	// Override help to write to stderr (preserving JSON-only stdout contract).
	// For the root command with no args, emit a JSON hint instead.
	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == rootCmd {
			// Bug #6: Output a helpful JSON hint and exit 0 for explicit --help / help.
			out, _ := json.Marshal(map[string]string{
				"hint":    "use `jr schema` to discover commands, or `jr schema <resource>` for operations on a resource",
				"version": Version,
			})
			fmt.Fprintf(os.Stdout, "%s\n", out)
			os.Exit(jrerrors.ExitOK)
			return
		}
		// Write help text to stderr so stdout stays JSON-only.
		cmd.SetOut(os.Stderr)
		defaultHelp(cmd, args)
	})
}
