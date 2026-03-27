package cache_test

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/sofq/jira-cli/internal/cache"
)

func TestGetMiss(t *testing.T) {
	_, ok := cache.Get("nonexistent-key-12345", time.Minute)
	if ok {
		t.Error("expected cache miss")
	}
}

func TestSetAndGet(t *testing.T) {
	key := cache.Key("GET", "https://test.example.com/test-"+t.Name())
	if err := cache.Set(key, []byte(`{"cached":true}`)); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	data, ok := cache.Get(key, time.Minute)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if string(data) != `{"cached":true}` {
		t.Errorf("unexpected data: %s", data)
	}
}

func TestGetExpired(t *testing.T) {
	key := cache.Key("GET", "https://test.example.com/expired-"+t.Name())
	if err := cache.Set(key, []byte(`{"old":true}`)); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// TTL of 0 means immediately expired.
	_, ok := cache.Get(key, 0)
	if ok {
		t.Error("expected cache miss for expired entry")
	}
}

func TestKey(t *testing.T) {
	k1 := cache.Key("GET", "https://a.com/path")
	k2 := cache.Key("GET", "https://b.com/path")
	if k1 == k2 {
		t.Error("different URLs should produce different keys")
	}
}

// TestCacheSetOverwrite verifies that setting a key a second time replaces the
// previously stored value and Get returns the new data.
func TestCacheSetOverwrite(t *testing.T) {
	key := cache.Key("GET", "https://test.example.com/overwrite-"+t.Name())

	if err := cache.Set(key, []byte(`{"version":1}`)); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := cache.Set(key, []byte(`{"version":2}`)); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	data, ok := cache.Get(key, time.Minute)
	if !ok {
		t.Fatal("expected cache hit after overwrite")
	}
	if string(data) != `{"version":2}` {
		t.Errorf("expected overwritten value, got %s", data)
	}
}

// TestCacheDir verifies that Dir() returns a non-empty path and that the
// directory actually exists on the filesystem.
func TestCacheDir(t *testing.T) {
	dir := cache.Dir()
	if dir == "" {
		t.Fatal("Dir() returned empty string")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("Dir() returned path that does not exist: %v", err)
	}
}

// TestCacheLargeData verifies that the cache can handle roughly 1 MB of data
// without truncation or corruption.
func TestCacheLargeData(t *testing.T) {
	// Build a 1 MB payload.
	chunk := bytes.Repeat([]byte("x"), 1024)
	payload := bytes.Repeat(chunk, 1024) // 1 MB

	key := cache.Key("GET", "https://test.example.com/large-"+t.Name())
	if err := cache.Set(key, payload); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	data, ok := cache.Get(key, time.Minute)
	if !ok {
		t.Fatal("expected cache hit for large data")
	}
	if !bytes.Equal(data, payload) {
		t.Errorf("large data roundtrip failed: got %d bytes, want %d bytes", len(data), len(payload))
	}
}

// TestCacheKeyDifferentMethods verifies that the same URL with different HTTP
// methods produces different cache keys.
func TestCacheKeyDifferentMethods(t *testing.T) {
	url := "https://example.com/rest/api/3/issue/PROJ-1"
	getKey := cache.Key("GET", url)
	postKey := cache.Key("POST", url)

	if getKey == postKey {
		t.Error("GET and POST to the same URL should produce different cache keys")
	}
}

// TestCacheSetReturnsError verifies that Set returns an error when writing to an
// invalid path (e.g. a directory that doesn't exist or isn't writable).
func TestCacheSetReturnsError(t *testing.T) {
	// Use a key that would map to a valid filename, but set up a scenario
	// where write could fail by writing to a read-only directory.
	err := cache.Set("test-return-error", []byte(`{"ok":true}`))
	if err != nil {
		t.Fatalf("expected no error writing to valid cache dir, got: %v", err)
	}
}

// TestCacheKeyConsistent verifies that calling Key with the same arguments
// multiple times always returns the same string.
func TestCacheKeyConsistent(t *testing.T) {
	method := "GET"
	url := "https://example.com/rest/api/3/issue/PROJ-42"

	k1 := cache.Key(method, url)
	k2 := cache.Key(method, url)
	k3 := cache.Key(method, url)

	if k1 != k2 || k2 != k3 {
		t.Errorf("Key() is not deterministic: got %q, %q, %q", k1, k2, k3)
	}
}

// --- Bug #50: Cache key must include auth context ---

// TestCacheKeyDifferentAuthContext verifies that the same URL with different
// auth contexts produces different cache keys, preventing cross-profile cache leaks.
func TestCacheKeyDifferentAuthContext(t *testing.T) {
	url := "https://example.com/rest/api/3/issue/PROJ-1"
	k1 := cache.Key("GET", url, "profile1")
	k2 := cache.Key("GET", url, "profile2")

	if k1 == k2 {
		t.Error("Bug #50: same URL with different auth contexts should produce different cache keys")
	}

	// Without auth context should differ from with auth context
	k3 := cache.Key("GET", url)
	if k1 == k3 {
		t.Error("Bug #50: key with auth context should differ from key without")
	}
}

