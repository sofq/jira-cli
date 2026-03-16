package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

var rawCmd = &cobra.Command{
	Use:   "raw <method> <path>",
	Short: "Execute a raw Jira API call",
	Args:  cobra.ExactArgs(2),
	RunE:  runRaw,
}

func init() {
	f := rawCmd.Flags()
	f.String("body", "", "request body as a JSON string or @filename")
	f.StringArray("query", nil, "query parameters as key=value (repeatable)")
}

// validHTTPMethods is the set of allowed HTTP methods.
var validHTTPMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "DELETE": true,
	"PATCH": true, "HEAD": true, "OPTIONS": true,
}

// methodNeedsBody returns true for HTTP methods that typically require a body.
var methodsWithBody = map[string]bool{
	"POST": true, "PUT": true, "PATCH": true,
}

func runRaw(cmd *cobra.Command, args []string) error {
	method := strings.ToUpper(args[0])
	path := args[1]

	// Bug #7: Validate HTTP method client-side.
	if !validHTTPMethods[method] {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   fmt.Sprintf("invalid HTTP method %q; must be one of GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS", method),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitValidation}
	}

	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "config_error",
			Message:   err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitError}
	}

	// Build query values.
	queryPairs, _ := cmd.Flags().GetStringArray("query")
	q := client.QueryFromFlags(cmd) // empty; query is handled manually below
	for _, pair := range queryPairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			apiErr := &jrerrors.APIError{
				ErrorType: "validation_error",
				Message:   fmt.Sprintf("invalid --query %q: expected key=value format", pair),
			}
			apiErr.WriteJSON(os.Stderr)
			return &errAlreadyWritten{code: jrerrors.ExitValidation}
		}
		q.Add(parts[0], parts[1])
	}

	// Resolve body reader.
	var bodyReader io.Reader

	bodyFlag, _ := cmd.Flags().GetString("body")
	switch {
	case bodyFlag == "-":
		// Explicit stdin: --body -
		bodyReader = os.Stdin
	case bodyFlag != "" && strings.HasPrefix(bodyFlag, "@"):
		filename := bodyFlag[1:]
		f, err := os.Open(filename)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "validation_error",
				Message:   "cannot open body file: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &errAlreadyWritten{code: jrerrors.ExitValidation}
		}
		defer f.Close()
		bodyReader = f
	case bodyFlag != "":
		bodyReader = strings.NewReader(bodyFlag)
	default:
		// Bug #3: Don't auto-read stdin for raw commands — require explicit --body - instead.
		// This prevents hanging when no body is piped.
	}

	// Bug #16: Warn if --body is used with GET/HEAD/DELETE/OPTIONS.
	if bodyFlag != "" && !methodsWithBody[method] {
		warnMsg := map[string]any{
			"type":    "warning",
			"message": fmt.Sprintf("--body is ignored for %s requests", method),
		}
		warnJSON, _ := json.Marshal(warnMsg)
		fmt.Fprintf(os.Stderr, "%s\n", warnJSON)
		bodyReader = nil
	}

	// Bug #3: If method needs a body but none was provided, error instead of hanging on stdin.
	if methodsWithBody[method] && bodyReader == nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   fmt.Sprintf("%s request requires a body; use --body '{...}' or pipe JSON to stdin", method),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitValidation}
	}

	code := c.Do(cmd.Context(), method, path, q, bodyReader)
	if code != jrerrors.ExitOK {
		return &errAlreadyWritten{code: code}
	}
	return nil
}
