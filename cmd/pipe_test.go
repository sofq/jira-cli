package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/sofq/jira-cli/cmd/generated"
	"github.com/sofq/jira-cli/internal/client"
	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/sofq/jira-cli/internal/jq"
)

// --- parseCommandString ---

func TestParseCommandString_Basic(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCommand string
		wantArgs    map[string]string
		wantErr     bool
	}{
		{
			name:        "resource and verb only",
			input:       "issue get",
			wantCommand: "issue get",
			wantArgs:    map[string]string{},
		},
		{
			name:        "with flag value",
			input:       "issue get --issueIdOrKey PROJ-1",
			wantCommand: "issue get",
			wantArgs:    map[string]string{"issueIdOrKey": "PROJ-1"},
		},
		{
			name:        "multiple flags",
			input:       "issue get --issueIdOrKey PROJ-1 --fields summary",
			wantCommand: "issue get",
			wantArgs:    map[string]string{"issueIdOrKey": "PROJ-1", "fields": "summary"},
		},
		{
			name:        "single-quoted value with spaces",
			input:       "search search-and-reconsile-issues-using-jql --jql 'project=PROJ AND status=Open'",
			wantCommand: "search search-and-reconsile-issues-using-jql",
			wantArgs:    map[string]string{"jql": "project=PROJ AND status=Open"},
		},
		{
			name:        "double-quoted value with spaces",
			input:       `search search-and-reconsile-issues-using-jql --jql "project=PROJ"`,
			wantCommand: "search search-and-reconsile-issues-using-jql",
			wantArgs:    map[string]string{"jql": "project=PROJ"},
		},
		{
			name:    "only one token",
			input:   "issue",
			wantErr: true,
		},
		{
			name:    "unclosed single quote",
			input:   "issue get --jql 'unclosed",
			wantErr: true,
		},
		{
			name:    "unclosed double quote",
			input:   `issue get --jql "unclosed`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bop, err := parseCommandString(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if bop.Command != tc.wantCommand {
				t.Errorf("command: got %q, want %q", bop.Command, tc.wantCommand)
			}
			for k, want := range tc.wantArgs {
				if got := bop.Args[k]; got != want {
					t.Errorf("arg %q: got %q, want %q", k, got, want)
				}
			}
		})
	}
}

// --- shellSplit ---

