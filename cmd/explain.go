package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

// ErrorExplanation is the structured remediation output for jr explain.
type ErrorExplanation struct {
	ErrorType string   `json:"error_type"`
	Summary   string   `json:"summary"`
	Causes    []string `json:"causes"`
	Fixes     []string `json:"fixes"`
	Retryable bool     `json:"retryable"`
	ExitCode  int      `json:"exit_code"`
}

var explainCmd = &cobra.Command{
	Use:   "explain [error-json]",
	Short: "Explain a jr error JSON and suggest remediation steps",
	Long: `Parse a jr error JSON object and return actionable remediation advice.

The error JSON can be passed as a positional argument, piped via stdin, or
read from a file. Output is a JSON object with causes and suggested fixes.

Examples:
  jr explain '{"error_type":"auth_failed","status":401,"message":"Unauthorized"}'
  jr issue get --issueIdOrKey PROJ-999 2>&1 | jr explain
`,
	RunE: runExplain,
}

func init() {
	rootCmd.AddCommand(explainCmd)
}

func runExplain(cmd *cobra.Command, args []string) error {
	var raw []byte

	switch {
	case len(args) > 0:
		// Argument takes priority.
		raw = []byte(strings.Join(args, " "))
	default:
		// Read from stdin.
		fi, statErr := os.Stdin.Stat()
		if statErr != nil || (fi.Mode()&os.ModeCharDevice) != 0 {
			apiErr := &jrerrors.APIError{
				ErrorType: "validation_error",
				Message:   "no input provided: pass error JSON as an argument or pipe it via stdin",
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
		}
		var err error
		raw, err = io.ReadAll(os.Stdin)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "validation_error",
				Message:   "failed to read stdin: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
		}
	}

	var apiErr jrerrors.APIError
	if err := json.Unmarshal(raw, &apiErr); err != nil {
		writeErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   "invalid JSON: " + err.Error(),
		}
		writeErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	// Require at least error_type or message to be meaningful.
	if apiErr.ErrorType == "" && apiErr.Message == "" {
		writeErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   "input does not look like a jr error: missing error_type and message fields",
		}
		writeErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	explanation := buildExplanation(&apiErr)

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(explanation)
	return nil
}

