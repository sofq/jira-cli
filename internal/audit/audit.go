package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is a single audit log record.
type Entry struct {
	Timestamp string `json:"ts,omitempty"`
	Profile   string `json:"profile,omitempty"`
	Operation string `json:"op,omitempty"`
	Method    string `json:"method,omitempty"`
	Path      string `json:"path,omitempty"`
	Status    int    `json:"status"`
	Exit      int    `json:"exit"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// Logger writes audit entries as JSONL to a file.
// A nil *Logger is safe to use — all methods are no-ops.
type Logger struct {
	mu   sync.Mutex
	file *os.File
}

// NewLogger opens (or creates) the audit log file for appending.
// The file is created with 0600 permissions. Parent directories are created
// if they do not exist.
func NewLogger(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &Logger{file: f}, nil
}

// DefaultPath returns the default audit log file path.
// Uses os.UserConfigDir which handles darwin/windows/linux portably.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "jr", "audit.log")
}

// Log writes an entry to the audit log. It sets the timestamp automatically.
// Safe for concurrent use. No-op on nil receiver.
func (l *Logger) Log(entry Entry) {
	if l == nil {
		return
	}
	entry.Timestamp = time.Now().UTC().Format(time.RFC3339)

	// Entry contains only string/int/bool fields — json.Marshal cannot fail.
	data, _ := json.Marshal(entry)
	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	_, _ = l.file.Write(data)
}

// Close closes the underlying file. No-op on nil receiver.
func (l *Logger) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}
