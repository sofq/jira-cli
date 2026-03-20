package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sofq/jira-cli/internal/cache"
)

// oauth2CacheEntry is the JSON structure stored on disk for a single token entry.
type oauth2CacheEntry struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// oauth2CacheFile is the JSON structure for the whole cache file.
type oauth2CacheFile map[string]oauth2CacheEntry

// oauth2CacheMu guards all reads and writes to the OAuth2 token cache file.
// A single mutex is sufficient because jr is a CLI — concurrent processes are
// rare, and the file is small enough that full-file rewrites are safe.
var oauth2CacheMu sync.Mutex

// oauth2CacheFilePathOverride, when non-empty, overrides the default cache
// file path.  It exists solely so tests can redirect writes to a temp dir
// without touching the real user cache.
var oauth2CacheFilePathOverride string

// oauth2CacheFilePath returns the path to the OAuth2 token cache file.
// It reuses the directory returned by cache.Dir() so all jr cache files sit
// alongside one another under ~/.cache/jr/.
func oauth2CacheFilePath() string {
	if oauth2CacheFilePathOverride != "" {
		return oauth2CacheFilePathOverride
	}
	return filepath.Join(cache.Dir(), "oauth2_tokens.json")
}

// oauth2CacheKey computes a stable cache key from a TokenURL and ClientID.
func oauth2CacheKey(tokenURL, clientID string) string {
	h := sha256.Sum256([]byte(tokenURL + "\x00" + clientID))
	return hex.EncodeToString(h[:])
}

// readCacheFile reads and parses the cache file. Returns an empty map on any
// error (missing file, corrupt JSON) so callers always get a usable value.
func readCacheFile(path string) oauth2CacheFile {
	data, err := os.ReadFile(path)
	if err != nil {
		return oauth2CacheFile{}
	}
	var f oauth2CacheFile
	if err := json.Unmarshal(data, &f); err != nil {
		return oauth2CacheFile{}
	}
	return f
}

// writeCacheFile serialises f and atomically replaces the cache file at path.
// Atomic replacement (write to temp + rename) prevents a concurrent reader
// from seeing a partially-written file.
func writeCacheFile(path string, f oauth2CacheFile) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	// Write to a sibling temp file then rename for atomicity.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// getToken returns the cached access token for key if it exists and has not
// expired. The second return value is false on a cache miss or expiry.
func getToken(key string) (string, bool) {
	oauth2CacheMu.Lock()
	defer oauth2CacheMu.Unlock()

	f := readCacheFile(oauth2CacheFilePath())
	entry, ok := f[key]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.ExpiresAt) {
		return "", false
	}
	return entry.Token, true
}

// setToken writes token to the file cache under key with an expiry of
// expiresIn seconds from now. A 60-second buffer is subtracted from
// expiresIn so the token is refreshed before it actually expires on the
// server side.
func setToken(key, token string, expiresIn int) error {
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	ttl := time.Duration(expiresIn-60) * time.Second
	if ttl <= 0 {
		ttl = 1 * time.Second
	}

	oauth2CacheMu.Lock()
	defer oauth2CacheMu.Unlock()

	path := oauth2CacheFilePath()
	f := readCacheFile(path)
	f[key] = oauth2CacheEntry{
		Token:     token,
		ExpiresAt: time.Now().Add(ttl),
	}
	return writeCacheFile(path, f)
}
