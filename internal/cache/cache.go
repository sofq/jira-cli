package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// MaxAge is the maximum age of a cache entry before it is eligible for
// eviction. Entries older than this are removed by Evict.
var MaxAge = 24 * time.Hour

// MaxEntries is the maximum number of cache entries to keep. When Evict
// runs and finds more entries than this, the oldest are removed first.
// Zero means no limit on the number of entries.
var MaxEntries = 1000

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

// Evict removes stale cache entries. It deletes files older than MaxAge
// and, if MaxEntries > 0, trims the remaining entries to that limit by
// removing the oldest first. Returns the number of entries removed.
func Evict() int {
	dir := Dir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	now := time.Now()
	removed := 0

	// Phase 1: remove entries older than MaxAge.
	type surviving struct {
		name    string
		modTime time.Time
	}
	var kept []surviving

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		age := now.Sub(info.ModTime())
		if age > MaxAge {
			if os.Remove(filepath.Join(dir, e.Name())) == nil {
				removed++
			}
		} else {
			kept = append(kept, surviving{name: e.Name(), modTime: info.ModTime()})
		}
	}

	// Phase 2: if MaxEntries > 0, trim to that limit.
	if MaxEntries > 0 && len(kept) > MaxEntries {
		// Sort oldest first.
		sort.Slice(kept, func(i, j int) bool {
			return kept[i].modTime.Before(kept[j].modTime)
		})
		toRemove := len(kept) - MaxEntries
		for i := 0; i < toRemove; i++ {
			if os.Remove(filepath.Join(dir, kept[i].name)) == nil {
				removed++
			}
		}
	}

	return removed
}
