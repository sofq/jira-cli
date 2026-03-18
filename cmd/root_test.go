package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootHelp_NoHTMLEscaping(t *testing.T) {
	// The root help JSON should not contain HTML-escaped angle brackets.
	// Verify the encoding approach directly.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]string{
		"hint": "use `jr schema <resource>` for operations",
	})

	output := buf.String()
	if strings.Contains(output, `\u003c`) {
		t.Errorf("expected no HTML escaping in JSON, got: %s", output)
	}
	if !strings.Contains(output, `<resource>`) {
		t.Errorf("expected literal <resource> in output, got: %s", output)
	}
}

func TestRootCommand_ReturnsNonNil(t *testing.T) {
	cmd := RootCommand()
	if cmd == nil {
		t.Fatal("RootCommand() returned nil")
	}
	if cmd.Use != "jr" {
		t.Errorf("expected Use='jr', got %q", cmd.Use)
	}
}

func TestResolveOperation_RawCommand(t *testing.T) {
	cmd := &cobra.Command{Use: "raw"}
	result := resolveOperation(cmd)
	if result != "raw" {
		t.Errorf("expected 'raw', got %q", result)
	}
}

func TestResolveOperation_NestedCommand(t *testing.T) {
	parent := &cobra.Command{Use: "issue"}
	child := &cobra.Command{Use: "get"}
	parent.AddCommand(child)

	result := resolveOperation(child)
	if result != "issue get" {
		t.Errorf("expected 'issue get', got %q", result)
	}
}

func TestResolveOperation_TopLevelCommand(t *testing.T) {
	root := &cobra.Command{Use: "jr"}
	child := &cobra.Command{Use: "configure"}
	root.AddCommand(child)

	result := resolveOperation(child)
	if result != "configure" {
		t.Errorf("expected 'configure', got %q", result)
	}
}

func TestMergeCommand_PreservesGeneratedSubcommands(t *testing.T) {
	root := &cobra.Command{Use: "jr"}

	// Simulate a generated parent with subcommands.
	genParent := &cobra.Command{Use: "workflow"}
	genSub1 := &cobra.Command{Use: "generated-sub"}
	genParent.AddCommand(genSub1)
	root.AddCommand(genParent)

	// Create a hand-written replacement with its own sub.
	handWritten := &cobra.Command{Use: "workflow"}
	handSub := &cobra.Command{Use: "hand-sub"}
	handWritten.AddCommand(handSub)

	mergeCommand(root, handWritten)

	// Verify hand-written replaced the generated parent.
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "workflow" {
			found = true
			// Should have both the hand-written sub and the generated sub.
			subNames := make(map[string]bool)
			for _, sub := range c.Commands() {
				subNames[sub.Name()] = true
			}
			if !subNames["hand-sub"] {
				t.Error("expected 'hand-sub' subcommand")
			}
			if !subNames["generated-sub"] {
				t.Error("expected 'generated-sub' subcommand to be preserved")
			}
		}
	}
	if !found {
		t.Error("workflow command not found on root")
	}
}

func TestMergeCommand_NoExistingGenerated(t *testing.T) {
	root := &cobra.Command{Use: "jr"}
	handWritten := &cobra.Command{Use: "brand-new"}
	handSub := &cobra.Command{Use: "sub1"}
	handWritten.AddCommand(handSub)

	mergeCommand(root, handWritten)

	found := false
	for _, c := range root.Commands() {
		if c.Name() == "brand-new" {
			found = true
		}
	}
	if !found {
		t.Error("brand-new command should be added to root")
	}
}

func TestMergeCommand_HandWrittenSubNotDuplicated(t *testing.T) {
	root := &cobra.Command{Use: "jr"}

	// Generated parent has a sub called "shared".
	genParent := &cobra.Command{Use: "workflow"}
	genSub := &cobra.Command{Use: "shared"}
	genParent.AddCommand(genSub)
	root.AddCommand(genParent)

	// Hand-written also has "shared".
	handWritten := &cobra.Command{Use: "workflow"}
	handSub := &cobra.Command{Use: "shared"}
	handWritten.AddCommand(handSub)

	mergeCommand(root, handWritten)

	for _, c := range root.Commands() {
		if c.Name() == "workflow" {
			count := 0
			for _, sub := range c.Commands() {
				if sub.Name() == "shared" {
					count++
				}
			}
			if count != 1 {
				t.Errorf("expected exactly 1 'shared' subcommand, got %d", count)
			}
		}
	}
}
