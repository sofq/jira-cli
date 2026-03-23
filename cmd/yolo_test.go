package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newYoloStatusCmd creates a standalone yolo status command for testing.
func newYoloStatusCmd() *cobra.Command {
	root := &cobra.Command{Use: "jr"}
	root.PersistentFlags().String("profile", "", "profile")
	root.SetContext(context.Background())

	cmd := &cobra.Command{Use: "status", RunE: runYoloStatus}
	cmd.SetContext(context.Background())
	root.AddCommand(cmd)
	return cmd
}

// newYoloHistoryCmd creates a standalone yolo history command for testing.
func newYoloHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "history", RunE: runYoloHistory}
	cmd.SetContext(context.Background())
	return cmd
}

// captureYoloCmd runs a yolo subcommand and captures stdout/stderr.
func captureYoloCmd(t *testing.T, cmd *cobra.Command, args []string) (stdout, stderr string, err error) {
	t.Helper()

	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w

	re, rw, _ := os.Pipe()
	origStderr := os.Stderr
	os.Stderr = rw

	err = cmd.RunE(cmd, args)

	w.Close()
	rw.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr

	var sbOut bytes.Buffer
	_, _ = sbOut.ReadFrom(r)
	var sbErr bytes.Buffer
	_, _ = sbErr.ReadFrom(re)

	return sbOut.String(), sbErr.String(), err
}

func TestYoloStatus_DefaultConfig(t *testing.T) {
	// Use a non-existent config path so we always get defaults.
	t.Setenv("JR_CONFIG_PATH", t.TempDir()+"/noconfig.json")

	cmd := newYoloStatusCmd()
	out, _, err := captureYoloCmd(t, cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v — output: %s", jsonErr, out)
	}

	// Default config has enabled=false.
	enabled, ok := result["enabled"].(bool)
	if !ok {
		t.Fatalf("expected 'enabled' bool in response, got %T: %v", result["enabled"], result["enabled"])
	}
	if enabled {
		t.Error("default config should have enabled=false")
	}

	// Should have scope field.
	if _, ok := result["scope"]; !ok {
		t.Error("expected 'scope' field in response")
	}

	// Should have rate object.
	rate, ok := result["rate"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'rate' map in response, got %T", result["rate"])
	}
	if _, ok := rate["per_hour"]; !ok {
		t.Error("expected 'per_hour' in rate object")
	}
	if _, ok := rate["burst"]; !ok {
		t.Error("expected 'burst' in rate object")
	}
	if _, ok := rate["remaining"]; !ok {
		t.Error("expected 'remaining' in rate object")
	}
}

func TestYoloStatus_Fields(t *testing.T) {
	t.Setenv("JR_CONFIG_PATH", t.TempDir()+"/noconfig.json")

	cmd := newYoloStatusCmd()
	out, _, err := captureYoloCmd(t, cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v — output: %s", jsonErr, out)
	}

	requiredFields := []string{"enabled", "scope", "character", "rate"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("expected field %q in yolo status response", field)
		}
	}
}

func TestYoloHistory_EmptyArray(t *testing.T) {
	cmd := newYoloHistoryCmd()
	out, _, err := captureYoloCmd(t, cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "[]" {
		t.Errorf("expected empty JSON array [], got: %s", trimmed)
	}
}

func TestYoloCmd_Registered(t *testing.T) {
	// Verify that the yolo command is registered on the root command.
	var found bool
	for _, c := range rootCmd.Commands() {
		if c.Name() == "yolo" {
			found = true
			break
		}
	}
	if !found {
		t.Error("yolo command not registered on root command")
	}
}

func TestYoloCmd_Subcommands(t *testing.T) {
	var found bool
	for _, c := range rootCmd.Commands() {
		if c.Name() == "yolo" {
			subs := make(map[string]bool)
			for _, sub := range c.Commands() {
				subs[sub.Name()] = true
			}
			if !subs["status"] {
				t.Error("yolo 'status' subcommand not registered")
			}
			if !subs["history"] {
				t.Error("yolo 'history' subcommand not registered")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("yolo command not found on root command")
	}
}

func TestYoloSkipClientCommands(t *testing.T) {
	if !skipClientCommands["yolo"] {
		t.Error("'yolo' should be in skipClientCommands")
	}
}
