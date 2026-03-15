package cmd

import (
	"context"
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

	// Load all schema ops for lookup (generated + hand-written).
	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)

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

	// Hand-written commands have custom execution logic.
	if bop.Command == "workflow transition" || bop.Command == "workflow assign" {
		exitCode := executeBatchWorkflow(cmd.Context(), opClient, bop)
		return buildBatchResult(index, exitCode, &stdoutBuf, &stderrBuf)
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

	var exitCode int
	if bodyReaderPtr != nil {
		exitCode = opClient.Do(cmd.Context(), schemaOp.Method, path, query, bodyReaderPtr)
	} else {
		exitCode = opClient.Do(cmd.Context(), schemaOp.Method, path, query, nil)
	}

	return buildBatchResult(index, exitCode, &stdoutBuf, &stderrBuf)
}

// executeBatchWorkflow runs workflow transition or assign as a batch operation.
func executeBatchWorkflow(ctx context.Context, c *client.Client, bop BatchOp) int {
	issueKey := bop.Args["issue"]
	to := bop.Args["to"]

	if issueKey == "" || to == "" {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   "workflow commands require 'issue' and 'to' args",
		}
		apiErr.WriteJSON(c.Stderr)
		return jrerrors.ExitValidation
	}

	if bop.Command == "workflow transition" {
		return batchTransition(ctx, c, issueKey, to)
	}
	return batchAssign(ctx, c, issueKey, to)
}

// batchTransition executes a workflow transition within a batch operation.
func batchTransition(ctx context.Context, c *client.Client, issueKey, toStatus string) int {
	if c.DryRun {
		out, _ := json.Marshal(map[string]string{
			"method": "POST",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey),
			"note":   fmt.Sprintf("would transition %s to %q (transition ID resolved at runtime)", issueKey, toStatus),
		})
		fmt.Fprintf(c.Stdout, "%s\n", out)
		return jrerrors.ExitOK
	}

	transitionsBody, exitCode := fetchJSON(c, ctx, "GET",
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey))
	if exitCode != jrerrors.ExitOK {
		return exitCode
	}

	var transResp struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			To   struct {
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := json.Unmarshal(transitionsBody, &transResp); err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "connection_error", Message: "failed to parse transitions: " + err.Error()}
		apiErr.WriteJSON(c.Stderr)
		return jrerrors.ExitError
	}

	toLower := strings.ToLower(toStatus)
	var matchedID, matchedName string
	for _, tr := range transResp.Transitions {
		if strings.ToLower(tr.Name) == toLower || strings.ToLower(tr.To.Name) == toLower {
			matchedID = tr.ID
			matchedName = tr.Name
			break
		}
	}
	if matchedID == "" {
		for _, tr := range transResp.Transitions {
			if strings.Contains(strings.ToLower(tr.Name), toLower) ||
				strings.Contains(strings.ToLower(tr.To.Name), toLower) {
				matchedID = tr.ID
				matchedName = tr.Name
				break
			}
		}
	}
	if matchedID == "" {
		names := make([]string, len(transResp.Transitions))
		for i, tr := range transResp.Transitions {
			names[i] = tr.Name
		}
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   fmt.Sprintf("no transition matching %q; available: %s", toStatus, strings.Join(names, ", ")),
		}
		apiErr.WriteJSON(c.Stderr)
		return jrerrors.ExitValidation
	}

	transBody, _ := json.Marshal(map[string]any{"transition": map[string]any{"id": matchedID}})
	body := string(transBody)
	_, code := fetchJSONWithBody(c, ctx, "POST",
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey),
		strings.NewReader(body))
	if code != jrerrors.ExitOK {
		return code
	}

	out, _ := json.Marshal(map[string]string{
		"status":     "transitioned",
		"issue":      issueKey,
		"transition": matchedName,
	})
	fmt.Fprintf(c.Stdout, "%s\n", out)
	return jrerrors.ExitOK
}

// batchAssign executes a workflow assign within a batch operation.
func batchAssign(ctx context.Context, c *client.Client, issueKey, to string) int {
	if c.DryRun {
		out, _ := json.Marshal(map[string]string{
			"method": "PUT",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/assignee", issueKey),
			"note":   fmt.Sprintf("would assign %s to %q (account ID resolved at runtime)", issueKey, to),
		})
		fmt.Fprintf(c.Stdout, "%s\n", out)
		return jrerrors.ExitOK
	}

	var accountID string

	switch strings.ToLower(to) {
	case "me":
		body, code := fetchJSON(c, ctx, "GET", "/rest/api/3/myself")
		if code != jrerrors.ExitOK {
			return code
		}
		var me struct {
			AccountID string `json:"accountId"`
		}
		if err := json.Unmarshal(body, &me); err != nil || me.AccountID == "" {
			apiErr := &jrerrors.APIError{ErrorType: "connection_error", Message: "could not determine your account ID from /myself"}
			apiErr.WriteJSON(c.Stderr)
			return jrerrors.ExitError
		}
		accountID = me.AccountID

	case "none", "unassign":
		assignBody := `{"accountId":null}`
		_, code := fetchJSONWithBody(c, ctx, "PUT",
			fmt.Sprintf("/rest/api/3/issue/%s/assignee", issueKey),
			strings.NewReader(assignBody))
		if code != jrerrors.ExitOK {
			return code
		}
		out, _ := json.Marshal(map[string]string{"status": "unassigned", "issue": issueKey})
		fmt.Fprintf(c.Stdout, "%s\n", out)
		return jrerrors.ExitOK

	default:
		body, code := fetchJSON(c, ctx, "GET",
			fmt.Sprintf("/rest/api/3/user/search?query=%s", url.QueryEscape(to)))
		if code != jrerrors.ExitOK {
			return code
		}
		var users []struct {
			AccountID   string `json:"accountId"`
			DisplayName string `json:"displayName"`
		}
		if err := json.Unmarshal(body, &users); err != nil {
			apiErr := &jrerrors.APIError{ErrorType: "connection_error", Message: "failed to parse user search response: " + err.Error()}
			apiErr.WriteJSON(c.Stderr)
			return jrerrors.ExitError
		}
		if len(users) == 0 {
			apiErr := &jrerrors.APIError{ErrorType: "not_found", Message: fmt.Sprintf("no user found matching %q", to)}
			apiErr.WriteJSON(c.Stderr)
			return jrerrors.ExitNotFound
		}
		accountID = users[0].AccountID
	}

	marshaledAssign, _ := json.Marshal(map[string]string{"accountId": accountID})
	assignBody := string(marshaledAssign)
	_, code := fetchJSONWithBody(c, ctx, "PUT",
		fmt.Sprintf("/rest/api/3/issue/%s/assignee", issueKey),
		strings.NewReader(assignBody))
	if code != jrerrors.ExitOK {
		return code
	}

	out, _ := json.Marshal(map[string]string{"status": "assigned", "issue": issueKey, "to": to})
	fmt.Fprintf(c.Stdout, "%s\n", out)
	return jrerrors.ExitOK
}

// buildBatchResult constructs a BatchResult from captured stdout/stderr.
func buildBatchResult(index, exitCode int, stdoutBuf, stderrBuf *strings.Builder) BatchResult {
	if exitCode != jrerrors.ExitOK {
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
