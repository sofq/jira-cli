package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

// newAvatarActCmd constructs a standalone avatar act command wired to the
// provided client, suitable for unit tests.
func newAvatarActCmd(c *client.Client) *cobra.Command {
	// Build a minimal root so that PersistentFlags().GetString("profile") works.
	root := &cobra.Command{Use: "jr"}
	root.PersistentFlags().String("profile", "", "")

	cmd := &cobra.Command{Use: "act", RunE: runAvatarAct}
	cmd.Flags().String("jql", "", "")
	cmd.Flags().Duration("interval", 30*time.Second, "")
	cmd.Flags().Int("max-actions", 0, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("once", false, "")
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().String("character", "", "")
	cmd.Flags().String("writing", "", "")
	cmd.Flags().StringSlice("react-to", nil, "")

	root.AddCommand(cmd)
	cmd.SetContext(client.NewContext(context.Background(), c))
	return cmd
}

// TestAvatarActCmd_JQLRequired verifies that running `jr avatar act` without
// --jql returns a validation error with exit code ExitValidation.
func TestAvatarActCmd_JQLRequired(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	cmd := newAvatarActCmd(c)
	// Do NOT set --jql.

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when --jql is missing, got nil")
	}

	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected *AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d (ExitValidation), got %d", jrerrors.ExitValidation, awe.Code)
	}
}

// TestAvatarActCmd_DryRun verifies that --dry-run prints a summary JSON and
// exits cleanly without contacting Jira.
func TestAvatarActCmd_DryRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	cmd := newAvatarActCmd(c)
	_ = cmd.Flags().Set("jql", "project = TEST")
	_ = cmd.Flags().Set("dry-run", "true")

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		t.Fatal("expected output on stdout for dry-run, got empty")
	}

	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %q — error: %v", output, jsonErr)
	}

	if result["dry_run"] != true {
		t.Errorf("expected dry_run=true in output, got: %v", result["dry_run"])
	}
	if result["jql"] != "project = TEST" {
		t.Errorf("expected jql field in output, got: %v", result["jql"])
	}
}

// TestAvatarActCmd_InvalidReactTo verifies that an unknown --react-to value
// returns a validation error.
func TestAvatarActCmd_InvalidReactTo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	c := newTestClient("http://unused", &stdout, &stderr)

	cmd := newAvatarActCmd(c)
	_ = cmd.Flags().Set("jql", "project = TEST")
	_ = cmd.Flags().Set("react-to", "not_a_real_event")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid react-to value, got nil")
	}

	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected *AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d (ExitValidation), got %d", jrerrors.ExitValidation, awe.Code)
	}
}

// TestActClassifyWatchEvent verifies the event type classification logic.
func TestActClassifyWatchEvent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"created", "issue_created"},
		{"initial", "issue_created"},
		{"updated", "field_change"},
		{"removed", "field_change"},
		{"unknown", "field_change"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := actClassifyWatchEvent(tc.input)
			if got != tc.want {
				t.Errorf("actClassifyWatchEvent(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestActExtractIssueKey verifies extraction of the issue key from raw JSON.
func TestActExtractIssueKey(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"valid key", `{"key":"PROJ-123","fields":{}}`, "PROJ-123"},
		{"no key field", `{"id":"10001"}`, ""},
		{"empty JSON", `{}`, ""},
		{"nil", ``, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := actExtractIssueKey(json.RawMessage(tc.input))
			if got != tc.want {
				t.Errorf("actExtractIssueKey(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestAvatarActCmd_RegisteredOnAvatarCmd ensures the act subcommand is
// accessible under avatarCmd after init().
func TestAvatarActCmd_RegisteredOnAvatarCmd(t *testing.T) {
	found := false
	for _, sub := range avatarCmd.Commands() {
		if sub.Name() == "act" {
			found = true
			break
		}
	}
	if !found {
		t.Error("avatarCmd does not have an 'act' subcommand — check that avatarCmd.AddCommand(avatarActCmd) was called")
	}
}

// TestAvatarNeedsClient_ActIsSet ensures "act" is present in avatarNeedsClient.
func TestAvatarNeedsClient_ActIsSet(t *testing.T) {
	if !avatarNeedsClient["act"] {
		t.Error("avatarNeedsClient[\"act\"] should be true")
	}
}
