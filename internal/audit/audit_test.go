package audit_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/audit"
)

func TestNewLogger_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	l, err := audit.NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 600", perm)
	}
}

func TestNewLogger_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "audit.log")

	l, err := audit.NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
}

func TestLogger_Log_WritesJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	l, err := audit.NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	l.Log(audit.Entry{
		Profile:   "test",
		Operation: "issue get",
		Method:    "GET",
		Path:      "/rest/api/3/issue/TEST-1",
		Status:    200,
		Exit:      0,
	})
	l.Log(audit.Entry{
		Profile:   "test",
		Operation: "issue delete",
		Method:    "DELETE",
		Path:      "/rest/api/3/issue/TEST-2",
		Status:    204,
		Exit:      0,
		DryRun:    true,
	})
	l.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %s", len(lines), string(data))
	}

	var entry1 audit.Entry
	if err := json.Unmarshal([]byte(lines[0]), &entry1); err != nil {
		t.Fatalf("line 1 not valid JSON: %v", err)
	}
	if entry1.Operation != "issue get" {
		t.Errorf("entry1.Operation = %q, want %q", entry1.Operation, "issue get")
	}
	if entry1.Timestamp == "" {
		t.Error("entry1.Timestamp should be set")
	}

	var entry2 audit.Entry
	if err := json.Unmarshal([]byte(lines[1]), &entry2); err != nil {
		t.Fatalf("line 2 not valid JSON: %v", err)
	}
	if !entry2.DryRun {
		t.Error("entry2.DryRun should be true")
	}
}

func TestLogger_Append(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	l1, _ := audit.NewLogger(path)
	l1.Log(audit.Entry{Operation: "first"})
	l1.Close()

	l2, _ := audit.NewLogger(path)
	l2.Log(audit.Entry{Operation: "second"})
	l2.Close()

	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines after reopen, got %d", len(lines))
	}
}

func TestNilLogger_LogIsNoop(t *testing.T) {
	var l *audit.Logger
	l.Log(audit.Entry{Operation: "test"})
	l.Close()
}

func TestDefaultPath(t *testing.T) {
	p := audit.DefaultPath()
	if p == "" {
		t.Error("DefaultPath() returned empty string")
	}
	if !strings.HasSuffix(p, "audit.log") {
		t.Errorf("DefaultPath() = %q, want suffix 'audit.log'", p)
	}
	if !filepath.IsAbs(p) {
		t.Errorf("DefaultPath() = %q, want absolute path", p)
	}
}

func TestNewLogger_InvalidPath(t *testing.T) {
	// Attempt to create a logger at a path that cannot be created.
	_, err := audit.NewLogger("/dev/null/impossible/audit.log")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestLogger_LogAfterClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	l, err := audit.NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	l.Close()
	// Log after close should be a no-op (no panic).
	l.Log(audit.Entry{Operation: "after-close"})

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "after-close") {
		t.Error("should not write after close")
	}
}

func TestLogger_DoubleClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	l, err := audit.NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	l.Close()
	// Second close should not panic.
	l.Close()
}
