package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/sofq/jira-cli/cmd/generated"
	"github.com/sofq/jira-cli/internal/adf"
	"github.com/sofq/jira-cli/internal/client"
	"github.com/sofq/jira-cli/internal/duration"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/sofq/jira-cli/internal/changelog"
	"github.com/sofq/jira-cli/internal/jq"
	tmpl "github.com/sofq/jira-cli/internal/template"
	"github.com/spf13/cobra"
	"github.com/tidwall/pretty"
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
    {"command": "project search", "args": {}}
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
	batchCmd.Flags().Int("max-batch", 50, "maximum number of operations per batch")
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
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
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
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
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
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
		}
		inputData, err = io.ReadAll(os.Stdin)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "validation_error",
				Status:    0,
				Message:   "failed to read stdin: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
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
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	// Reject null/empty input explicitly.
	if ops == nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Status:    0,
			Message:   "invalid JSON input: expected a JSON array of operations, got null",
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	// Enforce batch size limit.
	maxBatch, _ := cmd.Flags().GetInt("max-batch")
	if maxBatch > 0 && len(ops) > maxBatch {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   fmt.Sprintf("batch limit exceeded: %d operations, max %d", len(ops), maxBatch),
			Hint:      "Use --max-batch to increase the limit",
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	// Load all schema ops for lookup (generated + hand-written).
	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)
	allOps = append(allOps, TemplateSchemaOps()...)
	allOps = append(allOps, DiffSchemaOps()...)
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
	var resultBuf bytes.Buffer
	enc := json.NewEncoder(&resultBuf)
	enc.SetEscapeHTML(false)
	// BatchResult contains only int, string, and json.RawMessage fields —
	// json.Encode cannot fail.
	_ = enc.Encode(results)

	output := bytes.TrimRight(resultBuf.Bytes(), "\n")

	// Apply global --jq filter to the batch output array.
	if baseClient.JQFilter != "" {
		filtered, err := jq.Apply(output, baseClient.JQFilter)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "jq_error",
				Status:    0,
				Message:   "jq: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
		}
		output = filtered
	}

	// Apply --pretty to the batch output.
	if baseClient.Pretty {
		output = pretty.Pretty(output)
	}

	fmt.Fprintf(os.Stdout, "%s\n", output)

	// Exit with highest-severity exit code from batch operations.
	maxExit := 0
	for _, r := range results {
		if r.ExitCode > maxExit {
			maxExit = r.ExitCode
		}
	}
	if maxExit != 0 {
		return &jrerrors.AlreadyWrittenError{Code: maxExit}
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

	// Check policy for this operation.
	if baseClient.Policy != nil {
		if err := baseClient.Policy.Check(bop.Command); err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "validation_error",
				Message:   err.Error(),
			}
			encoded, _ := json.Marshal(apiErr)
			return BatchResult{
				Index:    index,
				ExitCode: jrerrors.ExitValidation,
				Error:    json.RawMessage(encoded),
			}
		}
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
		Pretty:      false, // Pretty is applied to the batch output array, not per-op.
		Fields:      baseClient.Fields,
		CacheTTL:    baseClient.CacheTTL,
		AuditLogger: baseClient.AuditLogger,
		Profile:     baseClient.Profile,
		Operation:   bop.Command,
	}

	// Hand-written commands have custom execution logic.
	if strings.HasPrefix(bop.Command, "workflow ") {
		exitCode := executeBatchWorkflow(cmd.Context(), opClient, bop)
		return buildBatchResult(index, exitCode, &stdoutBuf, &stderrBuf, baseClient.Verbose)
	}
	if strings.HasPrefix(bop.Command, "template ") {
		exitCode := executeBatchTemplate(cmd.Context(), opClient, bop)
		return buildBatchResult(index, exitCode, &stdoutBuf, &stderrBuf, baseClient.Verbose)
	}
	if bop.Command == "diff diff" {
		exitCode := executeBatchDiff(cmd.Context(), opClient, bop)
		return buildBatchResult(index, exitCode, &stdoutBuf, &stderrBuf, baseClient.Verbose)
	}

	// Build path by substituting {paramName} placeholders.
	path := schemaOp.Path
	for _, flag := range schemaOp.Flags {
		if flag.In == "path" {
			placeholder := "{" + flag.Name + "}"
			if val, exists := bop.Args[flag.Name]; exists && val != "" {
				path = strings.ReplaceAll(path, placeholder, url.PathEscape(val))
			} else if flag.Required && strings.Contains(path, placeholder) {
				errMsg := fmt.Sprintf("missing required path parameter %q", flag.Name)
				return errorResult(index, jrerrors.ExitValidation, "validation_error", errMsg)
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

	// If the batch op explicitly specifies "fields" in its args,
	// clear the per-op client's Fields so that client.Do() does not overwrite
	// the per-op value with the global --fields flag.
	if _, hasFields := bop.Args["fields"]; hasFields {
		opClient.Fields = ""
	}

	// Handle body.
	var body io.Reader
	if bodyStr, exists := bop.Args["body"]; exists && bodyStr != "" {
		body = strings.NewReader(bodyStr)
	}

	exitCode := opClient.Do(cmd.Context(), schemaOp.Method, path, query, body)

	return buildBatchResult(index, exitCode, &stdoutBuf, &stderrBuf, baseClient.Verbose)
}

// batchValidationError writes a validation error to the client's stderr and returns ExitValidation.
func batchValidationError(c *client.Client, message string) int {
	apiErr := &jrerrors.APIError{
		ErrorType: "validation_error",
		Message:   message,
	}
	apiErr.WriteJSON(c.Stderr)
	return jrerrors.ExitValidation
}

// executeBatchWorkflow runs workflow commands (transition, assign, comment, create, link) as a batch operation.
func executeBatchWorkflow(ctx context.Context, c *client.Client, bop BatchOp) int {
	switch bop.Command {
	case "workflow create":
		return batchCreateIssue(ctx, c, bop.Args)
	case "workflow link":
		from := bop.Args["from"]
		if from == "" {
			return batchValidationError(c, "workflow link requires a 'from' arg")
		}
		to := bop.Args["to"]
		if to == "" {
			return batchValidationError(c, "workflow link requires a 'to' arg")
		}
		linkType := bop.Args["type"]
		if linkType == "" {
			return batchValidationError(c, "workflow link requires a 'type' arg")
		}
		return batchLink(ctx, c, from, to, linkType)
	}

	issueKey := bop.Args["issue"]
	if issueKey == "" {
		return batchValidationError(c, "workflow commands require an 'issue' arg")
	}

	switch bop.Command {
	case "workflow transition":
		to := bop.Args["to"]
		if to == "" {
			return batchValidationError(c, "workflow transition requires a 'to' arg")
		}
		return batchTransition(ctx, c, issueKey, to)
	case "workflow move":
		to := bop.Args["to"]
		if to == "" {
			return batchValidationError(c, "workflow move requires a 'to' arg")
		}
		return batchMove(ctx, c, issueKey, to, bop.Args["assign"])
	case "workflow assign":
		to := bop.Args["to"]
		if to == "" {
			return batchValidationError(c, "workflow assign requires a 'to' arg")
		}
		return batchAssign(ctx, c, issueKey, to)
	case "workflow comment":
		text := bop.Args["text"]
		if text == "" {
			return batchValidationError(c, "workflow comment requires a 'text' arg")
		}
		return batchComment(ctx, c, issueKey, text)
	case "workflow log-work":
		timeStr := bop.Args["time"]
		if timeStr == "" {
			return batchValidationError(c, "workflow log-work requires a 'time' arg")
		}
		return batchLogWork(ctx, c, issueKey, timeStr, bop.Args["comment"])
	case "workflow sprint":
		to := bop.Args["to"]
		if to == "" {
			return batchValidationError(c, "workflow sprint requires a 'to' arg")
		}
		return batchSprint(ctx, c, issueKey, to)
	default:
		return batchValidationError(c, fmt.Sprintf("unknown workflow command %q", bop.Command))
	}
}

// batchTransition executes a workflow transition within a batch operation.
func batchTransition(ctx context.Context, c *client.Client, issueKey, toStatus string) int {
	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "POST",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/transitions", url.PathEscape(issueKey)),
			"note":   fmt.Sprintf("would transition %s to %q (transition ID resolved at runtime)", issueKey, toStatus),
		})
		return c.WriteOutput(out)
	}

	match, code := resolveTransition(ctx, c, issueKey, toStatus)
	if code != jrerrors.ExitOK {
		return code
	}

	transBody, _ := json.Marshal(map[string]any{"transition": map[string]any{"id": match.ID}})
	_, code = c.Fetch(ctx, "POST",
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", url.PathEscape(issueKey)),
		bytes.NewReader(transBody))
	if code != jrerrors.ExitOK {
		return code
	}

	out, _ := marshalNoEscape(map[string]string{
		"status":     "transitioned",
		"issue":      issueKey,
		"transition": match.Name,
	})
	return c.WriteOutput(out)
}