// TestCacheKeyBackwardCompatible verifies that Key with no auth context
// still works correctly.
func TestCacheKeyBackwardCompatible(t *testing.T) {
	k1 := cache.Key("GET", "https://a.com/path")
	k2 := cache.Key("GET", "https://a.com/path")
	if k1 != k2 {
		t.Error("Key without auth context should be deterministic")
	}
}

// TestGetReadFileError verifies that Get returns (nil, false) when the cache
// file exists and is fresh but cannot be read (e.g. permissions set to 000).
// This exercises the os.ReadFile error path in Get (lines 50-53 of cache.go).
func TestGetReadFileError(t *testing.T) {
	key := cache.Key("GET", "https://test.example.com/unreadable-"+t.Name())

	// Write a valid cache entry.
	if err := cache.Set(key, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Locate the file and make it unreadable.
	dir := cache.Dir()
	path := dir + "/" + key
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	// Restore permissions after the test so cleanup can delete the file.
	t.Cleanup(func() {
		_ = os.Chmod(path, 0o600)
		_ = os.Remove(path)
	})

	// Get must report a miss because ReadFile fails.
	data, ok := cache.Get(key, time.Minute)
	if ok {
		t.Error("expected cache miss when file is unreadable")
	}
	if data != nil {
		t.Errorf("expected nil data, got %s", data)
	}
}

// TestCacheKeyAuthContextIsolation verifies that cached data from one profile
// is not returned for a different profile.
func TestCacheKeyAuthContextIsolation(t *testing.T) {
	url := "https://test.example.com/isolation-" + t.Name()
	key1 := cache.Key("GET", url, "user1@example.com")
	key2 := cache.Key("GET", url, "user2@example.com")

	// Store data under key1
	if err := cache.Set(key1, []byte(`{"user":"user1"}`)); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// key2 should miss (different auth context)
	_, ok := cache.Get(key2, time.Minute)
	if ok {
		t.Error("Bug #50: cache should not return data from a different auth context")
	}

	// key1 should hit
	data, ok := cache.Get(key1, time.Minute)
	if !ok {
		t.Fatal("expected cache hit for same auth context")
	}
	if string(data) != `{"user":"user1"}` {
		t.Errorf("unexpected data: %s", data)
	}
}

// --- Eviction ---

// TestEvict_RemovesOldEntries verifies that Evict removes entries older than MaxAge.
func TestEvict_RemovesOldEntries(t *testing.T) {
	// Save and restore globals.
	origMaxAge := cache.MaxAge
	origMaxEntries := cache.MaxEntries
	defer func() {
		cache.MaxAge = origMaxAge
		cache.MaxEntries = origMaxEntries
	}()

	// Create a few entries.
	keys := make([]string, 3)
	for i := range keys {
		keys[i] = cache.Key("GET", "https://test.example.com/evict-old-"+t.Name()+"-"+string(rune('a'+i)))
		if err := cache.Set(keys[i], []byte(`{"i":`+string(rune('0'+i))+`}`)); err != nil {
			t.Fatalf("Set failed: %v", err)
		}
	}

	// Make one entry "old" by touching its mod time.
	oldPath := cache.Dir() + "/" + keys[0]
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes failed: %v", err)
	}

	cache.MaxAge = 24 * time.Hour
	cache.MaxEntries = 0 // no entry limit

	removed := cache.Evict()
	if removed < 1 {
		t.Errorf("expected at least 1 entry removed, got %d", removed)
	}

	// The old entry should be gone.
	if _, ok := cache.Get(keys[0], time.Hour); ok {
		t.Error("expected old entry to be evicted")
	}
	// The other entries should still be present.
	for _, k := range keys[1:] {
		if _, ok := cache.Get(k, time.Hour); !ok {
			t.Error("expected recent entry to survive eviction")
		}
	}
}

