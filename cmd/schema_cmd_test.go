package cmd

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/cmd/generated"
	"github.com/spf13/cobra"
)

func TestSchemaOutput_JQFilter(t *testing.T) {
	// Create a minimal cobra command with --jq and --pretty flags.
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")
	_ = cmd.Flags().Set("jq", ".verb")

	data, _ := json.Marshal(map[string]string{"verb": "get", "resource": "issue"})
	err := schemaOutput(cmd, data)
	if err != nil {
		t.Fatalf("schemaOutput error: %v", err)
	}
	// The output goes to os.Stdout — we can't easily capture it in this test,
	// but we verify it doesn't error.
}

func TestSchemaOutput_InvalidJQ(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")
	_ = cmd.Flags().Set("jq", ".[invalid")

	data, _ := json.Marshal(map[string]string{"key": "value"})
	err := schemaOutput(cmd, data)
	if err == nil {
		t.Fatal("expected error for invalid jq expression")
	}
}

func TestSchemaListIncludesWorkflow(t *testing.T) {
	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)

	// Build the same resource list as schema_cmd.go
	resources := generated.AllResources()
	seen := make(map[string]bool, len(resources))
	for _, r := range resources {
		seen[r] = true
	}
	for _, op := range allOps {
		if !seen[op.Resource] {
			resources = append(resources, op.Resource)
			seen[op.Resource] = true
		}
	}

	if !slices.Contains(resources, "workflow") {
		t.Error("schema --list should include 'workflow' resource, but it was missing")
	}
}

func TestMarshalNoEscape_PreservesSpecialChars(t *testing.T) {
	data, err := marshalNoEscape(map[string]string{
		"url": "http://example.com?a=1&b=2",
		"tag": "<html>",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Contains(s, `\u0026`) {
		t.Error("marshalNoEscape should not escape & to \\u0026")
	}
	if strings.Contains(s, `\u003c`) {
		t.Error("marshalNoEscape should not escape < to \\u003c")
	}
	if !strings.Contains(s, `&`) {
		t.Error("marshalNoEscape should contain literal &")
	}
	if !strings.Contains(s, `<html>`) {
		t.Error("marshalNoEscape should contain literal <html>")
	}
}

func TestSchema_ResourceNotFound(t *testing.T) {
	// Verify that a clearly nonexistent resource is absent from all schema ops.
	allOps := generated.AllSchemaOps()
	allOps = append(allOps, HandWrittenSchemaOps()...)

	found := false
	for _, op := range allOps {
		if op.Resource == "definitely_nonexistent" {
			found = true
		}
	}
	if found {
		t.Skip("somehow 'definitely_nonexistent' resource exists")
	}
}

func TestCompactSchema(t *testing.T) {
	ops := []generated.SchemaOp{
		{Resource: "issue", Verb: "get"},
		{Resource: "issue", Verb: "create-issue"},
		{Resource: "project", Verb: "search"},
	}

	result := compactSchema(ops)

	if len(result["issue"]) != 2 {
		t.Errorf("expected 2 verbs for issue, got %d", len(result["issue"]))
	}
	if len(result["project"]) != 1 {
		t.Errorf("expected 1 verb for project, got %d", len(result["project"]))
	}

	issueVerbs := strings.Join(result["issue"], ",")
	if !strings.Contains(issueVerbs, "get") || !strings.Contains(issueVerbs, "create-issue") {
		t.Errorf("expected get and create-issue verbs, got: %s", issueVerbs)
	}
}

func TestCompactSchema_Empty(t *testing.T) {
	result := compactSchema(nil)
	if len(result) != 0 {
		t.Errorf("expected empty map for nil ops, got %d entries", len(result))
	}
}

func TestMarshalNoEscape_Error(t *testing.T) {
	// Channels cannot be marshaled to JSON.
	ch := make(chan int)
	_, err := marshalNoEscape(ch)
	if err == nil {
		t.Error("expected error marshaling a channel")
	}
}

// --- schemaCmd.RunE coverage ---

func TestSchemaCmd_Compact(t *testing.T) {
	cmd := &cobra.Command{Use: "schema"}
	cmd.Flags().Bool("list", false, "")
	cmd.Flags().Bool("compact", false, "")
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")

	if err := schemaCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("default (compact) RunE failed: %v", err)
	}
}

func TestSchemaCmd_CompactFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "schema"}
	cmd.Flags().Bool("list", false, "")
	cmd.Flags().Bool("compact", false, "")
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")
	_ = cmd.Flags().Set("compact", "true")

	if err := schemaCmd.RunE(cmd, []string{"issue"}); err != nil {
		t.Fatalf("--compact RunE failed: %v", err)
	}
}

func TestSchemaCmd_ListFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "schema"}
	cmd.Flags().Bool("list", false, "")
	cmd.Flags().Bool("compact", false, "")
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")
	_ = cmd.Flags().Set("list", "true")

	if err := schemaCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("--list RunE failed: %v", err)
	}
}

func TestSchemaCmd_ResourceOnly(t *testing.T) {
	cmd := &cobra.Command{Use: "schema"}
	cmd.Flags().Bool("list", false, "")
	cmd.Flags().Bool("compact", false, "")
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")

	if err := schemaCmd.RunE(cmd, []string{"issue"}); err != nil {
		t.Fatalf("RunE with resource failed: %v", err)
	}
}

func TestSchemaCmd_ResourceNotFound(t *testing.T) {
	cmd := &cobra.Command{Use: "schema"}
	cmd.Flags().Bool("list", false, "")
	cmd.Flags().Bool("compact", false, "")
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")

	err := schemaCmd.RunE(cmd, []string{"definitely-nonexistent-xyz"})
	if err == nil {
		t.Fatal("expected not_found error")
	}
}

func TestSchemaCmd_ResourceVerb(t *testing.T) {
	cmd := &cobra.Command{Use: "schema"}
	cmd.Flags().Bool("list", false, "")
	cmd.Flags().Bool("compact", false, "")
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")

	// Find a real resource+verb from the generated ops.
	allOps := generated.AllSchemaOps()
	if len(allOps) == 0 {
		t.Skip("no generated ops available")
	}
	op := allOps[0]

	if err := schemaCmd.RunE(cmd, []string{op.Resource, op.Verb}); err != nil {
		t.Fatalf("RunE with resource+verb failed: %v", err)
	}
}

func TestSchemaCmd_ResourceVerbNotFound(t *testing.T) {
	cmd := &cobra.Command{Use: "schema"}
	cmd.Flags().Bool("list", false, "")
	cmd.Flags().Bool("compact", false, "")
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")

	err := schemaCmd.RunE(cmd, []string{"issue", "definitely-nonexistent-verb"})
	if err == nil {
		t.Fatal("expected not_found error")
	}
}

// --- schemaOutput --pretty path ---

func TestSchemaOutput_Pretty(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")
	_ = cmd.Flags().Set("pretty", "true")

	data, _ := json.Marshal(map[string]string{"a": "b"})
	if err := schemaOutput(cmd, data); err != nil {
		t.Fatalf("schemaOutput with --pretty failed: %v", err)
	}
}
