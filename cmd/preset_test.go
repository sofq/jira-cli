package cmd

import (
	"encoding/json"
	"strings"
	"testing"
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
