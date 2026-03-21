package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/sofq/jira-cli/cmd/generated"
	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/sofq/jira-cli/internal/jq"
	"github.com/spf13/cobra"
)

var pipeCmd = &cobra.Command{
	Use:   "pipe <source-command> <target-command>",
	Short: "Execute source command, extract values, run target for each value",
	Long: `Execute a source command, extract an array of values from the output using
a jq expression, then run the target command once per value with the value
injected as a named argument.

Examples:

  # Get each issue returned by a search
  jr pipe "search search-and-reconsile-issues-using-jql --jql 'project=PROJ'" "issue get" \
    --extract "[.issues[].key]" --inject issueIdOrKey

  # Run a target command for each value, parallel with 4 workers
  jr pipe "issue get --issueIdOrKey PROJ-1" "issue get" \
    --extract "[.key]" --inject issueIdOrKey --parallel 4

Output is a JSON array of BatchResult objects (same shape as jr batch):
  [{"index":0,"exit_code":0,"data":{...}}, ...]`,
	Args: cobra.ExactArgs(2),
	RunE: runPipe,
}

func init() {
	pipeCmd.Flags().String("extract", "[.issues[].key]", "jq expression to extract an array of values from source output")
	pipeCmd.Flags().String("inject", "issue", "argument name to inject the extracted value into target command")
	pipeCmd.Flags().Int("parallel", 1, "number of target operations to run concurrently")
	rootCmd.AddCommand(pipeCmd)
}

// parseCommandString splits a command string like
// "issue get --issueIdOrKey PROJ-1" into a BatchOp.
// The first two whitespace-separated tokens become the command ("resource verb"),
// remaining tokens are parsed as --flag value pairs.
func parseCommandString(raw string) (BatchOp, error) {
	tokens, err := shellSplit(raw)
	if err != nil {
		return BatchOp{}, fmt.Errorf("cannot parse command %q: %w", raw, err)
	}
	if len(tokens) < 2 {
		return BatchOp{}, fmt.Errorf("command must have at least <resource> <verb>, got %q", raw)
	}

	op := BatchOp{
		Command: tokens[0] + " " + tokens[1],
		Args:    make(map[string]string),
	}

	// Parse remaining tokens as --flag [value] pairs.
	rest := tokens[2:]
	for i := 0; i < len(rest); i++ {
		tok := rest[i]
		if strings.HasPrefix(tok, "--") {
			flagName := strings.TrimPrefix(tok, "--")
			if i+1 < len(rest) && !strings.HasPrefix(rest[i+1], "--") {
				op.Args[flagName] = rest[i+1]
				i++
			} else {
				// Boolean flag — treat as empty string to indicate presence.
				op.Args[flagName] = ""
			}
		}
	}
	return op, nil
}

// shellSplit splits a command string into tokens, respecting single and double
// quotes so that flag values with spaces survive intact.
func shellSplit(s string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case c == ' ' && !inSingle && !inDouble:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(c)
		}
	}

	if inSingle || inDouble {
		return nil, fmt.Errorf("unclosed quote in command string")
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens, nil
}

func runPipe(cmd *cobra.Command, args []string) error {
	baseClient, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "config_error",
			Message:   err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	sourceRaw := args[0]
	targetRaw := args[1]

	extractExpr, _ := cmd.Flags().GetString("extract")
	injectArg, _ := cmd.Flags().GetString("inject")
	parallel, _ := cmd.Flags().GetInt("parallel")

	if parallel < 1 {
		parallel = 1
	}

	// Build the shared opMap once.
	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)
	allOps = append(allOps, TemplateSchemaOps()...)
	allOps = append(allOps, DiffSchemaOps()...)
	opMap := make(map[string]generated.SchemaOp, len(allOps))
	for _, op := range allOps {
		opMap[op.Resource+" "+op.Verb] = op
	}

	// --- Step 1: parse and execute the source command ---
	sourceBop, err := parseCommandString(sourceRaw)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   "source command: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	sourceResult := executeBatchOp(cmd, baseClient, 0, sourceBop, opMap)
	if sourceResult.ExitCode != jrerrors.ExitOK {
		// Forward the source error to stderr and exit.
		if sourceResult.Error != nil {
			fmt.Fprintf(os.Stderr, "%s\n", sourceResult.Error)
		}
		return &jrerrors.AlreadyWrittenError{Code: sourceResult.ExitCode}
	}

	sourceData := sourceResult.Data
	if sourceData == nil {
		sourceData = json.RawMessage("null")
	}

	// --- Step 2: apply extract jq filter to get an array of values ---
	extracted, err := jq.Apply(sourceData, extractExpr)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "jq_error",
			Message:   "extract: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	var values []json.RawMessage
	if err := json.Unmarshal(extracted, &values); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   fmt.Sprintf("--extract must produce a JSON array, got: %s", string(extracted)),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	if len(values) == 0 {
		// Nothing to pipe — emit an empty array and succeed.
		fmt.Fprintf(os.Stdout, "[]\n")
		return nil
	}

	// --- Step 3: parse target command template ---
	targetBopTemplate, err := parseCommandString(targetRaw)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   "target command: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	// --- Step 4: execute target ops (sequentially or in parallel) ---
	results := make([]BatchResult, len(values))

	// Extract context before spawning goroutines to avoid concurrent
	// access to *cobra.Command (which is not goroutine-safe).
	ctx := cmd.Context()

	if parallel <= 1 {
		for i, rawVal := range values {
			bop := cloneBopWithInject(targetBopTemplate, injectArg, rawVal)
			results[i] = executeBatchOpWithContext(ctx, baseClient, i, bop, opMap)
		}
	} else {
		type work struct {
			index int
			val   json.RawMessage
		}
		workCh := make(chan work, len(values))
		for i, v := range values {
			workCh <- work{i, v}
		}
		close(workCh)

		var mu sync.Mutex
		var wg sync.WaitGroup

		for w := 0; w < parallel; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for item := range workCh {
					bop := cloneBopWithInject(targetBopTemplate, injectArg, item.val)
					r := executeBatchOpWithContext(ctx, baseClient, item.index, bop, opMap)
					mu.Lock()
					results[item.index] = r
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
	}

	// --- Step 5: write results as a JSON array ---
	var resultBuf bytes.Buffer
	enc := json.NewEncoder(&resultBuf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(results)

	output := bytes.TrimRight(resultBuf.Bytes(), "\n")

	// Apply global --jq filter if set.
	if baseClient.JQFilter != "" {
		filtered, err := jq.Apply(output, baseClient.JQFilter)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "jq_error",
				Message:   "jq: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
		}
		output = filtered
	}

	fmt.Fprintf(os.Stdout, "%s\n", output)

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

// cloneBopWithInject copies the target BatchOp template and injects the
// extracted value (stripped of JSON string quotes for plain strings) as the
// named argument.
func cloneBopWithInject(tmpl BatchOp, argName string, rawVal json.RawMessage) BatchOp {
	bop := BatchOp{
		Command: tmpl.Command,
		JQ:      tmpl.JQ,
		Args:    make(map[string]string, len(tmpl.Args)+1),
	}
	for k, v := range tmpl.Args {
		bop.Args[k] = v
	}

	// Unwrap JSON string values so that "PROJ-1" becomes PROJ-1.
	var s string
	if err := json.Unmarshal(rawVal, &s); err == nil {
		bop.Args[argName] = s
	} else {
		bop.Args[argName] = string(rawVal)
	}
	return bop
}