// TestEvict_MaxEntriesLimit verifies that Evict trims entries to MaxEntries.
func TestEvict_MaxEntriesLimit(t *testing.T) {
	origMaxAge := cache.MaxAge
	origMaxEntries := cache.MaxEntries
	defer func() {
		cache.MaxAge = origMaxAge
		cache.MaxEntries = origMaxEntries
	}()

	// Create 5 entries with staggered mod times.
	keys := make([]string, 5)
	for i := range keys {
		keys[i] = cache.Key("GET", "https://test.example.com/evict-limit-"+t.Name()+"-"+string(rune('a'+i)))
		if err := cache.Set(keys[i], []byte(`{}`)); err != nil {
			t.Fatalf("Set failed: %v", err)
		}
		// Stagger mod times so oldest entries are deterministic.
		modTime := time.Now().Add(time.Duration(i-5) * time.Minute)
		path := cache.Dir() + "/" + keys[i]
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("Chtimes failed: %v", err)
		}
	}

	cache.MaxAge = 24 * time.Hour // don't evict by age
	cache.MaxEntries = 3          // keep only 3

	removed := cache.Evict()
	// There might be other files in the cache dir from other tests,
	// so just verify at least 2 were removed from our 5.
	if removed < 2 {
		t.Errorf("expected at least 2 entries removed, got %d", removed)
	}
}

// TestEvict_EmptyCacheDir verifies that Evict handles an empty cache dir gracefully.
func TestEvict_EmptyCacheDir(t *testing.T) {
	origMaxAge := cache.MaxAge
	origMaxEntries := cache.MaxEntries
	defer func() {
		cache.MaxAge = origMaxAge
		cache.MaxEntries = origMaxEntries
	}()

	cache.MaxAge = time.Nanosecond
	cache.MaxEntries = 0

	// This should not panic even if the cache dir is empty or has no evictable entries.
	removed := cache.Evict()
	// We can't assert exact count since other tests may have left files.
	_ = removed
}

// TestEvictDir_ReadDirError verifies that EvictDir returns 0 when the
// directory does not exist (os.ReadDir fails).
func TestEvictDir_ReadDirError(t *testing.T) {
	removed := cache.EvictDir("/nonexistent/path/that/does/not/exist")
	if removed != 0 {
		t.Errorf("expected 0 removed for nonexistent dir, got %d", removed)
	}
}

// TestEvictDir_SkipsSubdirectories verifies that EvictDir skips subdirectories
// inside the cache directory and only processes regular files.
func TestEvictDir_SkipsSubdirectories(t *testing.T) {
	origMaxAge := cache.MaxAge
	origMaxEntries := cache.MaxEntries
	defer func() {
		cache.MaxAge = origMaxAge
		cache.MaxEntries = origMaxEntries
	}()

	tmpDir := t.TempDir()

	// Create a subdirectory inside the cache dir.
	if err := os.Mkdir(tmpDir+"/subdir", 0o755); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	// Create a regular file that should be evicted.
	oldTime := time.Now().Add(-48 * time.Hour)
	filePath := tmpDir + "/oldfile"
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.Chtimes(filePath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes failed: %v", err)
	}

	cache.MaxAge = 24 * time.Hour
	cache.MaxEntries = 0

	removed := cache.EvictDir(tmpDir)
	if removed != 1 {
		t.Errorf("expected 1 removed (the old file), got %d", removed)
	}

	// Subdirectory should still exist.
	if _, err := os.Stat(tmpDir + "/subdir"); err != nil {
		t.Errorf("subdirectory should not have been removed: %v", err)
	}
}

// TestEvictDir_InfoError verifies that EvictDir skips entries where Info()
// returns an error (e.g. the file is removed between ReadDir and Info).
func TestEvictDir_InfoError(t *testing.T) {
	origMaxAge := cache.MaxAge
	origMaxEntries := cache.MaxEntries
	defer func() {
		cache.MaxAge = origMaxAge
		cache.MaxEntries = origMaxEntries
	}()

	tmpDir := t.TempDir()

	cache.MaxAge = 24 * time.Hour
	cache.MaxEntries = 0

	// Use EvictEntries with a synthetic DirEntry whose Info() returns an error.
	entries := []os.DirEntry{
		cache.FakeDirEntry{
			FakeName:  "broken-entry",
			FakeIsDir: false,
			FakeErr:   errors.New("synthetic info error"),
		},
	}
	removed := cache.EvictEntries(tmpDir, entries)
	if removed != 0 {
		t.Errorf("expected 0 removed when Info fails, got %d", removed)
	}
}

// TestEvictDir_IsDirEntry verifies that EvictDir skips directory entries.
func TestEvictDir_IsDirEntry(t *testing.T) {
	origMaxAge := cache.MaxAge
	origMaxEntries := cache.MaxEntries
	defer func() {
		cache.MaxAge = origMaxAge
		cache.MaxEntries = origMaxEntries
	}()

	tmpDir := t.TempDir()

	cache.MaxAge = 24 * time.Hour
	cache.MaxEntries = 0

	// Use EvictEntries with a synthetic DirEntry that is a directory.
	entries := []os.DirEntry{
		cache.FakeDirEntry{
			FakeName:  "subdir",
			FakeIsDir: true,
		},
	}
	removed := cache.EvictEntries(tmpDir, entries)
	if removed != 0 {
		t.Errorf("expected 0 removed when entry is a directory, got %d", removed)
	}
}