// batchMove executes a workflow move (transition + optional assign) within a batch operation.
func batchMove(ctx context.Context, c *client.Client, issueKey, toStatus, assign string) int {
	if c.DryRun {
		dryOut := map[string]any{
			"method": "POST",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/transitions", url.PathEscape(issueKey)),
			"note":   fmt.Sprintf("would transition %s to %q (transition ID resolved at runtime)", issueKey, toStatus),
		}
		if assign != "" {
			dryOut["assign"] = fmt.Sprintf("would assign %s to %q (account ID resolved at runtime)", issueKey, assign)
		}
		out, _ := marshalNoEscape(dryOut)
		return c.WriteOutput(out)
	}

	match, code := resolveTransition(ctx, c, issueKey, toStatus)
	if code != jrerrors.ExitOK {
		return code
	}

	transBody, _ := json.Marshal(map[string]any{"transition": map[string]any{"id": match.ID}})
	_, code = c.Fetch(ctx, "POST",
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", url.PathEscape(issueKey)),
		bytes.NewReader(transBody))
	if code != jrerrors.ExitOK {
		return code
	}

	result := map[string]any{
		"status":     "moved",
		"issue":      issueKey,
		"transition": match.Name,
	}

	if assign != "" {
		accountID, isUnassign, code := resolveAssignee(ctx, c, assign)
		if code != jrerrors.ExitOK {
			return code
		}

		if isUnassign {
			assignBody := `{"accountId":null}`
			_, code = c.Fetch(ctx, "PUT",
				fmt.Sprintf("/rest/api/3/issue/%s/assignee", url.PathEscape(issueKey)),
				strings.NewReader(assignBody))
			if code != jrerrors.ExitOK {
				return code
			}
			result["assigned"] = "unassigned"
		} else {
			marshaledAssign, _ := json.Marshal(map[string]string{"accountId": accountID})
			_, code = c.Fetch(ctx, "PUT",
				fmt.Sprintf("/rest/api/3/issue/%s/assignee", url.PathEscape(issueKey)),
				bytes.NewReader(marshaledAssign))
			if code != jrerrors.ExitOK {
				return code
			}
			result["assigned"] = assign
		}
	}

	out, _ := marshalNoEscape(result)
	return c.WriteOutput(out)
}

