package cmd

import (
	"os"
	"testing"

	"github.com/sofq/jira-cli/internal/cache"
)

func TestCacheFilePermissions(t *testing.T) {
	key := "test-perms-key"
	data := []byte(`{"test":"data"}`)

	if err := cache.Set(key, data); err != nil {
		t.Fatal(err)
	}

	// Read back the file permissions.
	dir := cache.Dir()
	path := dir + "/" + key
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// Should be 0600 (owner read/write only), not 0644.
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected cache file permission 0600, got %04o", perm)
	}

	// Clean up.
	os.Remove(path)
}
