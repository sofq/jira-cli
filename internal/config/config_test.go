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

// Bug #44: Resolve must carry OAuth2 fields (client_id, client_secret, token_url,
// scopes) from the profile into the ResolvedConfig.
func TestResolvePreservesOAuth2Fields(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "oauth",
		Profiles: map[string]config.Profile{
			"oauth": {
				BaseURL: "https://oauth.atlassian.net",
				Auth: config.AuthConfig{
					Type:         "oauth2",
					ClientID:     "my-client-id",
					ClientSecret: "my-client-secret",
					TokenURL:     "https://auth.atlassian.com/oauth/token",
					Scopes:       "read:jira-work write:jira-work",
				},
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

	resolved, err := config.Resolve(cfgPath, "oauth", nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if resolved.Auth.Type != "oauth2" {
		t.Errorf("Auth.Type = %q, want %q", resolved.Auth.Type, "oauth2")
	}
	if resolved.Auth.ClientID != "my-client-id" {
		t.Errorf("Auth.ClientID = %q, want %q", resolved.Auth.ClientID, "my-client-id")
	}
	if resolved.Auth.ClientSecret != "my-client-secret" {
		t.Errorf("Auth.ClientSecret = %q, want %q", resolved.Auth.ClientSecret, "my-client-secret")
	}
	if resolved.Auth.TokenURL != "https://auth.atlassian.com/oauth/token" {
		t.Errorf("Auth.TokenURL = %q, want %q", resolved.Auth.TokenURL, "https://auth.atlassian.com/oauth/token")
	}
	if resolved.Auth.Scopes != "read:jira-work write:jira-work" {
		t.Errorf("Auth.Scopes = %q, want %q", resolved.Auth.Scopes, "read:jira-work write:jira-work")
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

// TestDefaultPathWithoutEnv verifies that DefaultPath returns a non-empty
// OS-specific path when JR_CONFIG_PATH is not set.
func TestDefaultPathWithoutEnv(t *testing.T) {
	setEnv(t, map[string]string{"JR_CONFIG_PATH": ""})

	got := config.DefaultPath()
	if got == "" {
		t.Error("DefaultPath() returned empty string when JR_CONFIG_PATH is unset")
	}
	if !strings.Contains(got, "config.json") {
		t.Errorf("DefaultPath() = %q, expected path ending in config.json", got)
	}
}

// TestDefaultPathPerOS verifies the OS-specific branches in DefaultPath.
func TestDefaultPathPerOS(t *testing.T) {
	setEnv(t, map[string]string{"JR_CONFIG_PATH": ""})

	tests := []struct {
		goos     string
		wantSub  string
	}{
		{"darwin", filepath.Join("Library", "Application Support", "jr", "config.json")},
		{"linux", filepath.Join(".config", "jr", "config.json")},
		{"windows", filepath.Join("jr", "config.json")},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			if tt.goos == "windows" {
				setEnv(t, map[string]string{"APPDATA": "/fake/appdata"})
			}
			config.SetGOOS(tt.goos)
			t.Cleanup(func() { config.ResetGOOS() })

			got := config.DefaultPath()
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("DefaultPath() with GOOS=%s = %q, want substring %q", tt.goos, got, tt.wantSub)
			}
		})
	}
}

// TestDefaultPathWindowsEmptyAPPDATA verifies that when APPDATA is unset on Windows,
// DefaultPath falls back to UserHomeDir rather than producing a relative path.
func TestDefaultPathWindowsEmptyAPPDATA(t *testing.T) {
	setEnv(t, map[string]string{
		"JR_CONFIG_PATH": "",
		"APPDATA":        "",
	})
	config.SetGOOS("windows")
	t.Cleanup(func() { config.ResetGOOS() })

	got := config.DefaultPath()
	if got == "" {
		t.Error("DefaultPath() returned empty string with APPDATA unset on Windows")
	}
	// Must not be a relative path.
	if !strings.Contains(got, "config.json") {
		t.Errorf("DefaultPath() = %q, expected path containing config.json", got)
	}
	// Should not start with just "jr/" (relative).
	if strings.HasPrefix(got, "jr") {
		t.Errorf("DefaultPath() = %q, should not be a relative path", got)
	}
}

// TestLoadFromReadError verifies that LoadFrom returns an error (not a
// fallback empty config) when the file exists but cannot be read.
func TestLoadFromReadError(t *testing.T) {
	dir := t.TempDir()
	// Use a directory as the file path — reading a directory as a file fails.
	_, err := config.LoadFrom(dir)
	if err == nil {
		t.Fatal("expected error when reading a directory as a file, got nil")
	}
}

// TestLoadFromNullProfiles verifies that LoadFrom initialises the Profiles
// map when the JSON has "profiles": null.
func TestLoadFromNullProfiles(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	if err := os.WriteFile(cfgPath, []byte(`{"profiles":null,"default_profile":"x"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Profiles == nil {
		t.Error("expected Profiles to be initialised (non-nil), got nil")
	}
}

// TestSaveToMkdirAllError verifies that SaveTo returns an error when it
// cannot create parent directories.
func TestSaveToMkdirAllError(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file where a directory is expected.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Try to save under blocker/sub/config.json — MkdirAll should fail.
	cfgPath := filepath.Join(blocker, "sub", "config.json")
	cfg := &config.Config{Profiles: map[string]config.Profile{}}
	if err := config.SaveTo(cfg, cfgPath); err == nil {
		t.Fatal("expected MkdirAll error, got nil")
	}
}

// TestResolveLoadFromError verifies that Resolve surfaces LoadFrom errors.
func TestResolveLoadFromError(t *testing.T) {
	dir := t.TempDir()
	setEnv(t, map[string]string{
		"JR_BASE_URL":   "",
		"JR_AUTH_TYPE":  "",
		"JR_AUTH_USER":  "",
		"JR_AUTH_TOKEN": "",
	})
	// Use a directory as the config path to trigger a read error.
	_, err := config.Resolve(dir, "", nil)
	if err == nil {
		t.Fatal("expected Resolve to return error from LoadFrom, got nil")
	}
}

// TestResolveNonexistentProfileNoProfiles verifies the "(none)" branch of
// availableProfiles when the config has zero profiles.
func TestResolveNonexistentProfileNoProfiles(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{Profiles: map[string]config.Profile{}}
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
	if !strings.Contains(err.Error(), "(none)") {
		t.Errorf("error should contain '(none)' for empty profiles, got: %v", err)
	}
}

// TestResolveFlagsUsernameAndAuthType verifies that flags.Username and
// flags.AuthType override all other sources.
func TestResolveFlagsUsernameAndAuthType(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Username: "file-user", Token: "tok"},
			},
		},
		DefaultProfile: "default",
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

	flags := &config.FlagOverrides{
		Username: "flag-user",
		AuthType: "bearer",
	}
	resolved, err := config.Resolve(cfgPath, "", flags)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Auth.Username != "flag-user" {
		t.Errorf("Auth.Username = %q, want %q", resolved.Auth.Username, "flag-user")
	}
	if resolved.Auth.Type != "bearer" {
		t.Errorf("Auth.Type = %q, want %q", resolved.Auth.Type, "bearer")
	}
}

// TestResolveOAuth2MissingFields verifies that each required OAuth2 field
// produces a validation error when missing.
func TestResolveOAuth2MissingFields(t *testing.T) {
	tests := []struct {
		name         string
		clientID     string
		clientSecret string
		tokenURL     string
		wantMissing  string
	}{
		{"missing_client_id", "", "secret", "https://tok", "client_id"},
		{"missing_client_secret", "cid", "", "https://tok", "client_secret"},
		{"missing_token_url", "cid", "secret", "", "token_url"},
		{"missing_all", "", "", "", "client_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.json")

			cfg := &config.Config{
				Profiles: map[string]config.Profile{
					"default": {
						BaseURL: "https://example.com",
						Auth: config.AuthConfig{
							Type:         "oauth2",
							ClientID:     tt.clientID,
							ClientSecret: tt.clientSecret,
							TokenURL:     tt.tokenURL,
						},
					},
				},
				DefaultProfile: "default",
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

			_, err := config.Resolve(cfgPath, "", nil)
			if err == nil {
				t.Fatal("expected error for missing OAuth2 fields, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantMissing) {
				t.Errorf("error should mention %q, got: %v", tt.wantMissing, err)
			}
		})
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

func TestValidAuthType(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"basic", true},
		{"bearer", true},
		{"oauth2", true},
		{"Basic", true},
		{"BEARER", true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := config.ValidAuthType(tt.input); got != tt.want {
				t.Errorf("ValidAuthType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestProfilePolicyFieldsRoundtrip verifies that allowed/denied operations
// survive a SaveTo/LoadFrom roundtrip.
func TestProfilePolicyFieldsRoundtrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "agent",
		Profiles: map[string]config.Profile{
			"agent": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Token: "tok"},
				AllowedOperations: []string{"issue get", "search *"},
			},
			"restricted": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Token: "tok"},
				DeniedOperations: []string{"* delete*", "bulk *"},
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

	agent := loaded.Profiles["agent"]
	if len(agent.AllowedOperations) != 2 {
		t.Errorf("agent AllowedOperations = %v, want 2 items", agent.AllowedOperations)
	}

	restricted := loaded.Profiles["restricted"]
	if len(restricted.DeniedOperations) != 2 {
		t.Errorf("restricted DeniedOperations = %v, want 2 items", restricted.DeniedOperations)
	}
}

// TestProfileAuditLogFieldRoundtrip verifies audit_log survives roundtrip.
func TestProfileAuditLogFieldRoundtrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "audited",
		Profiles: map[string]config.Profile{
			"audited": {
				BaseURL:  "https://example.com",
				Auth:     config.AuthConfig{Type: "basic", Token: "tok"},
				AuditLog: true,
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

	if !loaded.Profiles["audited"].AuditLog {
		t.Error("expected AuditLog=true after roundtrip")
	}
}

// TestResolveReturnsProfileName verifies that Resolve carries the profile name.
func TestResolveReturnsProfileName(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "work",
		Profiles: map[string]config.Profile{
			"work": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Token: "tok"},
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	setEnv(t, map[string]string{
		"JR_BASE_URL": "", "JR_AUTH_TYPE": "", "JR_AUTH_USER": "", "JR_AUTH_TOKEN": "",
	})

	resolved, err := config.Resolve(cfgPath, "work", nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.ProfileName != "work" {
		t.Errorf("ProfileName = %q, want %q", resolved.ProfileName, "work")
	}
}

// TestAvatarConfigRoundTrip verifies that AvatarConfig fields survive a
// SaveTo/LoadFrom roundtrip.
func TestAvatarConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Token: "tok"},
				Avatar: &config.AvatarConfig{
					Enabled:     true,
					Engine:      "claude-3-5-sonnet",
					LLMCmd:      "llm",
					MinComments: 20,
					MinUpdates:  5,
					MaxWindow:   "90d",
					Overrides:   map[string]string{"greeting": "Hi"},
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

	p := loaded.Profiles["default"]
	if p.Avatar == nil {
		t.Fatal("expected Avatar to be non-nil after roundtrip")
	}
	if !p.Avatar.Enabled {
		t.Error("Avatar.Enabled should be true")
	}
	if p.Avatar.Engine != "claude-3-5-sonnet" {
		t.Errorf("Avatar.Engine = %q, want %q", p.Avatar.Engine, "claude-3-5-sonnet")
	}
	if p.Avatar.LLMCmd != "llm" {
		t.Errorf("Avatar.LLMCmd = %q, want %q", p.Avatar.LLMCmd, "llm")
	}
	if p.Avatar.MinComments != 20 {
		t.Errorf("Avatar.MinComments = %d, want 20", p.Avatar.MinComments)
	}
	if p.Avatar.MinUpdates != 5 {
		t.Errorf("Avatar.MinUpdates = %d, want 5", p.Avatar.MinUpdates)
	}
	if p.Avatar.MaxWindow != "90d" {
		t.Errorf("Avatar.MaxWindow = %q, want 90d", p.Avatar.MaxWindow)
	}
	if v, ok := p.Avatar.Overrides["greeting"]; !ok || v != "Hi" {
		t.Errorf("Avatar.Overrides[greeting] = %q, want Hi", v)
	}
}

// TestResolveReturnsPolicyFields verifies that Resolve carries allow/deny lists.
func TestResolveReturnsPolicyFields(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL:           "https://example.com",
				Auth:              config.AuthConfig{Type: "basic", Token: "tok"},
				AllowedOperations: []string{"issue *"},
			},
		},
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	setEnv(t, map[string]string{
		"JR_BASE_URL": "", "JR_AUTH_TYPE": "", "JR_AUTH_USER": "", "JR_AUTH_TOKEN": "",
	})

	resolved, err := config.Resolve(cfgPath, "", nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(resolved.AllowedOperations) != 1 || resolved.AllowedOperations[0] != "issue *" {
		t.Errorf("AllowedOperations = %v, want [issue *]", resolved.AllowedOperations)
	}
}