// batchAssign executes a workflow assign within a batch operation.
func batchAssign(ctx context.Context, c *client.Client, issueKey, to string) int {
	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "PUT",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/assignee", url.PathEscape(issueKey)),
			"note":   fmt.Sprintf("would assign %s to %q (account ID resolved at runtime)", issueKey, to),
		})
		return c.WriteOutput(out)
	}

	accountID, isUnassign, code := resolveAssignee(ctx, c, to)
	if code != jrerrors.ExitOK {
		return code
	}

	if isUnassign {
		assignBody := `{"accountId":null}`
		_, code := c.Fetch(ctx, "PUT",
			fmt.Sprintf("/rest/api/3/issue/%s/assignee", url.PathEscape(issueKey)),
			strings.NewReader(assignBody))
		if code != jrerrors.ExitOK {
			return code
		}
		out, _ := marshalNoEscape(map[string]string{"status": "unassigned", "issue": issueKey})
		return c.WriteOutput(out)
	}

	marshaledAssign, _ := json.Marshal(map[string]string{"accountId": accountID})
	_, code = c.Fetch(ctx, "PUT",
		fmt.Sprintf("/rest/api/3/issue/%s/assignee", url.PathEscape(issueKey)),
		bytes.NewReader(marshaledAssign))
	if code != jrerrors.ExitOK {
		return code
	}

	out, _ := marshalNoEscape(map[string]string{"status": "assigned", "issue": issueKey, "to": to})
	return c.WriteOutput(out)
}

