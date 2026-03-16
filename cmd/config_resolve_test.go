package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/config"
)

func TestResolveRejectsInvalidAuthType(t *testing.T) {
	// Create a config file with a valid profile.
	dir := t.TempDir()
	cfgPath := dir + "/config.json"
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
			},
		},
		DefaultProfile: "default",
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	// Resolve with invalid auth-type flag override should fail.
	flags := &config.FlagOverrides{AuthType: "invalidtype"}
	_, err := config.Resolve(cfgPath, "", flags)
	if err == nil {
		t.Fatal("expected error for invalid auth type, got nil")
	}
	if !strings.Contains(err.Error(), "invalid auth type") {
		t.Errorf("error message = %q, want it to contain 'invalid auth type'", err.Error())
	}
}

func TestResolveAcceptsValidAuthTypes(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/config.json"
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth: config.AuthConfig{
					Type:         "basic",
					Username:     "u",
					Token:        "t",
					ClientID:     "cid",
					ClientSecret: "csecret",
					TokenURL:     "https://auth.example.com/token",
				},
			},
		},
		DefaultProfile: "default",
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	for _, validType := range []string{"basic", "bearer", "oauth2", "Basic", "BEARER", "OAuth2"} {
		flags := &config.FlagOverrides{AuthType: validType}
		resolved, err := config.Resolve(cfgPath, "", flags)
		if err != nil {
			t.Errorf("Resolve(%q): unexpected error: %v", validType, err)
			continue
		}
		if resolved.Auth.Type != strings.ToLower(validType) {
			t.Errorf("Resolve(%q): Auth.Type = %q, want %q", validType, resolved.Auth.Type, strings.ToLower(validType))
		}
	}
}

func TestResolveOAuth2MissingFields(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/config.json"
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
			},
		},
		DefaultProfile: "default",
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	// Override auth type to oauth2 via flags (simulating env var or --auth-type).
	flags := &config.FlagOverrides{AuthType: "oauth2"}
	_, err := config.Resolve(cfgPath, "", flags)
	if err == nil {
		t.Fatal("expected error for oauth2 without required fields, got nil")
	}
	if !strings.Contains(err.Error(), "client_id") {
		t.Errorf("error should mention client_id, got: %v", err)
	}
	if !strings.Contains(err.Error(), "client_secret") {
		t.Errorf("error should mention client_secret, got: %v", err)
	}
	if !strings.Contains(err.Error(), "token_url") {
		t.Errorf("error should mention token_url, got: %v", err)
	}
}

func TestResolveOAuth2WithAllFieldsSucceeds(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/config.json"
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth: config.AuthConfig{
					Type:         "oauth2",
					ClientID:     "my-client",
					ClientSecret: "my-secret",
					TokenURL:     "https://auth.example.com/token",
				},
			},
		},
		DefaultProfile: "default",
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	resolved, err := config.Resolve(cfgPath, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Auth.Type != "oauth2" {
		t.Errorf("Auth.Type = %q, want oauth2", resolved.Auth.Type)
	}
	if resolved.Auth.ClientID != "my-client" {
		t.Errorf("Auth.ClientID = %q, want my-client", resolved.Auth.ClientID)
	}
}

func TestResolveOAuth2PartialFieldsMissing(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/config.json"
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: "https://example.com",
				Auth: config.AuthConfig{
					Type:     "oauth2",
					ClientID: "my-client",
					// Missing client_secret and token_url.
				},
			},
		},
		DefaultProfile: "default",
	}
	if err := config.SaveTo(cfg, cfgPath); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	_, err := config.Resolve(cfgPath, "", nil)
	if err == nil {
		t.Fatal("expected error for oauth2 with missing client_secret and token_url")
	}
	if !strings.Contains(err.Error(), "client_secret") {
		t.Errorf("error should mention client_secret, got: %v", err)
	}
	if !strings.Contains(err.Error(), "token_url") {
		t.Errorf("error should mention token_url, got: %v", err)
	}
	// Should NOT mention client_id since it IS provided.
	if strings.Contains(err.Error(), "client_id") {
		t.Errorf("error should NOT mention client_id (it is provided), got: %v", err)
	}
}

func TestResolve_MissingProfile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"existing": {BaseURL: "https://example.com"},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}

	_, err := config.Resolve(configPath, "missing", nil)
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "existing") {
		t.Errorf("expected available profiles listed, got: %v", err)
	}
}

func TestResolve_EmptyProfileFallsToDefault(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "mydefault",
		Profiles: map[string]config.Profile{
			"mydefault": {
				BaseURL: "https://example.com",
				Auth:    config.AuthConfig{Type: "basic", Token: "tok"},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}

	resolved, err := config.Resolve(configPath, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.BaseURL != "https://example.com" {
		t.Errorf("expected base URL from default profile, got: %s", resolved.BaseURL)
	}
}
