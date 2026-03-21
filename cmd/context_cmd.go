package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context <issueIdOrKey>",
	Short: "Get full context for an issue (fields, comments, links, changelog)",
	Long:  "Fetches the issue, its comments, and changelog in a single call. Reduces 3 API calls to 1 CLI invocation.",
	Args:  cobra.ExactArgs(1),
	RunE:  runContext,
}

func init() {
	rootCmd.AddCommand(contextCmd)
}

func runContext(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	issueKey := args[0]
	escapedKey := url.PathEscape(issueKey)

	// Fetch issue with all fields
	issueBody, code := c.Fetch(cmd.Context(), "GET",
		fmt.Sprintf("/rest/api/3/issue/%s?expand=renderedFields", escapedKey), nil)
	if code != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: code}
	}

	// Fetch comments
	commentsBody, code := c.Fetch(cmd.Context(), "GET",
		fmt.Sprintf("/rest/api/3/issue/%s/comment", escapedKey), nil)
	if code != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: code}
	}

	// Fetch changelog
	changelogBody, code := c.Fetch(cmd.Context(), "GET",
		fmt.Sprintf("/rest/api/3/issue/%s/changelog", escapedKey), nil)
	if code != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: code}
	}

	result := map[string]json.RawMessage{
		"issue":     json.RawMessage(issueBody),
		"comments":  json.RawMessage(commentsBody),
		"changelog": json.RawMessage(changelogBody),
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(result)

	exitCode := c.WriteOutput(bytes.TrimRight(buf.Bytes(), "\n"))
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}
	return nil
}
