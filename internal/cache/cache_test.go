package cache_test

import (
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
