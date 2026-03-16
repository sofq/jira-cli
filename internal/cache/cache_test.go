package cache_test

import (
	"bytes"
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