// buildExplanation maps an APIError to an ErrorExplanation with actionable advice.
func buildExplanation(e *jrerrors.APIError) ErrorExplanation {
	et := e.ErrorType

	// Normalise: status-only errors without an explicit type.
	if et == "" {
		et = jrerrors.ErrorTypeFromStatus(e.Status)
	}

	switch et {
	case "auth_failed":
		fixes := []string{
			"Run `jr configure --base-url <url> --token <token> --username <email>` to (re-)authenticate.",
			"Verify the token has not expired in your Atlassian account settings.",
			"Check that the token belongs to the account that has access to this project.",
		}
		if e.Status == 403 {
			fixes = append(fixes,
				"For 403 Forbidden: confirm the account has the required project role or global permission.",
			)
		}
		return ErrorExplanation{
			ErrorType: et,
			Summary:   "Authentication or authorisation failed.",
			Causes: []string{
				"API token is invalid, expired, or revoked.",
				"Username does not match the token owner.",
				"Account lacks the required permission for this operation.",
			},
			Fixes:     fixes,
			Retryable: false,
			ExitCode:  jrerrors.ExitAuth,
		}

	case "not_found":
		return ErrorExplanation{
			ErrorType: et,
			Summary:   "The requested resource was not found.",
			Causes: []string{
				"The issue key, project key, or resource ID does not exist.",
				"The resource may have been deleted.",
				"The authenticated account may not have read access to this resource.",
			},
			Fixes: []string{
				"Double-check the issue key or resource ID.",
				"Use `jr project search` to list accessible projects.",
				"Confirm the account has the Browse Projects permission.",
			},
			Retryable: false,
			ExitCode:  jrerrors.ExitNotFound,
		}

	case "rate_limited":
		fixes := []string{
			"Wait before retrying — Jira enforces a request rate limit per account.",
			"Reduce request frequency or batch operations using `jr batch`.",
		}
		summary := "Request was rate-limited by the Jira API."
		if e.RetryAfter != nil {
			fixes = append([]string{
				fmt.Sprintf("Retry after %d seconds as indicated by the Retry-After header.", *e.RetryAfter),
			}, fixes...)
			summary = fmt.Sprintf("Request was rate-limited. Retry after %d seconds.", *e.RetryAfter)
		}
		return ErrorExplanation{
			ErrorType: et,
			Summary:   summary,
			Causes: []string{
				"Too many requests were sent in a short period.",
				"Concurrent batch operations may be hitting the rate limit together.",
			},
			Fixes:     fixes,
			Retryable: true,
			ExitCode:  jrerrors.ExitRateLimit,
		}

	case "validation_error":
		return ErrorExplanation{
			ErrorType: et,
			Summary:   "The request was rejected due to invalid input.",
			Causes: []string{
				"A required field is missing or has an incorrect value.",
				"The issue type, priority, or status name may be misspelled.",
				"A field value does not match the project's field configuration.",
			},
			Fixes: []string{
				"Review the error message for the specific field that failed validation.",
				"Use `jr schema <resource> <verb>` to check the expected request format.",
				"Use `jr issue get --issueIdOrKey <key>` to inspect valid field values for an existing issue.",
			},
			Retryable: false,
			ExitCode:  jrerrors.ExitValidation,
		}

	case "conflict":
		return ErrorExplanation{
			ErrorType: et,
			Summary:   "The operation conflicted with the current state of the resource.",
			Causes: []string{
				"Another process or user modified the resource concurrently.",
				"An optimistic-lock version mismatch occurred.",
				"A duplicate unique-field value was submitted.",
			},
			Fixes: []string{
				"Re-fetch the resource with `jr issue get` to get the latest state.",
				"Retry the operation with updated field values.",
			},
			Retryable: true,
			ExitCode:  jrerrors.ExitConflict,
		}

	case "server_error":
		return ErrorExplanation{
			ErrorType: et,
			Summary:   "The Jira server returned an internal error.",
			Causes: []string{
				"A transient server-side error or deployment in progress.",
				"The request triggered an unexpected condition in Jira.",
			},
			Fixes: []string{
				"Wait a moment and retry the operation.",
				"Check the Atlassian status page (https://status.atlassian.com) for ongoing incidents.",
				"If the problem persists, contact your Jira administrator.",
			},
			Retryable: true,
			ExitCode:  jrerrors.ExitServer,
		}

	case "connection_error":
		return ErrorExplanation{
			ErrorType: et,
			Summary:   "Could not connect to the Jira server.",
			Causes: []string{
				"The base URL configured in `jr configure` is incorrect or unreachable.",
				"A network outage, DNS failure, or firewall is blocking the connection.",
				"The Jira instance may be temporarily unavailable.",
			},
			Fixes: []string{
				"Verify the base URL with `jr configure --base-url <url> --token <token>`.",
				"Check your network connectivity and DNS resolution.",
				"Try `jr raw GET /rest/api/3/myself` to test basic connectivity.",
			},
			Retryable: true,
			ExitCode:  jrerrors.ExitError,
		}

	default:
		// Generic fallback for unknown or config-level errors.
		exitCode := jrerrors.ExitCodeFromStatus(e.Status)
		if exitCode == jrerrors.ExitOK {
			exitCode = jrerrors.ExitError
		}
		return ErrorExplanation{
			ErrorType: et,
			Summary:   "An unexpected error occurred.",
			Causes: []string{
				"The error type is not one of the standard jr error categories.",
				"This may be a configuration or network-level error.",
			},
			Fixes: []string{
				"Review the original error message for more details.",
				"Run with `--verbose` to see the full HTTP request and response.",
				"Check `jr configure` settings if this is a configuration error.",
			},
			Retryable: false,
			ExitCode:  exitCode,
		}
	}
}
