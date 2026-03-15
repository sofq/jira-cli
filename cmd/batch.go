package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/sofq/jira-cli/cmd/generated"
	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

// BatchOp represents a single operation in a batch request.
type BatchOp struct {
	Command string            `json:"command"`
	Args    map[string]string `json:"args"`
	JQ      string            `json:"jq,omitempty"`
}

// BatchResult holds the result of a single batch operation.
type BatchResult struct {
	Index    int             `json:"index"`
	ExitCode int             `json:"exit_code"`
	Data     json.RawMessage `json:"data,omitempty"`
	Error    json.RawMessage `json:"error,omitempty"`
}

var batchCmd = &cobra.Command{
	Use:   "batch",
	Short: "Execute multiple API operations in a single invocation",
	Long: `Execute multiple Jira API operations from a JSON array.

Input JSON array format:
  [
    {"command": "issue get", "args": {"issueIdOrKey": "FOO-1"}, "jq": ".key"},
    {"command": "project list", "args": {}}
  ]

Output JSON array format:
  [
    {"index": 0, "exit_code": 0, "data": "FOO-1"},
    {"index": 1, "exit_code": 0, "data": [...]}
  ]

Use --input to specify a file, or pipe JSON to stdin.`,
	RunE: runBatch,
}

func init() {
	batchCmd.Flags().String("input", "", "path to JSON input file (reads stdin if not set)")
	rootCmd.AddCommand(batchCmd)
}

func runBatch(cmd *cobra.Command, args []string) error {
	// Get the base client from context (configured in PersistentPreRunE).
	baseClient, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "config_error",
			Status:    0,
			Message:   err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitError}
	}

	// Read input.
	inputFlag, _ := cmd.Flags().GetString("input")

	var inputData []byte
	if inputFlag != "" {
		inputData, err = os.ReadFile(inputFlag)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "validation_error",
				Status:    0,
				Message:   "cannot read input file: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &errAlreadyWritten{code: jrerrors.ExitValidation}
		}
	} else {
		// Read from stdin.
		fi, statErr := os.Stdin.Stat()
		if statErr != nil || (fi.Mode()&os.ModeCharDevice) != 0 {
			apiErr := &jrerrors.APIError{
				ErrorType: "validation_error",
				Status:    0,
				Message:   "no input provided: use --input <file> or pipe JSON to stdin",
			}
			apiErr.WriteJSON(os.Stderr)
			return &errAlreadyWritten{code: jrerrors.ExitValidation}
		}
		inputData, err = io.ReadAll(os.Stdin)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "validation_error",
				Status:    0,
				Message:   "failed to read stdin: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &errAlreadyWritten{code: jrerrors.ExitError}
		}
	}

	// Parse the batch ops.
	var ops []BatchOp
	if err := json.Unmarshal(inputData, &ops); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Status:    0,
			Message:   "invalid JSON input: expected a JSON array of operations; " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitValidation}
	}

	// Bug #17: Reject null/empty input explicitly.
	if ops == nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Status:    0,
			Message:   "invalid JSON input: expected a JSON array of operations, got null",
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitValidation}
	}

	// Load all schema ops for lookup.
	allOps := generated.AllSchemaOps()

	// Build a lookup map: "resource verb" -> SchemaOp
	opMap := make(map[string]generated.SchemaOp, len(allOps))
	for _, op := range allOps {
		key := op.Resource + " " + op.Verb
		opMap[key] = op
	}

	// Execute each operation and collect results.
	results := make([]BatchResult, len(ops))

	for i, bop := range ops {
		result := executeBatchOp(cmd, baseClient, i, bop, opMap)
		results[i] = result
	}

	// Write all results as a JSON array to stdout.
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(results); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Status:    0,
			Message:   "failed to encode results: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitError}
	}

	// Bug #9: Exit with highest-severity exit code from batch operations.
	maxExit := 0
	for _, r := range results {
		if r.ExitCode > maxExit {
			maxExit = r.ExitCode
		}
	}
	if maxExit != 0 {
		return &errAlreadyWritten{code: maxExit}
	}

	return nil
}