// batchComment executes a workflow comment within a batch operation.
func batchComment(ctx context.Context, c *client.Client, issueKey, text string) int {
	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "POST",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/comment", url.PathEscape(issueKey)),
			"note":   fmt.Sprintf("would add comment to %s", issueKey),
		})
		return c.WriteOutput(out)
	}

	commentBody, _ := json.Marshal(map[string]any{"body": adf.FromText(text)})
	_, code := c.Fetch(ctx, "POST",
		fmt.Sprintf("/rest/api/3/issue/%s/comment", url.PathEscape(issueKey)),
		bytes.NewReader(commentBody))
	if code != jrerrors.ExitOK {
		return code
	}

	out, _ := marshalNoEscape(map[string]string{
		"status": "commented",
		"issue":  issueKey,
	})
	return c.WriteOutput(out)
}

// batchLink executes a workflow link within a batch operation.
func batchLink(ctx context.Context, c *client.Client, from, to, linkType string) int {
	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "POST",
			"url":    c.BaseURL + "/rest/api/3/issueLink",
			"note":   fmt.Sprintf("would link %s -> %s with type %q (link type ID resolved at runtime)", from, to, linkType),
		})
		return c.WriteOutput(out)
	}

	resolvedType, code := resolveLinkType(ctx, c, linkType)
	if code != jrerrors.ExitOK {
		return code
	}

	linkBody, _ := json.Marshal(map[string]any{
		"type":         map[string]string{"name": resolvedType},
		"inwardIssue":  map[string]string{"key": to},
		"outwardIssue": map[string]string{"key": from},
	})
	_, code = c.Fetch(ctx, "POST", "/rest/api/3/issueLink", bytes.NewReader(linkBody))
	if code != jrerrors.ExitOK {
		return code
	}

	out, _ := marshalNoEscape(map[string]string{
		"status": "linked",
		"from":   from,
		"to":     to,
		"type":   resolvedType,
	})
	return c.WriteOutput(out)
}

// batchCreateIssue executes a workflow create within a batch operation.
func batchCreateIssue(ctx context.Context, c *client.Client, args map[string]string) int {
	project := args["project"]
	if project == "" {
		return batchValidationError(c, "workflow create requires a 'project' arg")
	}
	issueType := args["type"]
	if issueType == "" {
		return batchValidationError(c, "workflow create requires a 'type' arg")
	}
	summary := args["summary"]
	if summary == "" {
		return batchValidationError(c, "workflow create requires a 'summary' arg")
	}

	fields := map[string]any{
		"project":   map[string]string{"key": project},
		"issuetype": map[string]string{"name": issueType},
		"summary":   summary,
	}
	if description := args["description"]; description != "" {
		fields["description"] = adf.FromText(description)
	}
	if priority := args["priority"]; priority != "" {
		fields["priority"] = map[string]string{"name": priority}
	}
	if labels := args["labels"]; labels != "" {
		fields["labels"] = strings.Split(labels, ",")
	}
	if parent := args["parent"]; parent != "" {
		fields["parent"] = map[string]string{"key": parent}
	}

	if c.DryRun {
		bodyPreview, _ := marshalNoEscape(map[string]any{"fields": fields})
		out, _ := marshalNoEscape(map[string]any{
			"method": "POST",
			"url":    c.BaseURL + "/rest/api/3/issue",
			"body":   json.RawMessage(bodyPreview),
		})
		return c.WriteOutput(out)
	}

	if assign := args["assign"]; assign != "" {
		accountID, isUnassign, code := resolveAssignee(ctx, c, assign)
		if code != jrerrors.ExitOK {
			return code
		}
		if !isUnassign {
			fields["assignee"] = map[string]string{"accountId": accountID}
		}
	}

	createBody, _ := json.Marshal(map[string]any{"fields": fields})
	respBody, code := c.Fetch(ctx, "POST", "/rest/api/3/issue", bytes.NewReader(createBody))
	if code != jrerrors.ExitOK {
		return code
	}

	return c.WriteOutput(respBody)
}

