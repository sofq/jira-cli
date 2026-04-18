package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestPresetList_OutputsJSON(t *testing.T) {
	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"preset", "list"})
		_ = rootCmd.Execute()
	})

	output = strings.TrimSpace(output)
	if output == "" {
		t.Fatal("expected non-empty output from preset list")
	}

	var entries []struct {
		Name   string `json:"name"`
		Fields string `json:"fields"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal([]byte(output), &entries); err != nil {
		t.Fatalf("expected valid JSON array, got: %s (err: %v)", output, err)
	}

	if len(entries) < 4 {
		t.Errorf("expected at least 4 presets, got %d", len(entries))
	}

	// Check that known presets are present.
	found := make(map[string]bool)
	for _, e := range entries {
		found[e.Name] = true
	}
	for _, name := range []string{"agent", "detail", "triage", "board"} {
		if !found[name] {
			t.Errorf("missing preset %q in output", name)
		}
	}
}

// Exercise the preset list command with --jq and --pretty flags and failure paths.

func TestPresetList_WithJQ(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")
	_ = cmd.Flags().Set("jq", "[.[].name]")

	if err := presetListCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPresetList_WithInvalidJQ(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")
	_ = cmd.Flags().Set("jq", "[.invalid")

	err := presetListCmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected jq error")
	}
}

func TestPresetList_WithPretty(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")
	_ = cmd.Flags().Set("pretty", "true")

	if err := presetListCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPresetList_PropagatesListError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	for _, p := range []string{
		home + "/jr/presets.json",
		home + "/Library/Application Support/jr/presets.json",
	} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("not-json"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")

	err := presetListCmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error from preset.List")
	}
}
