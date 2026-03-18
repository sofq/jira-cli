package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/client"
	"github.com/spf13/cobra"
)

func TestRunTemplateList(t *testing.T) {
	output := captureStdout(t, func() {
		cmd := &cobra.Command{Use: "list"}
		cmd.Flags().String("jq", "", "")
		cmd.Flags().Bool("pretty", false, "")

		err := runTemplateList(cmd, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Should contain builtin templates.
	if !strings.Contains(output, "bug-report") {
		t.Errorf("expected output to contain 'bug-report', got: %s", output)
	}
	if !strings.Contains(output, "story") {
		t.Errorf("expected output to contain 'story', got: %s", output)
	}
	if !strings.Contains(output, "task") {
		t.Errorf("expected output to contain 'task', got: %s", output)
	}
	if !strings.Contains(output, "epic") {
		t.Errorf("expected output to contain 'epic', got: %s", output)
	}
	if !strings.Contains(output, "subtask") {
		t.Errorf("expected output to contain 'subtask', got: %s", output)
	}
	if !strings.Contains(output, "spike") {
		t.Errorf("expected output to contain 'spike', got: %s", output)
	}

	// Should be valid JSON.
	var result []map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(result) != 6 {
		t.Errorf("expected 6 templates, got %d", len(result))
	}
}

func TestRunTemplateShow(t *testing.T) {
	output := captureStdout(t, func() {
		cmd := &cobra.Command{Use: "show"}
		cmd.Flags().String("jq", "", "")
		cmd.Flags().Bool("pretty", false, "")

		err := runTemplateShow(cmd, []string{"bug-report"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if result["name"] != "bug-report" {
		t.Errorf("expected name 'bug-report', got %v", result["name"])
	}
	if result["issuetype"] != "Bug" {
		t.Errorf("expected issuetype 'Bug', got %v", result["issuetype"])
	}
}

func TestRunTemplateShowNotFound(t *testing.T) {
	var stderr bytes.Buffer
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	cmd := &cobra.Command{Use: "show"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")

	err := runTemplateShow(cmd, []string{"nonexistent"})

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	stderr = buf

	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
	if !strings.Contains(stderr.String(), "not_found") {
		t.Errorf("expected not_found error, got: %s", stderr.String())
	}
}

func TestRunTemplateApply(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/rest/api/3/issue") {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			fields, _ := body["fields"].(map[string]any)
			if fields == nil {
				t.Error("expected fields in body")
				w.WriteHeader(400)
				return
			}
			project, _ := fields["project"].(map[string]any)
			if project["key"] != "PROJ" {
				t.Errorf("expected project key PROJ, got %v", project["key"])
			}
			issuetype, _ := fields["issuetype"].(map[string]any)
			if issuetype["name"] != "Bug" {
				t.Errorf("expected issuetype Bug, got %v", issuetype["name"])
			}
			fmt.Fprintln(w, `{"id":"10001","key":"PROJ-1","self":"https://example.atlassian.net/rest/api/3/issue/10001"}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "apply"}
	cmd.Flags().String("project", "", "")
	cmd.Flags().StringArray("var", nil, "")
	cmd.Flags().String("assign", "", "")
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("var", "summary=Login broken")
	cmd.SetContext(ctx)

	err := runTemplateApply(cmd, []string{"bug-report"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout.String(), "PROJ-1") {
		t.Errorf("expected output to contain PROJ-1, got: %s", stdout.String())
	}
}

func TestRunTemplateApplyDryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("https://example.atlassian.net", &stdout, &stderr)
	c.DryRun = true

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "apply"}
	cmd.Flags().String("project", "", "")
	cmd.Flags().StringArray("var", nil, "")
	cmd.Flags().String("assign", "", "")
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("var", "summary=Test bug")
	cmd.SetContext(ctx)

	err := runTemplateApply(cmd, []string{"bug-report"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if result["method"] != "POST" {
		t.Errorf("expected method POST, got %v", result["method"])
	}
	if result["template"] != "bug-report" {
		t.Errorf("expected template 'bug-report', got %v", result["template"])
	}
}

func TestRunTemplateApplyMissingVar(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("https://example.atlassian.net", &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "apply"}
	cmd.Flags().String("project", "", "")
	cmd.Flags().StringArray("var", nil, "")
	cmd.Flags().String("assign", "", "")
	_ = cmd.Flags().Set("project", "PROJ")
	// Not setting required 'summary' var.
	cmd.SetContext(ctx)

	// Capture os.Stderr since validation errors are written there.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := runTemplateApply(cmd, []string{"bug-report"})

	w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error for missing required variable")
	}
	if !strings.Contains(stderrBuf.String(), "missing required variables") {
		t.Errorf("expected missing required variables error, got: %s", stderrBuf.String())
	}
}

func TestRunTemplateCreateScaffold(t *testing.T) {
	// Redirect user config dir to temp dir via HOME env var.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	output := captureStdout(t, func() {
		cmd := &cobra.Command{Use: "create"}
		cmd.Flags().String("from", "", "")

		err := runTemplateCreate(cmd, []string{"my-custom"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "created") {
		t.Errorf("expected output to contain 'created', got: %s", output)
	}
	if !strings.Contains(output, "my-custom") {
		t.Errorf("expected output to contain 'my-custom', got: %s", output)
	}
}

func TestRunTemplateCreateFromIssue(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/rest/api/3/issue/") {
			fmt.Fprintln(w, `{
				"key": "PROJ-123",
				"fields": {
					"summary": "Login broken",
					"issuetype": {"name": "Bug"},
					"description": {"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"text":"Desc","type":"text"}]}]},
					"priority": {"name": "High"},
					"labels": ["bug","urgent"]
				}
			}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer ts.Close()

	// Redirect user config dir to temp dir via HOME env var.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	output := captureStdout(t, func() {
		ctx := client.NewContext(t.Context(), c)
		cmd := &cobra.Command{Use: "create"}
		cmd.Flags().String("from", "", "")
		_ = cmd.Flags().Set("from", "PROJ-123")
		cmd.SetContext(ctx)

		err := runTemplateCreate(cmd, []string{"from-issue"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "created") {
		t.Errorf("expected output to contain 'created', got: %s", output)
	}
	if !strings.Contains(output, "PROJ-123") {
		t.Errorf("expected output to contain 'PROJ-123', got: %s", output)
	}
}

func TestRunTemplateList_JQFilter(t *testing.T) {
	output := captureStdout(t, func() {
		cmd := &cobra.Command{Use: "list"}
		cmd.Flags().String("jq", "", "")
		cmd.Flags().Bool("pretty", false, "")
		_ = cmd.Flags().Set("jq", ".[0].name")

		err := runTemplateList(cmd, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Should be a single string value (the first template name).
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		t.Fatal("expected non-empty jq-filtered output")
	}
}

func TestRunTemplateList_Pretty(t *testing.T) {
	output := captureStdout(t, func() {
		cmd := &cobra.Command{Use: "list"}
		cmd.Flags().String("jq", "", "")
		cmd.Flags().Bool("pretty", false, "")
		_ = cmd.Flags().Set("pretty", "true")

		err := runTemplateList(cmd, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Pretty output should have indentation.
	if !strings.Contains(output, "\n") || !strings.Contains(output, "  ") {
		t.Errorf("expected pretty-printed output, got: %s", output)
	}
}

func TestRunTemplateShow_JQFilter(t *testing.T) {
	output := captureStdout(t, func() {
		cmd := &cobra.Command{Use: "show"}
		cmd.Flags().String("jq", "", "")
		cmd.Flags().Bool("pretty", false, "")
		_ = cmd.Flags().Set("jq", ".name")

		err := runTemplateShow(cmd, []string{"bug-report"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	trimmed := strings.TrimSpace(output)
	if trimmed != `"bug-report"` {
		t.Errorf("expected jq-filtered name, got: %s", trimmed)
	}
}

func TestRunTemplateShow_Pretty(t *testing.T) {
	output := captureStdout(t, func() {
		cmd := &cobra.Command{Use: "show"}
		cmd.Flags().String("jq", "", "")
		cmd.Flags().Bool("pretty", false, "")
		_ = cmd.Flags().Set("pretty", "true")

		err := runTemplateShow(cmd, []string{"bug-report"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "\n") || !strings.Contains(output, "  ") {
		t.Errorf("expected pretty-printed output, got: %s", output)
	}
}

func TestRunTemplateApplyWithAssignee(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/myself" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"accountId":"me123"}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/rest/api/3/issue") {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			fields, _ := body["fields"].(map[string]any)
			if fields["assignee"] == nil {
				t.Error("expected assignee field in create body")
			}
			fmt.Fprintln(w, `{"id":"10002","key":"PROJ-2","self":"https://example.atlassian.net/rest/api/3/issue/10002"}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "apply"}
	cmd.Flags().String("project", "", "")
	cmd.Flags().StringArray("var", nil, "")
	cmd.Flags().String("assign", "", "")
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("var", "summary=Test with assignee")
	_ = cmd.Flags().Set("assign", "me")
	cmd.SetContext(ctx)

	err := runTemplateApply(cmd, []string{"bug-report"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "PROJ-2") {
		t.Errorf("expected PROJ-2 in output, got: %s", stdout.String())
	}
}

func TestRunTemplateApplyNotFound(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("https://example.atlassian.net", &stdout, &stderr)

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "apply"}
	cmd.Flags().String("project", "", "")
	cmd.Flags().StringArray("var", nil, "")
	cmd.Flags().String("assign", "", "")
	_ = cmd.Flags().Set("project", "PROJ")
	cmd.SetContext(ctx)

	err := runTemplateApply(cmd, []string{"nonexistent-xyz"})

	w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
	if !strings.Contains(stderrBuf.String(), "not_found") {
		t.Errorf("expected not_found error, got: %s", stderrBuf.String())
	}
}

func TestRunTemplateCreateDryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("https://example.atlassian.net", &stdout, &stderr)
	c.DryRun = true

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("from", "", "")
	cmd.Flags().Bool("overwrite", false, "")
	_ = cmd.Flags().Set("from", "PROJ-123")
	cmd.SetContext(ctx)

	err := runTemplateCreate(cmd, []string{"test-dry"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output not valid JSON: %s", stdout.String())
	}
	if result["method"] != "GET" {
		t.Errorf("expected method=GET for dry-run, got %v", result["method"])
	}
}

func TestRunTemplateCreateInvalidName(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("from", "", "")
	cmd.Flags().Bool("overwrite", false, "")

	err := runTemplateCreate(cmd, []string{"invalid name with spaces"})

	w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error for invalid template name")
	}
	if !strings.Contains(stderrBuf.String(), "validation_error") {
		t.Errorf("expected validation_error, got: %s", stderrBuf.String())
	}
}

func TestRunTemplateCreateOverwriteExisting(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create first.
	output1 := captureStdout(t, func() {
		cmd := &cobra.Command{Use: "create"}
		cmd.Flags().String("from", "", "")
		cmd.Flags().Bool("overwrite", false, "")
		_ = cmd.Flags().Set("overwrite", "false")

		err := runTemplateCreate(cmd, []string{"dup-test"})
		if err != nil {
			t.Fatalf("first create unexpected error: %v", err)
		}
	})
	if !strings.Contains(output1, "created") {
		t.Fatalf("first create should succeed, got: %s", output1)
	}

	// Second without overwrite should fail.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("from", "", "")
	cmd.Flags().Bool("overwrite", false, "")
	_ = cmd.Flags().Set("overwrite", "false")

	err := runTemplateCreate(cmd, []string{"dup-test"})

	w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error for duplicate template without overwrite")
	}
	if !strings.Contains(stderrBuf.String(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %s", stderrBuf.String())
	}

	// Third with overwrite should succeed.
	output3 := captureStdout(t, func() {
		cmd := &cobra.Command{Use: "create"}
		cmd.Flags().String("from", "", "")
		cmd.Flags().Bool("overwrite", false, "")
		_ = cmd.Flags().Set("overwrite", "true")

		err := runTemplateCreate(cmd, []string{"dup-test"})
		if err != nil {
			t.Fatalf("overwrite create unexpected error: %v", err)
		}
	})
	if !strings.Contains(output3, "created") {
		t.Errorf("overwrite create should succeed, got: %s", output3)
	}
}

func TestTemplateFromIssue_WithParent(t *testing.T) {
	issueJSON := `{
		"key": "PROJ-1",
		"fields": {
			"summary": "Sub task",
			"issuetype": {"name": "Sub-task"},
			"description": null,
			"priority": {"name": ""},
			"labels": [],
			"parent": {"key": "PROJ-100"}
		}
	}`

	tmpl, err := templateFromIssue("parent-test", []byte(issueJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tmpl.IssueType != "Sub-task" {
		t.Errorf("expected issuetype Sub-task, got %s", tmpl.IssueType)
	}
	// Should have a parent variable.
	hasParent := false
	for _, v := range tmpl.Variables {
		if v.Name == "parent" {
			hasParent = true
		}
	}
	if !hasParent {
		t.Error("expected 'parent' variable for issue with parent key")
	}
	if tmpl.Fields["parent"] != "{{.parent}}" {
		t.Errorf("expected parent field template, got %s", tmpl.Fields["parent"])
	}
}

func TestTemplateFromIssue_InvalidJSON(t *testing.T) {
	_, err := templateFromIssue("bad", []byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseVars(t *testing.T) {
	tests := []struct {
		input   []string
		want    map[string]string
		wantErr bool
	}{
		{
			input: []string{"summary=Bug title", "severity=High"},
			want:  map[string]string{"summary": "Bug title", "severity": "High"},
		},
		{
			input: []string{"description=has=equals"},
			want:  map[string]string{"description": "has=equals"},
		},
		{
			input:   []string{"noequals"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		got, err := parseVars(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseVars(%v) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseVars(%v) unexpected error: %v", tt.input, err)
			continue
		}
		for k, v := range tt.want {
			if got[k] != v {
				t.Errorf("parseVars(%v)[%s] = %q, want %q", tt.input, k, got[k], v)
			}
		}
	}
}

func TestRunTemplateList_InvalidJQFilter(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")
	_ = cmd.Flags().Set("jq", "[invalid")

	err := runTemplateList(cmd, nil)

	w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error for invalid jq filter")
	}
	if !strings.Contains(stderrBuf.String(), "jq_error") {
		t.Errorf("expected jq_error in stderr, got: %s", stderrBuf.String())
	}
}

func TestRunTemplateShow_InvalidJQFilter(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	cmd := &cobra.Command{Use: "show"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")
	_ = cmd.Flags().Set("jq", "[invalid")

	err := runTemplateShow(cmd, []string{"bug-report"})

	w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error for invalid jq filter")
	}
	if !strings.Contains(stderrBuf.String(), "jq_error") {
		t.Errorf("expected jq_error in stderr, got: %s", stderrBuf.String())
	}
}

func TestRunTemplateApply_InvalidVarFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("https://example.atlassian.net", &stdout, &stderr)

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "apply"}
	cmd.Flags().String("project", "", "")
	cmd.Flags().StringArray("var", nil, "")
	cmd.Flags().String("assign", "", "")
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("var", "noequals")
	cmd.SetContext(ctx)

	err := runTemplateApply(cmd, []string{"bug-report"})

	w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error for invalid --var format")
	}
	if !strings.Contains(stderrBuf.String(), "validation_error") {
		t.Errorf("expected validation_error in stderr, got: %s", stderrBuf.String())
	}
}

func TestRunTemplateApply_PostFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/rest/api/3/issue") {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, `{"errorMessages":["internal server error"],"errors":{}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "apply"}
	cmd.Flags().String("project", "", "")
	cmd.Flags().StringArray("var", nil, "")
	cmd.Flags().String("assign", "", "")
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("var", "summary=Login broken")
	cmd.SetContext(ctx)

	err := runTemplateApply(cmd, []string{"bug-report"})
	if err == nil {
		t.Fatal("expected error when POST /rest/api/3/issue returns 500")
	}
}

func TestRunTemplateCreate_FromGetFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"errorMessages":["internal server error"],"errors":{}}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("from", "", "")
	cmd.Flags().Bool("overwrite", false, "")
	_ = cmd.Flags().Set("from", "PROJ-999")
	cmd.SetContext(ctx)

	err := runTemplateCreate(cmd, []string{"fail-template"})
	if err == nil {
		t.Fatal("expected error when GET issue returns 500")
	}
}

func TestRunTemplateCreate_FromInvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/rest/api/3/issue/") {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "not json")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("from", "", "")
	cmd.Flags().Bool("overwrite", false, "")
	_ = cmd.Flags().Set("from", "PROJ-123")
	cmd.SetContext(ctx)

	err := runTemplateCreate(cmd, []string{"bad-json-template"})

	w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error when issue response is not valid JSON")
	}
	if !strings.Contains(stderrBuf.String(), "server_error") {
		t.Errorf("expected server_error in stderr, got: %s", stderrBuf.String())
	}
}

// ---------------------------------------------------------------------------
// runTemplateApply — no client in context
// ---------------------------------------------------------------------------

func TestRunTemplateApply_NoClient(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	cmd := &cobra.Command{Use: "apply"}
	cmd.Flags().String("project", "", "")
	cmd.Flags().StringArray("var", nil, "")
	cmd.Flags().String("assign", "", "")
	_ = cmd.Flags().Set("project", "PROJ")
	// Context without a client.
	cmd.SetContext(t.Context())

	err := runTemplateApply(cmd, []string{"bug-report"})

	w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error when no client in context")
	}
	if !strings.Contains(stderrBuf.String(), "config_error") {
		t.Errorf("expected config_error in stderr, got: %s", stderrBuf.String())
	}
}

// ---------------------------------------------------------------------------
// runTemplateShow — config_error from tmpl.Lookup error
// Override userTemplatesDir to a file path so os.ReadDir fails.
// ---------------------------------------------------------------------------

func TestRunTemplateShow_LookupConfigError(t *testing.T) {
	// Create a temp dir and place a regular file where the templates dir is
	// expected, so that os.ReadDir returns a non-ErrNotExist error.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// On macOS UserConfigDir returns $HOME/Library/Application Support.
	// On Linux it returns $HOME/.config. Create a file at both possible paths
	// so the test is cross-platform.
	for _, rel := range []string{
		"Library/Application Support/jr/templates",
		".config/jr/templates",
	} {
		path := tmpHome + "/" + rel
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("failed to create parent dirs: %v", err)
		}
		// Write a regular file where a directory is expected.
		if err := os.WriteFile(path, []byte("not a directory"), 0o644); err != nil {
			t.Fatalf("failed to create file at templates path: %v", err)
		}
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	cmd := &cobra.Command{Use: "show"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")

	err := runTemplateShow(cmd, []string{"bug-report"})

	w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error when template directory is unreadable")
	}
	// Should be config_error or not_found — either indicates Lookup failed.
	gotStderr := stderrBuf.String()
	if !strings.Contains(gotStderr, "config_error") && !strings.Contains(gotStderr, "not_found") {
		t.Errorf("expected config_error or not_found in stderr, got: %s", gotStderr)
	}
}

// ---------------------------------------------------------------------------
// runTemplateCreate — no client in context when --from is set
// ---------------------------------------------------------------------------

func TestRunTemplateCreate_NoClientWithFrom(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("from", "", "")
	cmd.Flags().Bool("overwrite", false, "")
	_ = cmd.Flags().Set("from", "PROJ-123")
	// Context without a client.
	cmd.SetContext(t.Context())

	err := runTemplateCreate(cmd, []string{"my-template"})

	w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error when no client in context with --from flag")
	}
	if !strings.Contains(stderrBuf.String(), "config_error") {
		t.Errorf("expected config_error in stderr, got: %s", stderrBuf.String())
	}
}

// ---------------------------------------------------------------------------
// runTemplateList — List() returns error (templates dir is a file, not a dir)
// ---------------------------------------------------------------------------

func TestRunTemplateList_ListError(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	for _, rel := range []string{
		"Library/Application Support/jr/templates",
		".config/jr/templates",
	} {
		path := tmpHome + "/" + rel
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("failed to create parent dirs: %v", err)
		}
		if err := os.WriteFile(path, []byte("not a dir"), 0o644); err != nil {
			t.Fatalf("failed to create file at templates path: %v", err)
		}
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")

	err := runTemplateList(cmd, nil)

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error when templates dir is unreadable")
	}
	if !strings.Contains(buf.String(), "config_error") {
		t.Errorf("expected config_error in stderr, got: %s", buf.String())
	}
}

// ---------------------------------------------------------------------------
// runTemplateApply — Lookup returns error (templates dir is a file, not a dir)
// ---------------------------------------------------------------------------

func TestRunTemplateApply_LookupError(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	for _, rel := range []string{
		"Library/Application Support/jr/templates",
		".config/jr/templates",
	} {
		path := tmpHome + "/" + rel
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("failed to create parent dirs: %v", err)
		}
		if err := os.WriteFile(path, []byte("not a dir"), 0o644); err != nil {
			t.Fatalf("failed to create file at templates path: %v", err)
		}
	}

	var stdout, stderr bytes.Buffer
	c := newTestClient("https://example.com", &stdout, &stderr)

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "apply"}
	cmd.Flags().String("project", "", "")
	cmd.Flags().StringArray("var", nil, "")
	cmd.Flags().String("assign", "", "")
	_ = cmd.Flags().Set("project", "PROJ")
	cmd.SetContext(ctx)

	err := runTemplateApply(cmd, []string{"bug-report"})

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error when templates dir is unreadable")
	}
	gotStderr := buf.String()
	if !strings.Contains(gotStderr, "config_error") && !strings.Contains(gotStderr, "not_found") {
		t.Errorf("expected config_error or not_found in stderr, got: %s", gotStderr)
	}
}

func TestRunTemplateCreate_FromDuplicateWithoutOverwrite(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/rest/api/3/issue/") {
			fmt.Fprintln(w, `{
				"key": "PROJ-42",
				"fields": {
					"summary": "Some issue",
					"issuetype": {"name": "Task"},
					"description": null,
					"priority": {"name": ""},
					"labels": []
				}
			}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	createFromIssue := func() error {
		var stdout, stderr bytes.Buffer
		c := newTestClient(ts.URL, &stdout, &stderr)
		ctx := client.NewContext(t.Context(), c)
		cmd := &cobra.Command{Use: "create"}
		cmd.Flags().String("from", "", "")
		cmd.Flags().Bool("overwrite", false, "")
		_ = cmd.Flags().Set("from", "PROJ-42")
		cmd.SetContext(ctx)
		return runTemplateCreate(cmd, []string{"dup-from-issue"})
	}

	// First creation should succeed.
	if err := createFromIssue(); err != nil {
		t.Fatalf("first create from issue unexpected error: %v", err)
	}

	// Second creation without overwrite should fail with conflict.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := createFromIssue()

	w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error for duplicate template without overwrite")
	}
	if !strings.Contains(stderrBuf.String(), "already exists") {
		t.Errorf("expected 'already exists' in stderr, got: %s", stderrBuf.String())
	}
}

// ---------------------------------------------------------------------------
// runTemplateApply — DryRun + WriteOutput failure (invalid JQ filter, lines 248-250)
// ---------------------------------------------------------------------------

func TestRunTemplateApply_DryRunWriteOutputFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("https://example.atlassian.net", &stdout, &stderr)
	c.DryRun = true
	c.JQFilter = ".[[[invalid"

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "apply"}
	cmd.Flags().String("project", "", "")
	cmd.Flags().StringArray("var", nil, "")
	cmd.Flags().String("assign", "", "")
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("var", "summary=Test")
	cmd.SetContext(ctx)

	err := runTemplateApply(cmd, []string{"bug-report"})
	if err == nil {
		t.Fatal("expected error when dry-run WriteOutput fails due to invalid JQ filter")
	}
}

// ---------------------------------------------------------------------------
// runTemplateApply — assignee resolved from vars (not --assign flag, lines 255-258)
// ---------------------------------------------------------------------------

func TestRunTemplateApply_AssignFromVars(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/myself" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"accountId":"me123"}`)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/rest/api/3/issue") {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			fields, _ := body["fields"].(map[string]any)
			if fields["assignee"] == nil {
				t.Error("expected assignee field when assignee var is set")
			}
			fmt.Fprintln(w, `{"id":"10003","key":"PROJ-3"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "apply"}
	cmd.Flags().String("project", "", "")
	cmd.Flags().StringArray("var", nil, "")
	cmd.Flags().String("assign", "", "")
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("var", "summary=Test")
	_ = cmd.Flags().Set("var", "assignee=me")
	// Note: --assign flag is NOT set; assignee comes from the "assignee" var.
	cmd.SetContext(ctx)

	err := runTemplateApply(cmd, []string{"bug-report"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "PROJ-3") {
		t.Errorf("expected PROJ-3 in output, got: %s", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// runTemplateApply — resolveAssignee error when server returns 500 (lines 260-264)
// ---------------------------------------------------------------------------

// TestRunTemplateApply_AllFieldTypes exercises description, priority, labels,
// and parent field rendering in runTemplateApply.
func TestRunTemplateApply_AllFieldTypes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/rest/api/3/issue") {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			fields, _ := body["fields"].(map[string]any)

			// description should be ADF (has "type":"doc")
			desc, _ := fields["description"].(map[string]any)
			if desc == nil || desc["type"] != "doc" {
				t.Error("expected ADF description field")
			}
			// priority should be {name: ...}
			pri, _ := fields["priority"].(map[string]any)
			if pri == nil || pri["name"] != "High" {
				t.Errorf("expected priority.name=High, got %v", pri)
			}
			// labels should be an array
			labels, _ := fields["labels"].([]any)
			if len(labels) != 2 {
				t.Errorf("expected 2 labels, got %v", labels)
			}
			// parent should be {key: ...}
			parent, _ := fields["parent"].(map[string]any)
			if parent == nil || parent["key"] != "PROJ-100" {
				t.Errorf("expected parent.key=PROJ-100, got %v", parent)
			}

			fmt.Fprintln(w, `{"id":"10010","key":"PROJ-10"}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer ts.Close()

	// Use subtask template but also add labels via a user template with all fields.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create a user template that has all field types.
	configDir, _ := os.UserConfigDir()
	tmplDir := filepath.Join(configDir, "jr", "templates")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatalf("setup MkdirAll: %v", err)
	}
	tmplContent := `name: all-fields
issuetype: Sub-task
description: Template with all field types
variables:
  - name: summary
    required: true
  - name: description
  - name: severity
  - name: labels
  - name: parent
    required: true
fields:
  summary: "{{.summary}}"
  description: "{{.description}}"
  priority: "{{.severity}}"
  labels: "{{.labels}}"
`
	os.WriteFile(filepath.Join(tmplDir, "all-fields.yaml"), []byte(tmplContent), 0o644)

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "apply"}
	cmd.Flags().String("project", "", "")
	cmd.Flags().StringArray("var", nil, "")
	cmd.Flags().String("assign", "", "")
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("var", "summary=Test all fields")
	_ = cmd.Flags().Set("var", "description=Detailed desc")
	_ = cmd.Flags().Set("var", "severity=High")
	_ = cmd.Flags().Set("var", "labels=bug,urgent")
	_ = cmd.Flags().Set("var", "parent=PROJ-100")
	cmd.SetContext(ctx)

	err := runTemplateApply(cmd, []string{"all-fields"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "PROJ-10") {
		t.Errorf("expected PROJ-10 in output, got: %s", stdout.String())
	}
}

func TestRunTemplateApply_AssignResolveError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"errorMessages":["internal error"],"errors":{}}`)
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(ts.URL, &stdout, &stderr)

	ctx := client.NewContext(t.Context(), c)
	cmd := &cobra.Command{Use: "apply"}
	cmd.Flags().String("project", "", "")
	cmd.Flags().StringArray("var", nil, "")
	cmd.Flags().String("assign", "", "")
	_ = cmd.Flags().Set("project", "PROJ")
	_ = cmd.Flags().Set("var", "summary=Test")
	_ = cmd.Flags().Set("assign", "me")
	cmd.SetContext(ctx)

	err := runTemplateApply(cmd, []string{"bug-report"})
	if err == nil {
		t.Fatal("expected error when resolveAssignee fails (server 500)")
	}
}