// batchSprint executes a workflow sprint (move issue to sprint) within a batch operation.
func batchSprint(ctx context.Context, c *client.Client, issueKey, sprintName string) int {
	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "POST",
			"url":    c.BaseURL + "/rest/agile/1.0/sprint/{sprintId}/issue",
			"note":   fmt.Sprintf("would move %s to sprint %q (sprint ID resolved at runtime)", issueKey, sprintName),
		})
		return c.WriteOutput(out)
	}

	sprint, code := resolveSprint(ctx, c, sprintName)
	if code != jrerrors.ExitOK {
		return code
	}

	sprintBody, _ := json.Marshal(map[string]any{"issues": []string{issueKey}})
	_, code = c.Fetch(ctx, "POST",
		fmt.Sprintf("/rest/agile/1.0/sprint/%d/issue", sprint.ID),
		bytes.NewReader(sprintBody))
	if code != jrerrors.ExitOK {
		return code
	}

	out, _ := marshalNoEscape(map[string]string{
		"status": "sprint_set",
		"issue":  issueKey,
		"sprint": sprint.Name,
	})
	return c.WriteOutput(out)
}

// batchLogWork executes a workflow log-work within a batch operation.
func batchLogWork(ctx context.Context, c *client.Client, issueKey, timeStr, comment string) int {
	seconds, err := duration.Parse(timeStr)
	if err != nil {
		return batchValidationError(c, "invalid duration: "+err.Error())
	}

	if c.DryRun {
		out, _ := marshalNoEscape(map[string]any{
			"method":           "POST",
			"url":              c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/worklog", url.PathEscape(issueKey)),
			"timeSpentSeconds": seconds,
		})
		return c.WriteOutput(out)
	}

	worklogBody := map[string]any{
		"timeSpentSeconds": seconds,
	}
	if comment != "" {
		worklogBody["comment"] = adf.FromText(comment)
	}

	body, _ := json.Marshal(worklogBody)
	_, code := c.Fetch(ctx, "POST",
		fmt.Sprintf("/rest/api/3/issue/%s/worklog", url.PathEscape(issueKey)),
		bytes.NewReader(body))
	if code != jrerrors.ExitOK {
		return code
	}

	out, _ := marshalNoEscape(map[string]string{
		"status": "logged",
		"issue":  issueKey,
		"time":   timeStr,
	})
	return c.WriteOutput(out)
}

// stripVerboseLogs separates verbose log lines from error lines in captured stderr.
// Verbose lines (with "type":"request"/"response") are forwarded to real stderr.
// Returns only the non-verbose (error) lines joined as a single string.
func stripVerboseLogs(stderrStr string) string {
	var errorLines []string
	for _, line := range strings.Split(strings.TrimSpace(stderrStr), "\n") {
		if line == "" {
			continue
		}
		var parsed map[string]any
		if json.Unmarshal([]byte(line), &parsed) == nil {
			if tp, ok := parsed["type"].(string); ok && (tp == "request" || tp == "response") {
				fmt.Fprintln(os.Stderr, line)
				continue
			}
		}
		errorLines = append(errorLines, line)
	}
	return strings.Join(errorLines, "\n")
}

// parseErrorJSON parses stderr output into a json.RawMessage.
// Handles single JSON objects, multiple JSON lines, and plain text.
func parseErrorJSON(errOutput string) json.RawMessage {
	if json.Valid([]byte(errOutput)) {
		return json.RawMessage(errOutput)
	}
	lines := strings.Split(errOutput, "\n")
	if len(lines) > 1 {
		var jsonLines []json.RawMessage
		allValid := true
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if json.Valid([]byte(line)) {
				jsonLines = append(jsonLines, json.RawMessage(line))
			} else {
				allValid = false
				break
			}
		}
		// If all non-empty lines are valid JSON but the whole string is not
		// (checked above), there must be multiple JSON values.
		if allValid && len(jsonLines) > 0 {
			arrBytes, _ := json.Marshal(jsonLines)
			return json.RawMessage(arrBytes)
		}
	}
	encoded, _ := json.Marshal(map[string]string{"message": errOutput})
	return json.RawMessage(encoded)
}

