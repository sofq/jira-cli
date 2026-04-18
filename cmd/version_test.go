package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestVersionCmd exercises versionCmd.RunE which emits the current Version as JSON.
func TestVersionCmd(t *testing.T) {
	cmd := &cobra.Command{Use: "version"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")

	if err := versionCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("versionCmd.RunE failed: %v", err)
	}
}

// TestVersionCmd_MarshalError forces marshalNoEscape to return an error by
// swapping it with a failing stub to cover the err-handling branch.
func TestVersionCmd_MarshalError(t *testing.T) {
	orig := marshalNoEscape
	defer func() { marshalNoEscape = orig }()
	marshalNoEscape = func(any) ([]byte, error) {
		return nil, errStub
	}

	cmd := &cobra.Command{Use: "version"}
	cmd.Flags().String("jq", "", "")
	cmd.Flags().Bool("pretty", false, "")

	err := versionCmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected marshal error to propagate")
	}
}

var errStub = &stubError{msg: "stub marshal error"}

type stubError struct{ msg string }

func (e *stubError) Error() string { return e.msg }
