package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/avatar"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

// newAvatarStatusCmd creates a standalone avatar status command for testing.
func newAvatarStatusCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "status", RunE: runAvatarStatus}
	cmd.SetContext(context.Background())
	return cmd
}

// newAvatarShowCmd creates a standalone avatar show command for testing.
func newAvatarShowCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "show", RunE: runAvatarShow}
	cmd.SetContext(context.Background())
	return cmd
}

// setTempAvatarBase points resolveAvatarDirFromDisk at a temp directory via
// the JR_AVATAR_BASE env var.
func setTempAvatarBase(t *testing.T, base string) {
	t.Helper()
	orig := os.Getenv("JR_AVATAR_BASE")
	t.Cleanup(func() { os.Setenv("JR_AVATAR_BASE", orig) })
	os.Setenv("JR_AVATAR_BASE", base)
}

// TestAvatarSubcommands_RegisteredWithMerge verifies that the hand-written
// avatar subcommands (extract, build, etc.) are accessible after merging with
// the generated avatar command. This was a bug where the generated avatarCmd
// from the OpenAPI spec shadowed the hand-written one, making custom
// subcommands like "extract" unreachable.
func TestAvatarSubcommands_RegisteredWithMerge(t *testing.T) {
	// The root command is initialized via init(), so avatarCmd should already
	// have both hand-written and generated subcommands merged.
	wantSubs := []string{"extract", "build", "prompt", "show", "edit", "refresh", "status"}
	gotSubs := make(map[string]bool)
	for _, sub := range avatarCmd.Commands() {
		gotSubs[sub.Name()] = true
	}
	for _, name := range wantSubs {
		if !gotSubs[name] {
			t.Errorf("avatar subcommand %q not found — mergeCommand may not be wiring custom subcommands", name)
		}
	}

	// Also verify the generated subcommand is preserved.
	if !gotSubs["get-all-system"] {
		t.Error("generated subcommand \"get-all-system\" should be preserved after merge")
	}
}

// TestAvatarStatusCmd_NoProfile verifies that `jr avatar status` outputs
// {"exists":false} when no avatar data is present (empty avatars directory).
func TestAvatarStatusCmd_NoProfile(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)
	// base is empty — no user subdirectories.

	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w

	cmd := newAvatarStatusCmd()
	err := cmd.RunE(cmd, nil)

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := strings.TrimSpace(buf.String())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %q — error: %v", output, jsonErr)
	}

	if exists := result["exists"]; exists != false {
		t.Errorf("expected {\"exists\":false}, got: %s", output)
	}
}

// TestAvatarShowCmd_NoProfile verifies that `jr avatar show` returns a
// not_found error (exit code 3) when no profile exists.
func TestAvatarShowCmd_NoProfile(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	var stderr bytes.Buffer

	cmd := newAvatarShowCmd()
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error but got nil")
	}

	awe, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if awe.Code != jrerrors.ExitNotFound {
		t.Errorf("expected exit code %d (not_found), got %d", jrerrors.ExitNotFound, awe.Code)
	}
}

// TestAvatarStatusCmd_WithProfile verifies that status returns profile info
// when a valid profile.yaml exists.
func TestAvatarStatusCmd_WithProfile(t *testing.T) {
	base := t.TempDir()
	setTempAvatarBase(t, base)

	avatarDir := filepath.Join(base, "abc123def456789a")
	if err := os.MkdirAll(avatarDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	p := &avatar.Profile{
		Version:     "1",
		User:        "account:123",
		DisplayName: "Test User",
		GeneratedAt: "2026-01-01T00:00:00Z",
		Engine:      "local",
	}
	if err := avatar.SaveProfile(avatarDir, p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w

	cmd := newAvatarStatusCmd()
	err := cmd.RunE(cmd, nil)

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := strings.TrimSpace(buf.String())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %q — error: %v", output, jsonErr)
	}

	if exists := result["exists"]; exists != true {
		t.Errorf("expected exists=true, got: %s", output)
	}
	if user := result["user"]; user != "account:123" {
		t.Errorf("expected user=account:123, got: %v", user)
	}
	if engine := result["engine"]; engine != "local" {
		t.Errorf("expected engine=local, got: %v", engine)
	}
}
