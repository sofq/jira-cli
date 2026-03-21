package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/sofq/jira-cli/cmd/generated"
	"github.com/sofq/jira-cli/internal/audit"
	"github.com/sofq/jira-cli/internal/client"
	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/sofq/jira-cli/internal/policy"
	"github.com/sofq/jira-cli/internal/preset"
	"github.com/spf13/cobra"
)

// skipClientCommands are command names that do not require a configured client.
var skipClientCommands = map[string]bool{
	"configure":  true,
	"version":    true,
	"completion": true,
	"help":       true,
	"schema":     true,
	"preset":     true,
	"doctor":     true,
	"explain":    true,
}

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
		timeout, _ := cmd.Flags().GetDuration("timeout")
		outputFormat, _ := cmd.Flags().GetString("format")
		retryCount, _ := cmd.Flags().GetInt("retry")

		// Expand --preset into --fields and --jq defaults.
		// Explicit --fields or --jq flags take precedence over preset values.
		presetName, _ := cmd.Flags().GetString("preset")
		if presetName != "" {
			p, ok, presetErr := preset.Lookup(presetName)
			if presetErr != nil {
				apiErr := &jrerrors.APIError{
					ErrorType: "config_error",
					Message:   "failed to load user presets: " + presetErr.Error(),
				}
				apiErr.WriteJSON(os.Stderr)
				return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
			}
			if !ok {
				apiErr := &jrerrors.APIError{
					ErrorType: "validation_error",
					Message:   fmt.Sprintf("unknown preset %q; use `jr preset list` to see available presets", presetName),
				}
				apiErr.WriteJSON(os.Stderr)
				return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
			}
			if !cmd.Flags().Lookup("fields").Changed && p.Fields != "" {
				fields = p.Fields
			}
			if !cmd.Flags().Lookup("jq").Changed && p.JQ != "" {
				jqFilter = p.JQ
			}
		}

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
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
		}

		if resolved.BaseURL == "" {
			apiErr := &jrerrors.APIError{
				ErrorType: "config_error",
				Status:    0,
				Message:   "base_url is not set; run `jr configure --base-url <url> --token <token>` or set JR_BASE_URL",
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
		}

		// Build operation policy from profile config.
		pol, polErr := policy.NewFromConfig(resolved.AllowedOperations, resolved.DeniedOperations)
		if polErr != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "config_error",
				Message:   "invalid policy config: " + polErr.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
		}

		// Resolve the operation name: "resource verb" or "raw METHOD".
		operation := resolveOperation(cmd)

		// Check policy before proceeding.
		// Skip for commands that check policy themselves (raw needs HTTP method
		// from positional args; batch checks per sub-operation).
		skipPolicyHere := map[string]bool{"raw": true, "batch": true}
		if pol != nil && !skipPolicyHere[cmd.Name()] {
			if err := pol.Check(operation); err != nil {
				apiErr := &jrerrors.APIError{
					ErrorType: "validation_error",
					Message:   err.Error(),
					Hint:      fmt.Sprintf("This operation is not allowed by profile %q", resolved.ProfileName),
				}
				apiErr.WriteJSON(os.Stderr)
				return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
			}
		}

		// Set up audit logger if enabled.
		auditFlag, _ := cmd.Flags().GetBool("audit")
		auditFile, _ := cmd.Flags().GetString("audit-file")
		auditEnabled := auditFlag || auditFile != "" || resolved.AuditLog

		var auditLogger *audit.Logger
		if auditEnabled {
			if auditFile == "" {
				auditFile = audit.DefaultPath()
			}
			var auditErr error
			auditLogger, auditErr = audit.NewLogger(auditFile)
			if auditErr != nil {
				// Audit failure is non-fatal — warn on stderr, continue.
				warnMsg := map[string]string{
					"type":    "warning",
					"message": "audit log open failed: " + auditErr.Error(),
				}
				warnJSON, _ := json.Marshal(warnMsg)
				fmt.Fprintf(os.Stderr, "%s\n", warnJSON)
			}
		}

		c := &client.Client{
			BaseURL:     resolved.BaseURL,
			Auth:        resolved.Auth,
			HTTPClient:  &http.Client{Timeout: timeout},
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
			JQFilter:    jqFilter,
			Paginate:    !noPaginate,
			DryRun:      dryRun,
			Verbose:     verbose,
			Pretty:      pretty,
			Fields:      fields,
			CacheTTL:    cacheTTL,
			Format:      outputFormat,
			MaxRetries:  retryCount,
			AuditLogger: auditLogger,
			Profile:     resolved.ProfileName,
			Operation:   operation,
			Policy:      pol,
		}

		cmd.SetContext(client.NewContext(cmd.Context(), c))
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		// Close the audit logger if one was opened during PersistentPreRunE.
		c, err := client.FromContext(cmd.Context())
		if err == nil && c.AuditLogger != nil {
			c.AuditLogger.Close()
		}
		return nil
	},
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringP("profile", "p", "", "config profile to use")
	pf.String("base-url", "", "Jira base URL (overrides config)")
	pf.String("auth-type", "", "auth type: basic, bearer, or oauth2 (overrides config)")
	pf.String("auth-user", "", "username for basic auth (overrides config)")
	pf.String("auth-token", "", "API token or bearer token (overrides config)")
	pf.String("jq", "", "jq filter expression to apply to the response")
	pf.Bool("pretty", false, "pretty-print JSON output")
	pf.Bool("no-paginate", false, "disable automatic pagination")
	pf.Bool("verbose", false, "log HTTP request/response details to stderr")
	pf.Bool("dry-run", false, "print the request as JSON without executing it")
	pf.String("fields", "", "comma-separated list of fields to return (GET only)")
	pf.Duration("cache", 0, "cache GET responses for this duration (e.g. 5m, 1h)")
	pf.Duration("timeout", 30*time.Second, "HTTP request timeout (e.g. 10s, 1m)")
	pf.String("preset", "", "named output preset (expands to --fields and --jq defaults)")
	pf.Bool("audit", false, "enable audit logging for this invocation")
	pf.String("audit-file", "", "path to audit log file (implies --audit)")
	pf.Int("retry", 0, "number of retries for transient errors (429, 5xx)")
	pf.String("format", "", "additional output format written to stderr: table or csv")

	// Override --version template to output JSON.
	rootCmd.SetVersionTemplate(`{"version":"{{.Version}}"}` + "\n")

	// Register generated commands first, then replace version/workflow parents
	// with hand-written ones while preserving generated subcommands.
	generated.RegisterAll(rootCmd)

	// Merge hand-written version/workflow/avatar with their generated subcommands.
	mergeCommand(rootCmd, versionCmd)
	mergeCommand(rootCmd, workflowCmd)
	mergeCommand(rootCmd, avatarCmd)

	rootCmd.AddCommand(configureCmd)
	rootCmd.AddCommand(rawCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(diffCmd)

	// Override cobra's default help output so that "jr" with no args and
	// "jr help <resource>" emit JSON errors to stderr instead of plain text
	// to stdout. This preserves the JSON-only stdout contract.
	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == rootCmd {
			// Output a helpful JSON hint and exit 0 for explicit --help / help.
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			_ = enc.Encode(map[string]string{
				"hint":    "use `jr schema` to discover commands, or `jr schema <resource>` for operations on a resource",
				"version": Version,
			})
			fmt.Fprintf(os.Stdout, "%s", buf.String())
			return
		}
		// Write help text to stderr so stdout stays JSON-only.
		cmd.SetOut(os.Stderr)
		defaultHelp(cmd, args)
	})
}

// RootCommand returns the root cobra.Command for documentation generation.
func RootCommand() *cobra.Command {
	return rootCmd
}

// Execute runs the root command and returns an exit code.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
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

// resolveOperation returns the operation name for the current command.
// For most commands: "parent child" (e.g., "issue get").
// For raw commands: "raw METHOD" (e.g., "raw GET") — placeholder until RunE.
func resolveOperation(cmd *cobra.Command) string {
	if cmd.Name() == "raw" {
		return "raw"
	}
	if cmd.Parent() != nil && cmd.Parent().Name() != "jr" {
		return cmd.Parent().Name() + " " + cmd.Name()
	}
	return cmd.Name()
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
