package client

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// redirectCacheDir points the package-level oauth2CacheFilePath helper at a
// temporary directory for the duration of the test.  It restores the original
// behaviour via t.Cleanup.
func redirectCacheDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Monkey-patch oauth2CacheFilePath by replacing its result at call sites.
	// Because oauth2CacheFilePath is a plain function (not a variable), we
	// shadow the test's calls via a package-level variable below.
	oauth2CacheFilePathOverride = filepath.Join(dir, "oauth2_tokens.json")
	t.Cleanup(func() { oauth2CacheFilePathOverride = "" })
	return dir
}

func TestOAuth2Cache_MissOnEmpty(t *testing.T) {
	redirectCacheDir(t)

	_, ok := getToken("nonexistent-key")
	if ok {
		t.Fatal("expected cache miss on empty cache, got hit")
	}
}

func TestOAuth2Cache_HitAfterSet(t *testing.T) {
	redirectCacheDir(t)

	const key = "test-key"
	const token = "access-token-abc"
	const expiresIn = 3600

	if err := setToken(key, token, expiresIn); err != nil {
		t.Fatalf("setToken returned error: %v", err)
	}

	got, ok := getToken(key)
	if !ok {
		t.Fatal("expected cache hit, got miss")
	}
	if got != token {
		t.Fatalf("expected token %q, got %q", token, got)
	}
}

func TestOAuth2Cache_ExpiredEntryReturnsMiss(t *testing.T) {
	redirectCacheDir(t)

	const key = "expiring-key"
	const token = "expiring-token"

	// Write an entry that is already expired by setting expiresIn=1 (which
	// becomes a TTL of 1-60 = clamped to 1 second, then we wait past it).
	// Instead, write the cache file directly with a past ExpiresAt.
	path := oauth2CacheFilePath()
	f := oauth2CacheFile{
		key: {
			Token:     token,
			ExpiresAt: time.Now().Add(-1 * time.Second), // already in the past
		},
	}
	if err := writeCacheFile(path, f); err != nil {
		t.Fatalf("writeCacheFile: %v", err)
	}

	_, ok := getToken(key)
	if ok {
		t.Fatal("expected cache miss for expired entry, got hit")
	}
}

func TestOAuth2Cache_MultipleKeys(t *testing.T) {
	redirectCacheDir(t)

	entries := []struct {
		key   string
		token string
	}{
		{"key-a", "token-a"},
		{"key-b", "token-b"},
		{"key-c", "token-c"},
	}

	for _, e := range entries {
		if err := setToken(e.key, e.token, 3600); err != nil {
			t.Fatalf("setToken(%q): %v", e.key, err)
		}
	}

	for _, e := range entries {
		got, ok := getToken(e.key)
		if !ok {
			t.Errorf("key %q: expected hit, got miss", e.key)
			continue
		}
		if got != e.token {
			t.Errorf("key %q: expected %q, got %q", e.key, e.token, got)
		}
	}
}

func TestOAuth2Cache_DefaultExpiresIn(t *testing.T) {
	redirectCacheDir(t)

	// expiresIn=0 should default to 3600 and produce a valid cached entry.
	if err := setToken("default-key", "tok", 0); err != nil {
		t.Fatalf("setToken with expiresIn=0: %v", err)
	}
	_, ok := getToken("default-key")
	if !ok {
		t.Fatal("expected hit with default expiresIn, got miss")
	}
}

func TestOAuth2CacheKey_Deterministic(t *testing.T) {
	k1 := oauth2CacheKey("https://auth.example.com/token", "client-id")
	k2 := oauth2CacheKey("https://auth.example.com/token", "client-id")
	if k1 != k2 {
		t.Fatalf("oauth2CacheKey is not deterministic: %q vs %q", k1, k2)
	}

	k3 := oauth2CacheKey("https://auth.example.com/token", "other-client")
	if k1 == k3 {
		t.Fatal("different clientIDs should produce different cache keys")
	}
}

func TestOAuth2Cache_AtomicWrite(t *testing.T) {
	redirectCacheDir(t)

	// Ensure the temp file is cleaned up even if rename fails.
	const key = "atomic-key"
	if err := setToken(key, "tok", 3600); err != nil {
		t.Fatalf("setToken: %v", err)
	}

	// The .tmp file should not exist after a successful write.
	tmp := oauth2CacheFilePath() + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("expected .tmp file to be removed after atomic write, stat err: %v", err)
	}
}