// executeBatchOp runs a single batch operation and returns its result.
func executeBatchOp(
	cmd *cobra.Command,
	baseClient *client.Client,
	index int,
	bop BatchOp,
	opMap map[string]generated.SchemaOp,
) BatchResult {
	// Look up the schema op.
	schemaOp, ok := opMap[bop.Command]
	if !ok {
		errMsg := fmt.Sprintf("unknown command %q", bop.Command)
		return errorResult(index, jrerrors.ExitError, "validation_error", errMsg)
	}

	// Build path by substituting {paramName} placeholders.
	path := schemaOp.Path
	for _, flag := range schemaOp.Flags {
		if flag.In == "path" {
			placeholder := "{" + flag.Name + "}"
			if val, exists := bop.Args[flag.Name]; exists {
				path = strings.ReplaceAll(path, placeholder, url.PathEscape(val))
			}
		}
	}

	// Build query params from args that match "query" flags in the schema.
	query := url.Values{}
	for _, flag := range schemaOp.Flags {
		if flag.In == "query" {
			if val, exists := bop.Args[flag.Name]; exists {
				query.Set(flag.Name, val)
			}
		}
	}

	// Handle body.
	var bodyReader strings.Reader
	var bodyReaderPtr *strings.Reader
	if bodyStr, exists := bop.Args["body"]; exists && bodyStr != "" {
		bodyReader = *strings.NewReader(bodyStr)
		bodyReaderPtr = &bodyReader
	}

	// Create a per-operation client with captured stdout/stderr.
	var stdoutBuf strings.Builder
	var stderrBuf strings.Builder

	opClient := &client.Client{
		BaseURL:    baseClient.BaseURL,
		Auth:       baseClient.Auth,
		HTTPClient: baseClient.HTTPClient,
		Stdout:     &stdoutBuf,
		Stderr:     &stderrBuf,
		JQFilter:   bop.JQ,
		Paginate:   baseClient.Paginate,
		DryRun:     baseClient.DryRun,
		Verbose:    baseClient.Verbose,
		Pretty:     baseClient.Pretty,
		Fields:     baseClient.Fields,
		CacheTTL:   baseClient.CacheTTL,
	}

	var exitCode int
	if bodyReaderPtr != nil {
		exitCode = opClient.Do(cmd.Context(), schemaOp.Method, path, query, bodyReaderPtr)
	} else {
		exitCode = opClient.Do(cmd.Context(), schemaOp.Method, path, query, nil)
	}

	if exitCode != jrerrors.ExitOK {
		// Return error result using stderr content.
		errOutput := strings.TrimSpace(stderrBuf.String())
		var rawErr json.RawMessage
		if json.Valid([]byte(errOutput)) {
			rawErr = json.RawMessage(errOutput)
		} else {
			encoded, _ := json.Marshal(map[string]string{"message": errOutput})
			rawErr = json.RawMessage(encoded)
		}
		return BatchResult{
			Index:    index,
			ExitCode: exitCode,
			Error:    rawErr,
		}
	}

	// Parse stdout as JSON data.
	outStr := strings.TrimSpace(stdoutBuf.String())
	var rawData json.RawMessage
	if outStr != "" && json.Valid([]byte(outStr)) {
		rawData = json.RawMessage(outStr)
	} else if outStr != "" {
		encoded, _ := json.Marshal(outStr)
		rawData = json.RawMessage(encoded)
	}

	return BatchResult{
		Index:    index,
		ExitCode: jrerrors.ExitOK,
		Data:     rawData,
	}
}

// errorResult constructs a BatchResult for an error condition.
func errorResult(index, exitCode int, errType, message string) BatchResult {
	apiErr := &jrerrors.APIError{
		ErrorType: errType,
		Status:    0,
		Message:   message,
	}
	encoded, _ := json.Marshal(apiErr)
	return BatchResult{
		Index:    index,
		ExitCode: exitCode,
		Error:    json.RawMessage(encoded),
	}
}
