package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"
)

// Dir returns the cache directory, creating it if needed.
func Dir() string {
	dir, _ := os.UserCacheDir()
	p := filepath.Join(dir, "jr")
	os.MkdirAll(p, 0o755)
	return p
}

// Key generates a cache key from method + URL.
func Key(method, url string) string {
	h := sha256.Sum256([]byte(method + " " + url))
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

// Set writes data to the cache.
func Set(key string, data []byte) {
	path := filepath.Join(Dir(), key)
	os.WriteFile(path, data, 0o644)
}
