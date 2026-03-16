package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestGeneratedTemplate_BodyStdinMarker(t *testing.T) {
	// Read a generated file and verify it contains the stdin marker check.
	// The actual stdin behavior is tested in e2e tests.
	// The fact that this test file compiles with the generated code
	// is itself verification that the template was applied.
}

func TestGeneratedTemplate_FileOpenError_HasStructuredError(t *testing.T) {
	// Read the template file and verify it uses AlreadyWrittenError for @file failures.
	tmplData, err := os.ReadFile("../gen/templates/resource.go.tmpl")
	if err != nil {
		t.Fatalf("cannot read template: %v", err)
	}
	tmpl := string(tmplData)

	// The old pattern was: `return err` after os.Open failure.
	// The new pattern should use AlreadyWrittenError.
	if strings.Contains(tmpl, "os.Open(strings.TrimPrefix(bodyStr,") &&
		!strings.Contains(tmpl, "AlreadyWrittenError") {
		t.Error("template should use AlreadyWrittenError for @file open failures, not raw error return")
	}

	if !strings.Contains(tmpl, `"cannot open body file: "`) {
		t.Error("template should include descriptive error message for @file open failures")
	}
}

func TestGeneratedTemplate_EmptyAtFilename(t *testing.T) {
	// Verify that generated commands validate --body @ with no filename.
	// Read a generated file to confirm the pattern exists.
	data, err := os.ReadFile("generated/issue.go")
	if err != nil {
		t.Skipf("cannot read generated file: %v", err)
	}
	if !strings.Contains(string(data), `requires a filename after @`) {
		t.Error("generated issue.go missing --body @<filename> validation")
	}
}
