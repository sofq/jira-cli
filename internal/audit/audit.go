package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
	Status    int    `json:"status,omitempty"`
	Exit      int    `json:"exit,omitempty"`
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
func DefaultPath() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "jr", "audit.log")
	case "windows":
		appdata := os.Getenv("APPDATA")
		return filepath.Join(appdata, "jr", "audit.log")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "jr", "audit.log")
	}
}

// Log writes an entry to the audit log. It sets the timestamp automatically.
// Safe for concurrent use. No-op on nil receiver.
func (l *Logger) Log(entry Entry) {
	if l == nil {
		return
	}
	entry.Timestamp = time.Now().UTC().Format(time.RFC3339)

	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')
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
	}
}