func TestShellSplit(t *testing.T) {
	tests := []struct {
		input   string
		want    []string
		wantErr bool
	}{
		{"a b c", []string{"a", "b", "c"}, false},
		{"a 'b c' d", []string{"a", "b c", "d"}, false},
		{`a "b c" d`, []string{"a", "b c", "d"}, false},
		{"a 'unclosed", nil, true},
		{`a "unclosed`, nil, true},
		{"  ", nil, false},
		{"single", []string{"single"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := shellSplit(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("token count: got %d %v, want %d %v", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("token[%d]: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// --- cloneBopWithInject ---

func TestCloneBopWithInject(t *testing.T) {
	t.Run("injects string value", func(t *testing.T) {
		base := BatchOp{
			Command: "issue get",
			Args:    map[string]string{"fields": "summary"},
		}
		raw := json.RawMessage(`"PROJ-1"`)
		bop := cloneBopWithInject(base, "issueIdOrKey", raw)

		if bop.Args["issueIdOrKey"] != "PROJ-1" {
			t.Errorf("expected PROJ-1, got %q", bop.Args["issueIdOrKey"])
		}
		if bop.Args["fields"] != "summary" {
			t.Errorf("fields arg missing or wrong: %q", bop.Args["fields"])
		}
		// Mutation must not affect template.
		if _, ok := base.Args["issueIdOrKey"]; ok {
			t.Error("template args were mutated")
		}
	})

	t.Run("injects non-string JSON value verbatim", func(t *testing.T) {
		base := BatchOp{Command: "issue get", Args: map[string]string{}}
		bop := cloneBopWithInject(base, "num", json.RawMessage(`42`))
		if bop.Args["num"] != "42" {
			t.Errorf("expected '42', got %q", bop.Args["num"])
		}
	})

	t.Run("preserves command and jq", func(t *testing.T) {
		base := BatchOp{Command: "issue get", JQ: ".key", Args: map[string]string{}}
		bop := cloneBopWithInject(base, "x", json.RawMessage(`"v"`))
		if bop.Command != "issue get" {
			t.Errorf("command mismatch: %q", bop.Command)
		}
		if bop.JQ != ".key" {
			t.Errorf("jq mismatch: %q", bop.JQ)
		}
	})
}

// --- shared test helpers ---

// newPipeClient creates a test client pointing at serverURL.
func newPipeClient(serverURL string, stdout, stderr *bytes.Buffer) *client.Client {
	return &client.Client{
		BaseURL:    serverURL,
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: &http.Client{},
		Stdout:     stdout,
		Stderr:     stderr,
		Paginate:   false,
	}
}

// pipeCtx stores c in a background context.
func pipeCtx(c *client.Client) context.Context {
	return client.NewContext(context.Background(), c)
}

// buildTestOpMap assembles the full operation lookup table for tests.
func buildTestOpMap() map[string]generated.SchemaOp {
	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)
	allOps = append(allOps, TemplateSchemaOps()...)
	allOps = append(allOps, DiffSchemaOps()...)
	m := make(map[string]generated.SchemaOp, len(allOps))
	for _, op := range allOps {
		m[op.Resource+" "+op.Verb] = op
	}
	return m
}

// mustExtract applies jq filter and unmarshals the result as a JSON array.
func mustExtract(t *testing.T, data json.RawMessage, filter string) []json.RawMessage {
	t.Helper()
	out, err := jq.Apply(data, filter)
	if err != nil {
		t.Fatalf("jq.Apply(%q): %v", filter, err)
	}
	var vals []json.RawMessage
	if err := json.Unmarshal(out, &vals); err != nil {
		t.Fatalf("extract result is not a JSON array (%q): %v", filter, err)
	}
	return vals
}

// --- TestPipe_BasicIssueGet ---

// TestPipe_BasicIssueGet verifies the full pipe pattern: source returns a
// search result, extract pulls out issue keys, target fetches each issue.
func TestPipe_BasicIssueGet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Source: JQL search returns two issues.
		if r.URL.Query().Get("jql") != "" || strings.Contains(r.URL.Path, "/search") {
			fmt.Fprint(w, `{"issues":[{"key":"PROJ-1"},{"key":"PROJ-2"}]}`)
			return
		}
		// Target: individual issue fetch.
		for _, k := range []string{"PROJ-1", "PROJ-2"} {
			if strings.HasSuffix(r.URL.Path, "/"+k) {
				fmt.Fprintf(w, `{"key":%q,"fields":{"summary":"Test"}}`, k)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newPipeClient(ts.URL, &stdout, &stderr)
	opMap := buildTestOpMap()

	fakeCmd := batchCmd
	fakeCmd.SetContext(pipeCtx(c))

	// Step 1: execute source.
	sourceBop, err := parseCommandString("search search-and-reconsile-issues-using-jql --jql 'project=PROJ'")
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	sourceResult := executeBatchOp(fakeCmd, c, 0, sourceBop, opMap)
	if sourceResult.ExitCode != jrerrors.ExitOK {
		t.Fatalf("source failed (code=%d): %s", sourceResult.ExitCode, sourceResult.Error)
	}

	// Step 2: extract keys with default filter.
	vals := mustExtract(t, sourceResult.Data, "[.issues[].key]")
	if len(vals) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(vals))
	}

	// Step 3: run target for each value.
	targetTmpl, _ := parseCommandString("issue get")
	results := make([]BatchResult, len(vals))
	for i, v := range vals {
		bop := cloneBopWithInject(targetTmpl, "issueIdOrKey", v)
		results[i] = executeBatchOp(fakeCmd, c, i, bop, opMap)
	}

	// Collect and validate the returned issue keys.
	var gotKeys []string
	for _, r := range results {
		if r.ExitCode != jrerrors.ExitOK {
			t.Errorf("target[%d] failed: %v", r.Index, r.Error)
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(r.Data, &obj); err != nil {
			t.Errorf("result[%d] data not JSON object: %v", r.Index, err)
			continue
		}
		var k string
		_ = json.Unmarshal(obj["key"], &k)
		gotKeys = append(gotKeys, k)
	}
	sort.Strings(gotKeys)
	if len(gotKeys) != 2 || gotKeys[0] != "PROJ-1" || gotKeys[1] != "PROJ-2" {
		t.Errorf("unexpected keys: %v", gotKeys)
	}
}

// --- TestPipe_CustomExtractAndInject ---

// TestPipe_CustomExtractAndInject verifies custom --extract and --inject values
// by piping a project list into individual project fetches.
func TestPipe_CustomExtractAndInject(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/project/AAAA"):
			fmt.Fprint(w, `{"key":"AAAA","name":"Project A"}`)
		case strings.HasSuffix(r.URL.Path, "/project/BBBB"):
			fmt.Fprint(w, `{"key":"BBBB","name":"Project B"}`)
		case strings.Contains(r.URL.Path, "/project"):
			fmt.Fprint(w, `{"values":[{"key":"AAAA"},{"key":"BBBB"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not found"}`)
		}
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newPipeClient(ts.URL, &stdout, &stderr)
	opMap := buildTestOpMap()

	fakeCmd := batchCmd
	fakeCmd.SetContext(pipeCtx(c))

	// Source: project search.
	sourceBop, err := parseCommandString("project search")
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	sourceResult := executeBatchOp(fakeCmd, c, 0, sourceBop, opMap)
	if sourceResult.ExitCode != jrerrors.ExitOK {
		t.Fatalf("source failed: %s", sourceResult.Error)
	}

	// Custom extract: pull project keys from "values" array.
	vals := mustExtract(t, sourceResult.Data, "[.values[].key]")
	if len(vals) != 2 {
		t.Fatalf("expected 2 vals, got %d", len(vals))
	}

	// Target: project get, inject into "projectIdOrKey".
	targetTmpl, _ := parseCommandString("project get")
	results := make([]BatchResult, len(vals))
	for i, v := range vals {
		bop := cloneBopWithInject(targetTmpl, "projectIdOrKey", v)
		results[i] = executeBatchOp(fakeCmd, c, i, bop, opMap)
	}

	var names []string
	for _, r := range results {
		if r.ExitCode != jrerrors.ExitOK {
			t.Errorf("target[%d] failed: %v", r.Index, r.Error)
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(r.Data, &obj); err != nil {
			t.Errorf("result[%d] not JSON: %v", r.Index, err)
			continue
		}
		var name string
		_ = json.Unmarshal(obj["name"], &name)
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) != 2 || names[0] != "Project A" || names[1] != "Project B" {
		t.Errorf("unexpected names: %v", names)
	}
}

// --- TestPipe_ParallelExecution ---

// TestPipe_ParallelExecution verifies that running target ops concurrently
// (parallel > 1) produces all expected results without data races.
func TestPipe_ParallelExecution(t *testing.T) {
	const n = 5

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		for i := 1; i <= n; i++ {
			key := fmt.Sprintf("PROJ-%d", i)
			if strings.HasSuffix(r.URL.Path, "/"+key) {
				fmt.Fprintf(w, `{"key":%q}`, key)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newPipeClient(ts.URL, &stdout, &stderr)
	opMap := buildTestOpMap()

	fakeCmd := batchCmd
	fakeCmd.SetContext(pipeCtx(c))

	// Pre-build values as if they were extracted from a source.
	values := make([]json.RawMessage, n)
	for i := 0; i < n; i++ {
		v, _ := json.Marshal(fmt.Sprintf("PROJ-%d", i+1))
		values[i] = json.RawMessage(v)
	}

	targetTmpl, _ := parseCommandString("issue get")
	results := make([]BatchResult, n)

	// Parallel execution identical to runPipe's goroutine fan-out.
	type work struct {
		index int
		val   json.RawMessage
	}
	workCh := make(chan work, n)
	for i, v := range values {
		workCh <- work{i, v}
	}
	close(workCh)

	var mu sync.Mutex
	var wg sync.WaitGroup
	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range workCh {
				bop := cloneBopWithInject(targetTmpl, "issueIdOrKey", item.val)
				r := executeBatchOp(fakeCmd, c, item.index, bop, opMap)
				mu.Lock()
				results[item.index] = r
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Verify all results are populated and successful.
	var gotKeys []string
	for i, r := range results {
		if r.ExitCode != jrerrors.ExitOK {
			t.Errorf("result[%d] failed: %v", i, r.Error)
			continue
		}
		if r.Data == nil {
			t.Errorf("result[%d] has nil data", i)
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(r.Data, &obj); err != nil {
			t.Errorf("result[%d] data not JSON: %v", i, err)
			continue
		}
		var k string
		_ = json.Unmarshal(obj["key"], &k)
		gotKeys = append(gotKeys, k)
	}

	sort.Strings(gotKeys)
	if len(gotKeys) != n {
		t.Fatalf("expected %d keys, got %d: %v", n, len(gotKeys), gotKeys)
	}
	for i := 0; i < n; i++ {
		want := fmt.Sprintf("PROJ-%d", i+1)
		if gotKeys[i] != want {
			t.Errorf("key[%d]: got %q, want %q", i, gotKeys[i], want)
		}
	}
}
