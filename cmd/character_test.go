package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/character"
	"github.com/spf13/cobra"
)

// setTempCharacterBase redirects the character package to a temp directory
// via the JR_CHARACTER_BASE env var.
func setTempCharacterBase(t *testing.T, base string) {
	t.Helper()
	orig := os.Getenv("JR_CHARACTER_BASE")
	t.Cleanup(func() { os.Setenv("JR_CHARACTER_BASE", orig) })
	os.Setenv("JR_CHARACTER_BASE", base)
}

// newCharacterListCmd creates a standalone list command for testing.
func newCharacterListCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "list", RunE: runCharacterList}
	cmd.SetContext(context.Background())
	return cmd
}

func newCharacterShowCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "show", Args: cobra.ExactArgs(1), RunE: runCharacterShow}
	cmd.SetContext(context.Background())
	return cmd
}

func newCharacterCreateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "create", Args: cobra.ExactArgs(1), RunE: runCharacterCreate}
	cmd.Flags().String("template", "", "template name")
	cmd.Flags().String("from-user", "", "from user")
	cmd.Flags().String("persona", "", "persona")
	cmd.SetContext(context.Background())
	return cmd
}

func newCharacterDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "delete", Args: cobra.ExactArgs(1), RunE: runCharacterDelete}
	cmd.SetContext(context.Background())
	return cmd
}

func newCharacterUseCmd(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{Use: "use", Args: cobra.ExactArgs(1), RunE: runCharacterUse}
	cmd.Flags().String("writing", "", "writing override")
	cmd.Flags().String("workflow", "", "workflow override")
	cmd.Flags().String("interaction", "", "interaction override")
	cmd.SetContext(context.Background())
	if root != nil {
		root.AddCommand(cmd)
	}
	return cmd
}

func newCharacterPromptCmd(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{Use: "prompt", RunE: runCharacterPrompt}
	cmd.Flags().String("format", "prose", "format")
	cmd.Flags().StringSlice("section", nil, "sections")
	cmd.Flags().Bool("redact", false, "redact")
	cmd.SetContext(context.Background())
	if root != nil {
		root.AddCommand(cmd)
	}
	return cmd
}

// captureCharacterCmd runs a character subcommand and captures stdout/stderr.
func captureCharacterCmd(t *testing.T, cmd *cobra.Command, args []string) (stdout, stderr string, err error) {
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
	sbOut.ReadFrom(r)
	var sbErr bytes.Buffer
	sbErr.ReadFrom(re)

	return sbOut.String(), sbErr.String(), err
}

func TestCharacterList_Empty(t *testing.T) {
	dir := t.TempDir()
	setTempCharacterBase(t, dir)

	cmd := newCharacterListCmd()
	out, _, err := captureCharacterCmd(t, cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var names []string
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &names); jsonErr != nil {
		t.Fatalf("output is not valid JSON array: %v — output: %s", jsonErr, out)
	}
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}
}

