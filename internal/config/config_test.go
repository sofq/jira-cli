package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
		"JR_AUTH_TYPE":  "bearer",
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
	if resolved.Auth.Type != "bearer" {
		t.Errorf("Auth.Type = %q, want %q", resolved.Auth.Type, "bearer")
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
		"JR_AUTH_TYPE":  "bearer",
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
	if resolved.Auth.Type != "bearer" {
		t.Errorf("Auth.Type = %q, want %q", resolved.Auth.Type, "bearer")
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

// Bug #10: Explicit --profile that doesn't exist should return error.
func TestResolveNonexistentProfileReturnsError(t *testing.T) {
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

	setEnv(t, map[string]string{
		"JR_BASE_URL":   "",
		"JR_AUTH_TYPE":  "",
		"JR_AUTH_USER":  "",
		"JR_AUTH_TOKEN": "",
	})

	_, err := config.Resolve(cfgPath, "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent profile, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention the profile name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "default") {
		t.Errorf("error should list available profiles, got: %v", err)
	}
}

// Empty profileName (default behavior) should NOT error even if "default" profile doesn't exist.
func TestResolveEmptyProfileNameNoError(t *testing.T) {
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
		t.Fatalf("Resolve with empty profileName should not error: %v", err)
	}
	if resolved.BaseURL != "https://example.com" {
		t.Errorf("BaseURL = %q, want %q", resolved.BaseURL, "https://example.com")
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

// TestResolveMultipleProfiles verifies that distinct profiles resolve to their
// own BaseURL values and do not bleed into each other.
func TestResolveMultipleProfiles(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "work",
		Profiles: map[string]config.Profile{
			"work": {
				BaseURL: "https://work.atlassian.net",
				Auth:    config.AuthConfig{Type: "basic", Token: "work-token"},
			},
			"personal": {
				BaseURL: "https://personal.atlassian.net",
				Auth:    config.AuthConfig{Type: "basic", Token: "personal-token"},
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	setEnv(t, map[string]string{
		"JR_BASE_URL":   "",
		"JR_AUTH_TYPE":  "",
		"JR_AUTH_USER":  "",
		"JR_AUTH_TOKEN": "",
	})

	workResolved, err := config.Resolve(cfgPath, "work", nil)
	if err != nil {
		t.Fatalf("Resolve(work): %v", err)
	}
	if workResolved.BaseURL != "https://work.atlassian.net" {
		t.Errorf("work BaseURL = %q, want %q", workResolved.BaseURL, "https://work.atlassian.net")
	}

	personalResolved, err := config.Resolve(cfgPath, "personal", nil)
	if err != nil {
		t.Fatalf("Resolve(personal): %v", err)
	}
	if personalResolved.BaseURL != "https://personal.atlassian.net" {
		t.Errorf("personal BaseURL = %q, want %q", personalResolved.BaseURL, "https://personal.atlassian.net")
	}
}

// TestResolveProfileWithOAuth2 verifies that OAuth2 fields survive a
// SaveTo/LoadFrom roundtrip and are present in the resolved profile.
func TestResolveProfileWithOAuth2(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "oauth",
		Profiles: map[string]config.Profile{
			"oauth": {
				BaseURL: "https://oauth.atlassian.net",
				Auth: config.AuthConfig{
					Type:         "oauth2",
					ClientID:     "client-id-123",
					ClientSecret: "client-secret-abc",
					TokenURL:     "https://auth.atlassian.com/oauth/token",
					Scopes:       "read:jira-work write:jira-work",
				},
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	loaded, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	p := loaded.Profiles["oauth"]
	if p.Auth.ClientID != "client-id-123" {
		t.Errorf("ClientID = %q, want %q", p.Auth.ClientID, "client-id-123")
	}
	if p.Auth.ClientSecret != "client-secret-abc" {
		t.Errorf("ClientSecret = %q, want %q", p.Auth.ClientSecret, "client-secret-abc")
	}
	if p.Auth.TokenURL != "https://auth.atlassian.com/oauth/token" {
		t.Errorf("TokenURL = %q, want %q", p.Auth.TokenURL, "https://auth.atlassian.com/oauth/token")
	}
	if p.Auth.Scopes != "read:jira-work write:jira-work" {
		t.Errorf("Scopes = %q, want %q", p.Auth.Scopes, "read:jira-work write:jira-work")
	}
}

// TestLoadFromInvalidJSON verifies that a file with malformed JSON returns an
// error rather than an empty config.
func TestLoadFromInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	if err := os.WriteFile(cfgPath, []byte(`{not valid json`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := config.LoadFrom(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// TestLoadFromEmptyFile verifies that an empty file is treated as invalid JSON
// and returns an error.
func TestLoadFromEmptyFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	if err := os.WriteFile(cfgPath, []byte(""), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := config.LoadFrom(cfgPath)
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
}

// TestSaveToCreatesParentDirs verifies that SaveTo creates all missing parent
// directories before writing the config file.
func TestSaveToCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "a", "b", "c", "config.json")

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

	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("expected file to exist at deeply nested path: %v", err)
	}
}

// TestResolveDefaultProfileFallback verifies that when no explicit profileName
// is given, Resolve falls back to the config's default_profile value.
func TestResolveDefaultProfileFallback(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "work",
		Profiles: map[string]config.Profile{
			"work": {
				BaseURL: "https://default-fallback.atlassian.net",
				Auth:    config.AuthConfig{Type: "basic", Token: "work-tok"},
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	setEnv(t, map[string]string{
		"JR_BASE_URL":   "",
		"JR_AUTH_TYPE":  "",
		"JR_AUTH_USER":  "",
		"JR_AUTH_TOKEN": "",
	})

	resolved, err := config.Resolve(cfgPath, "", nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.BaseURL != "https://default-fallback.atlassian.net" {
		t.Errorf("BaseURL = %q, want %q", resolved.BaseURL, "https://default-fallback.atlassian.net")
	}
}

// TestResolveEnvOnlyNoFlags verifies that resolution succeeds with env vars
// alone, without a config file or CLI flags.
func TestResolveEnvOnlyNoFlags(t *testing.T) {
	dir := t.TempDir()
	missingPath := filepath.Join(dir, "nonexistent.json")

	setEnv(t, map[string]string{
		"JR_BASE_URL":   "https://env-only-no-flags.example.com",
		"JR_AUTH_TYPE":  "bearer",
		"JR_AUTH_USER":  "env-user",
		"JR_AUTH_TOKEN": "env-tok",
	})

	resolved, err := config.Resolve(missingPath, "", nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.BaseURL != "https://env-only-no-flags.example.com" {
		t.Errorf("BaseURL = %q, want %q", resolved.BaseURL, "https://env-only-no-flags.example.com")
	}
	if resolved.Auth.Type != "bearer" {
		t.Errorf("Auth.Type = %q, want %q", resolved.Auth.Type, "bearer")
	}
	if resolved.Auth.Username != "env-user" {
		t.Errorf("Auth.Username = %q, want %q", resolved.Auth.Username, "env-user")
	}
	if resolved.Auth.Token != "env-tok" {
		t.Errorf("Auth.Token = %q, want %q", resolved.Auth.Token, "env-tok")
	}
}

// TestResolveRejectsInvalidAuthType verifies that Resolve returns an error
// for invalid auth types from any source (config, env, flags).
func TestResolveRejectsInvalidAuthType(t *testing.T) {
	dir := t.TempDir()

	// Case 1: Invalid auth type from config file.
	cfgPath := filepath.Join(dir, "config1.json")
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "invalidtype", Username: "u", Token: "t"},
			},
		},
		DefaultProfile: "default",
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	_, err := config.Resolve(cfgPath, "", nil)
	if err == nil {
		t.Error("case 1: expected error for invalid auth type in config file, got nil")
	}

	// Case 2: Invalid auth type from env var.
	cfgPath2 := filepath.Join(dir, "config2.json")
	cfg2 := &config.Config{
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
			},
		},
		DefaultProfile: "default",
	}
	if err := config.SaveTo(cfg2, cfgPath2); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	setEnv(t, map[string]string{"JR_AUTH_TYPE": "badtype"})
	_, err = config.Resolve(cfgPath2, "", nil)
	if err == nil {
		t.Error("case 2: expected error for invalid auth type from env var, got nil")
	}

	// Case 3: Invalid auth type from flags.
	setEnv(t, map[string]string{"JR_AUTH_TYPE": ""}) // clear env
	cfgPath3 := filepath.Join(dir, "config3.json")
	cfg3 := &config.Config{
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
			},
		},
		DefaultProfile: "default",
	}
	if err := config.SaveTo(cfg3, cfgPath3); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	flags := &config.FlagOverrides{AuthType: "notreal"}
	_, err = config.Resolve(cfgPath3, "", flags)
	if err == nil {
		t.Error("case 3: expected error for invalid auth type from flags, got nil")
	}
}

// TestResolveNormalizesAuthTypeCase verifies that auth type is lowercased.
func TestResolveNormalizesAuthTypeCase(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "BEARER", Username: "u", Token: "t"},
			},
		},
		DefaultProfile: "default",
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	resolved, err := config.Resolve(cfgPath, "", nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Auth.Type != "bearer" {
		t.Errorf("Auth.Type = %q, want %q (lowercased)", resolved.Auth.Type, "bearer")
	}
}

