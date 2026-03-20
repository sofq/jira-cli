package cmd

import (
	"bytes"
	"net/http"
	"os"
	"testing"

	"github.com/sofq/jira-cli/internal/client"
	"github.com/sofq/jira-cli/internal/config"
)

// captureOutput redirects os.Stdout to a pipe and returns the original stdout,
// the read end of the pipe, and the write end of the pipe. Call restoreOutput
// after the function under test completes to restore os.Stdout and close the
// write end so that the read end returns EOF.
func captureOutput(t *testing.T) (old *os.File, r *os.File, w *os.File) {
	t.Helper()
	old = os.Stdout
	var err error
	r, w, err = os.Pipe()
	if err != nil {
		t.Fatalf("captureOutput: os.Pipe: %v", err)
	}
	os.Stdout = w
	return old, r, w
}

// restoreOutput restores os.Stdout to old and closes the write end of the pipe
// so that readers see EOF.
func restoreOutput(t *testing.T, old *os.File, w *os.File) {
	t.Helper()
	os.Stdout = old
	if err := w.Close(); err != nil {
		t.Fatalf("restoreOutput: close write end: %v", err)
	}
}

// captureStdout captures os.Stdout output during fn execution.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
}

// newTestClient creates a Client wired to a test server.
func newTestClient(serverURL string, stdout, stderr *bytes.Buffer) *client.Client {
	return &client.Client{
		BaseURL:    serverURL,
		Auth:       config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
		HTTPClient: &http.Client{},
		Stdout:     stdout,
		Stderr:     stderr,
		Paginate:   true,
	}
}