func TestCharacterCreate_Scaffold(t *testing.T) {
	dir := t.TempDir()
	setTempCharacterBase(t, dir)

	cmd := newCharacterCreateCmd()
	out, _, err := captureCharacterCmd(t, cmd, []string{"mychar"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v — output: %s", jsonErr, out)
	}
	if result["status"] != "created" {
		t.Errorf("expected status=created, got %q", result["status"])
	}
	if result["name"] != "mychar" {
		t.Errorf("expected name=mychar, got %q", result["name"])
	}

	// Verify the file was actually written.
	if !character.Exists(dir, "mychar") {
		t.Error("character file was not created")
	}
}

func TestCharacterCreate_WithTemplate(t *testing.T) {
	dir := t.TempDir()
	setTempCharacterBase(t, dir)

	cmd := newCharacterCreateCmd()
	if err := cmd.Flags().Set("template", "concise"); err != nil {
		t.Fatal(err)
	}
	out, _, err := captureCharacterCmd(t, cmd, []string{"shortform"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v — output: %s", jsonErr, out)
	}
	if result["status"] != "created" {
		t.Errorf("expected status=created, got %q", result["status"])
	}

	ch, loadErr := character.Load(dir, "shortform")
	if loadErr != nil {
		t.Fatalf("failed to load created character: %v", loadErr)
	}
	if ch.Source != character.SourceTemplate {
		t.Errorf("expected source=template, got %q", ch.Source)
	}
}

func TestCharacterCreate_UnknownTemplate(t *testing.T) {
	dir := t.TempDir()
	setTempCharacterBase(t, dir)

	cmd := newCharacterCreateCmd()
	if err := cmd.Flags().Set("template", "nonexistent"); err != nil {
		t.Fatal(err)
	}
	_, _, err := captureCharacterCmd(t, cmd, []string{"x"})
	if err == nil {
		t.Fatal("expected error for unknown template")
	}
}

func TestCharacterCreate_FromUserStub(t *testing.T) {
	dir := t.TempDir()
	setTempCharacterBase(t, dir)

	cmd := newCharacterCreateCmd()
	if err := cmd.Flags().Set("from-user", "user@example.com"); err != nil {
		t.Fatal(err)
	}
	_, _, err := captureCharacterCmd(t, cmd, []string{"x"})
	if err == nil {
		t.Fatal("expected error: --from-user is not yet implemented")
	}
}

func TestCharacterCreate_PersonaStub(t *testing.T) {
	dir := t.TempDir()
	setTempCharacterBase(t, dir)

	cmd := newCharacterCreateCmd()
	if err := cmd.Flags().Set("persona", "very formal person"); err != nil {
		t.Fatal(err)
	}
	_, _, err := captureCharacterCmd(t, cmd, []string{"x"})
	if err == nil {
		t.Fatal("expected error: --persona is not yet implemented")
	}
}

func TestCharacterShow(t *testing.T) {
	dir := t.TempDir()
	setTempCharacterBase(t, dir)

	// Create a character first.
	ch := &character.Character{
		Version: "1",
		Name:    "testchar",
		Source:  character.SourceManual,
		StyleGuide: character.StyleGuide{
			Writing:     "w",
			Workflow:    "wf",
			Interaction: "i",
		},
	}
	if err := character.Save(dir, ch); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cmd := newCharacterShowCmd()
	out, _, err := captureCharacterCmd(t, cmd, []string{"testchar"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got character.Character
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v — output: %s", jsonErr, out)
	}
	if got.Name != "testchar" {
		t.Errorf("expected name=testchar, got %q", got.Name)
	}
}

func TestCharacterShow_NotFound(t *testing.T) {
	dir := t.TempDir()
	setTempCharacterBase(t, dir)

	cmd := newCharacterShowCmd()
	_, _, err := captureCharacterCmd(t, cmd, []string{"ghost"})
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestCharacterDelete(t *testing.T) {
	dir := t.TempDir()
	setTempCharacterBase(t, dir)

	ch := &character.Character{
		Version: "1",
		Name:    "todelete",
		Source:  character.SourceManual,
		StyleGuide: character.StyleGuide{
			Writing:     "w",
			Workflow:    "wf",
			Interaction: "i",
		},
	}
	if err := character.Save(dir, ch); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cmd := newCharacterDeleteCmd()
	out, _, err := captureCharacterCmd(t, cmd, []string{"todelete"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v — output: %s", jsonErr, out)
	}
	if result["status"] != "deleted" {
		t.Errorf("expected status=deleted, got %q", result["status"])
	}

	// File should be gone.
	if character.Exists(dir, "todelete") {
		t.Error("character file still exists after delete")
	}
}

func TestCharacterDelete_NotFound(t *testing.T) {
	dir := t.TempDir()
	setTempCharacterBase(t, dir)

	cmd := newCharacterDeleteCmd()
	_, _, err := captureCharacterCmd(t, cmd, []string{"nope"})
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestCharacterList_WithEntries(t *testing.T) {
	dir := t.TempDir()
	setTempCharacterBase(t, dir)

	for _, name := range []string{"alpha", "beta"} {
		ch := &character.Character{
			Version: "1",
			Name:    name,
			Source:  character.SourceManual,
			StyleGuide: character.StyleGuide{
				Writing:     "w",
				Workflow:    "wf",
				Interaction: "i",
			},
		}
		if err := character.Save(dir, ch); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	cmd := newCharacterListCmd()
	out, _, err := captureCharacterCmd(t, cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var names []string
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &names); jsonErr != nil {
		t.Fatalf("output is not valid JSON array: %v — output: %s", jsonErr, out)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d: %v", len(names), names)
	}
}

func TestCharacterUse(t *testing.T) {
	dir := t.TempDir()
	setTempCharacterBase(t, dir)

	// Also redirect state file into temp dir.
	stateDir := t.TempDir()
	orig := os.Getenv("JR_CHARACTER_BASE")
	t.Cleanup(func() { os.Setenv("JR_CHARACTER_BASE", orig) })
	os.Setenv("JR_CHARACTER_BASE", dir)

	// Save a character to use.
	ch := &character.Character{
		Version: "1",
		Name:    "mychar",
		Source:  character.SourceManual,
		StyleGuide: character.StyleGuide{
			Writing:     "w",
			Workflow:    "wf",
			Interaction: "i",
		},
	}
	if err := character.Save(dir, ch); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Create a root command that carries the --profile flag.
	root := &cobra.Command{Use: "jr"}
	root.PersistentFlags().String("profile", "", "profile")
	root.SetContext(context.Background())

	useCmd := newCharacterUseCmd(root)

	// Override state file path.
	stateFilePath := filepath.Join(stateDir, "state.json")
	origStateFn := characterStateFile
	_ = origStateFn // we can't monkey-patch it directly, so test via env.
	_ = stateFilePath

	out, _, err := captureCharacterCmd(t, useCmd, []string{"mychar"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v — output: %s", jsonErr, out)
	}
	if result["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", result["status"])
	}
	if result["character"] != "mychar" {
		t.Errorf("expected character=mychar, got %v", result["character"])
	}
}

func TestCharacterPrompt_NoActive(t *testing.T) {
	dir := t.TempDir()
	setTempCharacterBase(t, dir)

	root := &cobra.Command{Use: "jr"}
	root.PersistentFlags().String("profile", "", "profile")
	root.SetContext(context.Background())

	promptCmd := newCharacterPromptCmd(root)

	_, _, err := captureCharacterCmd(t, promptCmd, nil)
	if err == nil {
		t.Fatal("expected not-found error when no character is active")
	}
}

func TestCharacterPrompt_WithActive(t *testing.T) {
	dir := t.TempDir()
	setTempCharacterBase(t, dir)

	// Save a character and set it as active via state file.
	ch := &character.Character{
		Version:     "1",
		Name:        "mychar",
		Source:      character.SourceManual,
		Description: "test",
		StyleGuide: character.StyleGuide{
			Writing:     "Be concise.",
			Workflow:    "Move fast.",
			Interaction: "Be direct.",
		},
	}
	if err := character.Save(dir, ch); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Set as active via state file.
	stateFile := filepath.Join(dir, "state.json")
	if err := character.SaveState(stateFile, "default", "mychar", nil); err != nil {
		t.Fatalf("setup state: %v", err)
	}

	root := &cobra.Command{Use: "jr"}
	root.PersistentFlags().String("profile", "", "profile")
	root.SetContext(context.Background())

	promptCmd := newCharacterPromptCmd(root)

	out, _, err := captureCharacterCmd(t, promptCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "mychar") {
		t.Errorf("expected character name in output, got: %s", out)
	}
}