// buildBatchResult constructs a BatchResult from captured stdout/stderr.
// Verbose log lines (from --verbose) are forwarded to real stderr so they
// are visible to the caller, while structured error JSON is kept in the result.
func buildBatchResult(index, exitCode int, stdoutBuf, stderrBuf *strings.Builder, verbose bool) BatchResult {
	stderrStr := stderrBuf.String()

	if verbose && stderrStr != "" {
		stderrStr = stripVerboseLogs(stderrStr)
	}

	if exitCode != jrerrors.ExitOK {
		return BatchResult{
			Index:    index,
			ExitCode: exitCode,
			Error:    parseErrorJSON(strings.TrimSpace(stderrStr)),
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

// executeBatchTemplate runs template commands as a batch operation.
func executeBatchTemplate(ctx context.Context, c *client.Client, bop BatchOp) int {
	switch bop.Command {
	case "template apply":
		return batchTemplateApply(ctx, c, bop.Args)
	default:
		return batchValidationError(c, fmt.Sprintf("unknown template command %q in batch; only 'template apply' is supported", bop.Command))
	}
}

// batchTemplateApply executes a template apply within a batch operation.
func batchTemplateApply(ctx context.Context, c *client.Client, args map[string]string) int {
	templateName := args["name"]
	if templateName == "" {
		return batchValidationError(c, "template apply requires a 'name' arg")
	}
	project := args["project"]
	if project == "" {
		return batchValidationError(c, "template apply requires a 'project' arg")
	}

	t, ok, err := tmpl.Lookup(templateName)
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: "failed to load templates: " + err.Error()}
		apiErr.WriteJSON(c.Stderr)
		return jrerrors.ExitError
	}
	if !ok {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   fmt.Sprintf("template %q not found", templateName),
		}
		apiErr.WriteJSON(c.Stderr)
		return jrerrors.ExitNotFound
	}

	// Build vars from args (anything that isn't name/project/assign).
	vars := make(map[string]string)
	for k, v := range args {
		if k != "name" && k != "project" && k != "assign" {
			vars[k] = v
		}
	}

	rendered, err := tmpl.RenderFields(t, vars)
	if err != nil {
		return batchValidationError(c, err.Error())
	}

	fields := map[string]any{
		"project":   map[string]string{"key": project},
		"issuetype": map[string]string{"name": t.IssueType},
	}
	if v, ok := rendered["summary"]; ok {
		fields["summary"] = v
	}
	if v, ok := rendered["description"]; ok {
		fields["description"] = adf.FromText(v)
	}
	if v, ok := rendered["priority"]; ok {
		fields["priority"] = map[string]string{"name": v}
	}
	if v, ok := rendered["labels"]; ok {
		fields["labels"] = strings.Split(v, ",")
	}
	if v, ok := vars["parent"]; ok && v != "" {
		fields["parent"] = map[string]string{"key": v}
	}

	if c.DryRun {
		bodyPreview, _ := marshalNoEscape(map[string]any{"fields": fields})
		out, _ := marshalNoEscape(map[string]any{
			"method":   "POST",
			"url":      c.BaseURL + "/rest/api/3/issue",
			"template": templateName,
			"body":     json.RawMessage(bodyPreview),
		})
		return c.WriteOutput(out)
	}

	if assign := args["assign"]; assign != "" {
		accountID, isUnassign, code := resolveAssignee(ctx, c, assign)
		if code != jrerrors.ExitOK {
			return code
		}
		if !isUnassign {
			fields["assignee"] = map[string]string{"accountId": accountID}
		}
	}

	createBody, _ := json.Marshal(map[string]any{"fields": fields})
	respBody, code := c.Fetch(ctx, "POST", "/rest/api/3/issue", bytes.NewReader(createBody))
	if code != jrerrors.ExitOK {
		return code
	}
	return c.WriteOutput(respBody)
}

// executeBatchDiff runs a diff command as a batch operation.
func executeBatchDiff(ctx context.Context, c *client.Client, bop BatchOp) int {
	issueKey := bop.Args["issue"]
	if issueKey == "" {
		return batchValidationError(c, "diff requires an 'issue' arg")
	}

	changelogPath := fmt.Sprintf("/rest/api/3/issue/%s/changelog", url.PathEscape(issueKey))

	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "GET",
			"url":    c.BaseURL + changelogPath,
			"note":   fmt.Sprintf("would fetch changelog for %s", issueKey),
		})
		return c.WriteOutput(out)
	}

	body, exitCode := c.Fetch(ctx, "GET", changelogPath, nil)
	if exitCode != jrerrors.ExitOK {
		return exitCode
	}

	opts := changelog.Options{
		Since: bop.Args["since"],
		Field: bop.Args["field"],
	}

	result, err := changelog.Parse(issueKey, body, opts)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return jrerrors.ExitValidation
	}

	out, _ := marshalNoEscape(result)
	return c.WriteOutput(out)
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
