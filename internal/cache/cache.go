package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	cacheDir     string
	cacheDirOnce sync.Once
)

// Dir returns the cache directory, creating it on first call.
func Dir() string {
	cacheDirOnce.Do(func() {
		dir, _ := os.UserCacheDir()
		cacheDir = filepath.Join(dir, "jr")
		_ = os.MkdirAll(cacheDir, 0o700)
	})
	return cacheDir
}

// Key generates a cache key from method + URL + auth context.
// The authContext should include enough information to distinguish requests
// made with different credentials (e.g. profile name or base-url + username).
func Key(method, url string, authContext ...string) string {
	input := method + " " + url
	for _, ctx := range authContext {
		input += "\x00" + ctx
	}
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}

// Get returns cached data if it exists and is not expired.
func Get(key string, ttl time.Duration) ([]byte, bool) {
	path := filepath.Join(Dir(), key)
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if time.Since(info.ModTime()) > ttl {
		os.Remove(path)
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}

// Set writes data to the cache. Returns an error if the write fails.
func Set(key string, data []byte) error {
	path := filepath.Join(Dir(), key)
	return os.WriteFile(path, data, 0o600)
}
