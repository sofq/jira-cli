package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sofq/jira-cli/internal/client"
	"github.com/sofq/jira-cli/internal/watch"
	"github.com/spf13/cobra"
)

func newWatchCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "watch", RunE: runWatch}
	cmd.Flags().String("jql", "", "")
	cmd.Flags().String("issue", "", "")
	cmd.Flags().Duration("interval", 30*time.Second, "")
	cmd.Flags().Int("max-events", 0, "")
	cmd.MarkFlagsMutuallyExclusive("jql", "issue")
	cmd.SetContext(client.NewContext(context.Background(), c))
	return cmd
}

func TestRunWatch_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)
	c.DryRun = true

	cmd := newWatchCmd(c)
	_ = cmd.Flags().Set("jql", "project = TEST")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "POST") {
		t.Errorf("expected POST in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "search/jql") {
		t.Errorf("expected search/jql in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "project = TEST") {
		t.Errorf("expected JQL in dry-run output, got: %s", output)
	}
}

func TestRunWatch_RequiresJQLOrIssue(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	cmd := newWatchCmd(c)
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for missing --jql and --issue")
	}

	if !strings.Contains(stderr.String(), "validation_error") {
		t.Errorf("expected validation_error, got: %s", stderr.String())
	}
}

func TestWatch_InitialPollEmitsEvents(t *testing.T) {
	issues := []map[string]any{
		{"key": "TEST-1", "fields": map[string]any{"summary": "First", "updated": "2026-01-01T00:00:00Z"}},
		{"key": "TEST-2", "fields": map[string]any{"summary": "Second", "updated": "2026-01-01T00:00:00Z"}},
	}
	resp := map[string]any{"issues": issues}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := watch.Options{
		JQL:       "project = TEST",
		Interval:  1 * time.Second,
		MaxEvents: 2,
	}

	exitCode := watch.Run(ctx, c, opts)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d: %s", len(lines), stdout.String())
	}

	for _, line := range lines {
		var event watch.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("invalid JSON event: %s", line)
		}
		if event.Type != "initial" {
			t.Errorf("expected type 'initial', got %q", event.Type)
		}
	}
}

func TestWatch_DetectsUpdates(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var issues []map[string]any
		if callCount == 1 {
			issues = []map[string]any{
				{"key": "TEST-1", "fields": map[string]any{"summary": "First", "updated": "2026-01-01T00:00:00Z"}},
			}
		} else {
			issues = []map[string]any{
				{"key": "TEST-1", "fields": map[string]any{"summary": "Updated", "updated": "2026-01-01T01:00:00Z"}},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"issues": issues})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := watch.Options{
		JQL:       "project = TEST",
		Interval:  50 * time.Millisecond,
		MaxEvents: 2,
	}

	exitCode := watch.Run(ctx, c, opts)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d: %s", len(lines), stdout.String())
	}

	var event1, event2 watch.Event
	if err := json.Unmarshal([]byte(lines[0]), &event1); err != nil {
		t.Fatalf("failed to parse event1: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &event2); err != nil {
		t.Fatalf("failed to parse event2: %v", err)
	}

	if event1.Type != "initial" {
		t.Errorf("expected first event type 'initial', got %q", event1.Type)
	}
	if event2.Type != "updated" {
		t.Errorf("expected second event type 'updated', got %q", event2.Type)
	}
}

func TestWatch_SingleIssueMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		jql, _ := reqBody["jql"].(string)
		if !strings.Contains(jql, "PROJ-123") {
			t.Errorf("expected JQL to contain PROJ-123, got: %s", jql)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issues": []map[string]any{
				{"key": "PROJ-123", "fields": map[string]any{"summary": "Test", "updated": "2026-01-01T00:00:00Z"}},
			},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	c := newTestClient(srv.URL, &stdout, &stderr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := watch.Options{
		Issue:     "PROJ-123",
		Interval:  1 * time.Second,
		MaxEvents: 1,
	}

	exitCode := watch.Run(ctx, c, opts)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 NDJSON line, got %d", len(lines))
	}
}

func TestWatchSchemaOps(t *testing.T) {
	ops := WatchSchemaOps()
	if len(ops) != 1 {
		t.Fatalf("expected 1 watch schema op, got %d", len(ops))
	}
	op := ops[0]
	if op.Resource != "watch" || op.Verb != "watch" {
		t.Errorf("expected resource=watch verb=watch, got %s %s", op.Resource, op.Verb)
	}
}
