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
	cache.Set(key, []byte(`{"cached":true}`))

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
	cache.Set(key, []byte(`{"old":true}`))

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

	cache.Set(key, []byte(`{"version":1}`))
	cache.Set(key, []byte(`{"version":2}`))

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
	cache.Set(key, payload)

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
