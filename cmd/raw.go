package cmd

import (
	"io"
	"os"
	"strings"

	"github.com/quanhoang/jr/internal/client"
	jrerrors "github.com/quanhoang/jr/internal/errors"
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

func runRaw(cmd *cobra.Command, args []string) error {
	method := strings.ToUpper(args[0])
	path := args[1]

	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "config_error",
			Status:    0,
			Message:   err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return err
	}

	// Build query values.
	queryPairs, _ := cmd.Flags().GetStringArray("query")
	q := client.QueryFromFlags(cmd) // empty; query is handled manually below
	for _, pair := range queryPairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			q.Add(parts[0], parts[1])
		}
	}

	// Resolve body reader.
	var bodyReader io.Reader

	bodyFlag, _ := cmd.Flags().GetString("body")
	switch {
	case bodyFlag != "" && strings.HasPrefix(bodyFlag, "@"):
		filename := bodyFlag[1:]
		f, err := os.Open(filename)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "validation_error",
				Status:    0,
				Message:   "cannot open body file: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return err
		}
		defer f.Close()
		bodyReader = f
	case bodyFlag != "":
		bodyReader = strings.NewReader(bodyFlag)
	default:
		// Use stdin if it is not a TTY.
		fi, err := os.Stdin.Stat()
		if err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
			bodyReader = os.Stdin
		}
	}

	code := c.Do(cmd.Context(), method, path, q, bodyReader)
	if code != jrerrors.ExitOK {
		os.Exit(code)
	}
	return nil
}
