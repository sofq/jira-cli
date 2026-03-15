package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sofq/jira-cli/internal/config"
)

// helper to unset a list of env vars and restore them after the test.
func setEnv(t *testing.T, pairs map[string]string) {
	t.Helper()
	for k, v := range pairs {
		old, exists := os.LookupEnv(k)
		if exists {
			t.Cleanup(func() { os.Setenv(k, old) })
		} else {
			t.Cleanup(func() { os.Unsetenv(k) })
		}
		if v == "" {
			os.Unsetenv(k)
		} else {
			os.Setenv(k, v)
		}
	}
}

// TestResolveEnvOverridesConfig verifies that env vars take precedence over
// values stored in the config file.
func TestResolveEnvOverridesConfig(t *testing.T) {
	// Write a minimal config file.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://file.example.com",
				Auth: config.AuthConfig{
					Type:     "basic",
					Username: "file-user",
					Token:    "file-token",
				},
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	setEnv(t, map[string]string{
		"JR_BASE_URL":   "https://env.example.com",
		"JR_AUTH_TYPE":  "token",
		"JR_AUTH_USER":  "env-user",
		"JR_AUTH_TOKEN": "env-token",
	})

	resolved, err := config.Resolve(cfgPath, "", nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if resolved.BaseURL != "https://env.example.com" {
		t.Errorf("BaseURL = %q, want %q", resolved.BaseURL, "https://env.example.com")
	}
	if resolved.Auth.Type != "token" {
		t.Errorf("Auth.Type = %q, want %q", resolved.Auth.Type, "token")
	}
	if resolved.Auth.Username != "env-user" {
		t.Errorf("Auth.Username = %q, want %q", resolved.Auth.Username, "env-user")
	}
	if resolved.Auth.Token != "env-token" {
		t.Errorf("Auth.Token = %q, want %q", resolved.Auth.Token, "env-token")
	}
}

// TestResolveFlagsOverrideEnv verifies that CLI flags take the highest priority.
func TestResolveFlagsOverrideEnv(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	// Config file with different values.
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://file.example.com",
				Auth: config.AuthConfig{
					Type:  "basic",
					Token: "file-token",
				},
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	setEnv(t, map[string]string{
		"JR_BASE_URL":   "https://env.example.com",
		"JR_AUTH_TOKEN": "env-token",
		"JR_AUTH_TYPE":  "",
		"JR_AUTH_USER":  "",
	})

	flags := &config.FlagOverrides{
		BaseURL: "https://flag.example.com",
		Token:   "flag-token",
	}

	resolved, err := config.Resolve(cfgPath, "", flags)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if resolved.BaseURL != "https://flag.example.com" {
		t.Errorf("BaseURL = %q, want %q", resolved.BaseURL, "https://flag.example.com")
	}
	if resolved.Auth.Token != "flag-token" {
		t.Errorf("Auth.Token = %q, want %q", resolved.Auth.Token, "flag-token")
	}
}

// TestResolvePureEnvNoConfigFile verifies that resolution works with only env
// vars, when no config file exists.
func TestResolvePureEnvNoConfigFile(t *testing.T) {
	dir := t.TempDir()
	missingPath := filepath.Join(dir, "nonexistent.json")

	setEnv(t, map[string]string{
		"JR_BASE_URL":   "https://env-only.example.com",
		"JR_AUTH_TYPE":  "token",
		"JR_AUTH_USER":  "only-env-user",
		"JR_AUTH_TOKEN": "only-env-token",
	})

	resolved, err := config.Resolve(missingPath, "", nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if resolved.BaseURL != "https://env-only.example.com" {
		t.Errorf("BaseURL = %q, want %q", resolved.BaseURL, "https://env-only.example.com")
	}
	if resolved.Auth.Type != "token" {
		t.Errorf("Auth.Type = %q, want %q", resolved.Auth.Type, "token")
	}
	if resolved.Auth.Username != "only-env-user" {
		t.Errorf("Auth.Username = %q, want %q", resolved.Auth.Username, "only-env-user")
	}
	if resolved.Auth.Token != "only-env-token" {
		t.Errorf("Auth.Token = %q, want %q", resolved.Auth.Token, "only-env-token")
	}
}

// TestSaveToAndLoadFrom verifies roundtrip persistence.
func TestSaveToAndLoadFrom(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "subdir", "config.json")

	original := &config.Config{
		DefaultProfile: "work",
		Profiles: map[string]config.Profile{
			"work": {
				BaseURL: "https://work.example.com",
				Auth: config.AuthConfig{
					Type:     "oauth2",
					Username: "worker",
					Token:    "work-token",
				},
			},
			"personal": {
				BaseURL: "https://personal.example.com",
				Auth: config.AuthConfig{
					Type:  "basic",
					Token: "personal-token",
				},
			},
		},
	}

	if err := config.SaveTo(original, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	// Verify file permissions are 0600.
	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file permissions = %o, want 600", perm)
	}

	loaded, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if loaded.DefaultProfile != original.DefaultProfile {
		t.Errorf("DefaultProfile = %q, want %q", loaded.DefaultProfile, original.DefaultProfile)
	}
	if len(loaded.Profiles) != len(original.Profiles) {
		t.Errorf("len(Profiles) = %d, want %d", len(loaded.Profiles), len(original.Profiles))
	}

	work := loaded.Profiles["work"]
	if work.BaseURL != "https://work.example.com" {
		t.Errorf("work.BaseURL = %q", work.BaseURL)
	}
	if work.Auth.Type != "oauth2" {
		t.Errorf("work.Auth.Type = %q", work.Auth.Type)
	}
}

// TestLoadFromMissingFile verifies that a missing config file returns an empty
// (non-nil) Config without error.
func TestLoadFromMissingFile(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.json")

	cfg, err := config.LoadFrom(missing)
	if err != nil {
		t.Fatalf("LoadFrom missing file returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadFrom returned nil config")
	}
}

// TestResolveDefaultsAuthTypeToBasic verifies that when no auth type is
// provided through any source, the default "basic" is used.
func TestResolveDefaultsAuthTypeToBasic(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.json")

	setEnv(t, map[string]string{
		"JR_BASE_URL":   "https://example.com",
		"JR_AUTH_TYPE":  "",
		"JR_AUTH_USER":  "",
		"JR_AUTH_TOKEN": "",
	})

	resolved, err := config.Resolve(missing, "", nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Auth.Type != "basic" {
		t.Errorf("Auth.Type = %q, want %q", resolved.Auth.Type, "basic")
	}
}

// TestResolveTrimsTrailingSlash verifies BaseURL trailing slashes are removed.
func TestResolveTrimsTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.json")

	setEnv(t, map[string]string{
		"JR_BASE_URL":   "https://example.com/jira/",
		"JR_AUTH_TYPE":  "",
		"JR_AUTH_USER":  "",
		"JR_AUTH_TOKEN": "",
	})

	resolved, err := config.Resolve(missing, "", nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.BaseURL != "https://example.com/jira" {
		t.Errorf("BaseURL = %q, want trailing slash trimmed", resolved.BaseURL)
	}
}

// TestDefaultPath verifies JR_CONFIG_PATH env var is respected.
func TestDefaultPath(t *testing.T) {
	custom := "/tmp/custom-jr-config.json"
	setEnv(t, map[string]string{"JR_CONFIG_PATH": custom})

	got := config.DefaultPath()
	if got != custom {
		t.Errorf("DefaultPath() = %q, want %q", got, custom)
	}
}

// TestSaveToCreatesJSON verifies the saved file is valid JSON.
func TestSaveToCreatesJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Token: "tok"},
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("saved file is not valid JSON: %v", err)
	}
}
