package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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
