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

// TestWriteCacheFile_WriteError verifies that writeCacheFile returns an error
// when the parent directory does not exist.
func TestWriteCacheFile_WriteError(t *testing.T) {
	badPath := filepath.Join(t.TempDir(), "nonexistent", "subdir", "cache.json")
	err := writeCacheFile(badPath, oauth2CacheFile{"k": {Token: "t", ExpiresAt: time.Now().Add(time.Hour)}})
	if err == nil {
		t.Fatal("expected error writing to non-existent dir, got nil")
	}
}

// TestWriteCacheFile_RenameError verifies that writeCacheFile returns an error
// when the tmp file cannot be renamed (destination is a directory).
func TestWriteCacheFile_RenameError(t *testing.T) {
	dir := t.TempDir()
	// Create a directory where the final cache file should go so that rename
	// from the .tmp file to the target path fails (can't rename file over dir).
	targetPath := filepath.Join(dir, "cache.json")
	if err := os.Mkdir(targetPath, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	err := writeCacheFile(targetPath, oauth2CacheFile{"k": {Token: "t", ExpiresAt: time.Now().Add(time.Hour)}})
	if err == nil {
		t.Fatal("expected error when rename target is a directory, got nil")
	}
}

// TestReadCacheFile_CorruptJSON verifies that readCacheFile returns an empty
// map when the cache file contains invalid JSON.
func TestReadCacheFile_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
	if err := os.WriteFile(path, []byte(`{corrupt json!!!`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := readCacheFile(path)
	if len(got) != 0 {
		t.Errorf("expected empty map for corrupt JSON, got %v", got)
	}
}

// TestWriteCacheFile_TmpWriteError verifies that writeCacheFile returns an
// error when tmp.Write fails (read-only file descriptor).
func TestWriteCacheFile_TmpWriteError(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "cache.json")

	orig := openTempFile
	openTempFile = func(dir, pattern string) (*os.File, error) {
		// Create a real temp file for its name, then reopen read-only.
		realTmp, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		tmpName := realTmp.Name()
		realTmp.Close()
		ro, err := os.Open(tmpName)
		if err != nil {
			return nil, err
		}
		return ro, nil
	}
	defer func() { openTempFile = orig }()

	err := writeCacheFile(targetPath, oauth2CacheFile{"k": {Token: "t", ExpiresAt: time.Now().Add(time.Hour)}})
	if err == nil {
		t.Fatal("expected error from Write to read-only fd, got nil")
	}
}

// TestOAuth2Cache_TTLClamping verifies that when expiresIn-60 <= 0, the TTL is
// clamped to 1 second so the token is still written and retrievable immediately.
// With expiresIn=30, ttl = 30-60 = -30 which clamps to 1s.
func TestOAuth2Cache_TTLClamping(t *testing.T) {
	redirectCacheDir(t)

	const key = "clamped-key"
	const token = "short-lived-token"

	// expiresIn=30 → ttl = 30-60 = -30 → clamped to 1s.
	if err := setToken(key, token, 30); err != nil {
		t.Fatalf("setToken with expiresIn=30: %v", err)
	}

	// Token should be retrievable immediately (TTL=1s has not elapsed yet).
	got, ok := getToken(key)
	if !ok {
		t.Fatal("expected cache hit immediately after setToken with clamped TTL, got miss")
	}
	if got != token {
		t.Fatalf("expected token %q, got %q", token, got)
	}

	// Verify the expiry is within a reasonable window: between now and now+2s.
	path := oauth2CacheFilePath()
	f := readCacheFile(path)
	entry, ok := f[key]
	if !ok {
		t.Fatal("expected entry in cache file")
	}
	now := time.Now()
	if entry.ExpiresAt.Before(now) {
		t.Errorf("expected ExpiresAt to be in the future, got %v (now=%v)", entry.ExpiresAt, now)
	}
	if entry.ExpiresAt.After(now.Add(2 * time.Second)) {
		t.Errorf("expected ExpiresAt to be within ~1s from now, got %v (now=%v)", entry.ExpiresAt, now)
	}
}
