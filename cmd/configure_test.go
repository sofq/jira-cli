package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

// newConfigureCmd returns a fresh cobra.Command wired to runConfigure with
// all flags that the real configureCmd registers. Using a local command
// avoids mutating the global configureCmd state between tests.
func newConfigureCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "configure", RunE: runConfigure}
	f := cmd.Flags()
	f.String("base-url", "", "")
	f.String("token", "", "")
	f.StringP("profile", "p", "default", "profile name")
	f.String("auth-type", "basic", "")
	f.String("username", "", "")
	f.Bool("test", false, "")
	f.Bool("delete", false, "")
	f.String("jq", "", "")
	f.Bool("pretty", false, "")
	return cmd
}

// --- deleteProfileByName ---

func TestConfigureDelete_RequiresExplicitProfile(t *testing.T) {
	// --delete without --profile must be rejected.
	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("delete", "true")

	err := runConfigure(cmd, nil)
	if err == nil {
		t.Fatal("expected error when --delete used without --profile")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

func TestDeleteProfile_ListsAvailableProfiles(t *testing.T) {
	// Deleting a nonexistent profile should return ExitNotFound and list
	// what profiles are available.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "prod",
		Profiles: map[string]config.Profile{
			"prod":    {BaseURL: "https://prod.example.com"},
			"staging": {BaseURL: "https://staging.example.com"},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("delete", "true")
	_ = cmd.Flags().Set("profile", "nonexistent")

	err := runConfigure(cmd, nil)
	if err == nil {
		t.Fatal("expected error deleting nonexistent profile")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitNotFound {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitNotFound, aw.Code)
	}
}

func TestDeleteProfile_Success(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "myprofile",
		Profiles: map[string]config.Profile{
			"myprofile": {BaseURL: "https://example.com"},
			"other":     {BaseURL: "https://other.example.com"},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("delete", "true")
	_ = cmd.Flags().Set("profile", "myprofile")

	captured := captureStdout(t, func() {
		if err := runConfigure(cmd, nil); err != nil {
			t.Fatalf("runConfigure error: %v", err)
		}
	})

	if !strings.Contains(captured, `"deleted"`) {
		t.Errorf("expected deleted status, got: %s", captured)
	}

	// Profile should be gone; default should be cleared since it was the deleted profile.
	reloaded, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Profiles["myprofile"]; ok {
		t.Error("profile should have been deleted")
	}
	if reloaded.DefaultProfile != "" {
		t.Errorf("default profile should have been cleared, got: %s", reloaded.DefaultProfile)
	}
}

func TestDeleteProfile_NotDefault_KeepsDefault(t *testing.T) {
	// Deleting a non-default profile must not affect the default pointer.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {BaseURL: "https://example.com"},
			"other":   {BaseURL: "https://other.example.com"},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("delete", "true")
	_ = cmd.Flags().Set("profile", "other")

	captureStdout(t, func() {
		if err := runConfigure(cmd, nil); err != nil {
			t.Fatalf("runConfigure error: %v", err)
		}
	})

	reloaded, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Profiles["other"]; ok {
		t.Error("profile 'other' should have been deleted")
	}
	if reloaded.DefaultProfile != "default" {
		t.Errorf("default profile should be preserved, got: %s", reloaded.DefaultProfile)
	}
}

// --- runConfigure: validation ---

func TestConfigureRejectsEmptyProfile(t *testing.T) {
	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("base-url", "https://test.atlassian.net")
	_ = cmd.Flags().Set("token", "fake-token")
	_ = cmd.Flags().Set("username", "user@test.com")
	_ = cmd.Flags().Set("profile", "   ")

	err := runConfigure(cmd, nil)
	if err == nil {
		t.Fatal("expected error for whitespace-only profile name")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

func TestConfigureRejectsInvalidAuthType(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	origPath := os.Getenv("JR_CONFIG_PATH")
	os.Setenv("JR_CONFIG_PATH", configPath)
	defer os.Setenv("JR_CONFIG_PATH", origPath)

	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("base-url", "https://example.atlassian.net")
	_ = cmd.Flags().Set("token", "test-token")
	_ = cmd.Flags().Set("auth-type", "cloud")

	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	err := runConfigure(cmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid auth-type, got nil")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T: %v", err, err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit code %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

func TestConfigureRejectsOAuth2(t *testing.T) {
	// The configure command lacks OAuth2-specific flags, so oauth2 profiles
	// would always fail at runtime. Reject it at configure time.
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	origPath := os.Getenv("JR_CONFIG_PATH")
	os.Setenv("JR_CONFIG_PATH", configPath)
	defer os.Setenv("JR_CONFIG_PATH", origPath)

	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("base-url", "https://example.atlassian.net")
	_ = cmd.Flags().Set("token", "test-token")
	_ = cmd.Flags().Set("auth-type", "oauth2")

	// Capture os.Stderr directly because runConfigure writes there before
	// wrapping the error.
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := runConfigure(cmd, nil)

	w.Close()
	stderrData := make([]byte, 4096)
	n, _ := r.Read(stderrData)
	os.Stderr = origStderr

	if err == nil {
		t.Fatal("expected error when configuring oauth2")
	}
	stderrStr := string(stderrData[:n])
	if !strings.Contains(stderrStr, "oauth2 is not supported by the configure command") {
		t.Errorf("expected helpful oauth2 rejection message, got: %s", stderrStr)
	}
}

func TestConfigureAcceptsValidAuthTypes(t *testing.T) {
	for _, authType := range []string{"basic", "bearer", "BASIC", "Bearer"} {
		t.Run(authType, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := tmpDir + "/config.json"
			origPath := os.Getenv("JR_CONFIG_PATH")
			os.Setenv("JR_CONFIG_PATH", configPath)
			defer os.Setenv("JR_CONFIG_PATH", origPath)

			cmd := newConfigureCmd()
			_ = cmd.Flags().Set("base-url", "https://example.atlassian.net")
			_ = cmd.Flags().Set("token", "test-token")
			_ = cmd.Flags().Set("username", "user@example.com")
			_ = cmd.Flags().Set("auth-type", authType)

			captured := captureStdout(t, func() {
				if err := runConfigure(cmd, nil); err != nil {
					t.Fatalf("unexpected error for auth-type %q: %v", authType, err)
				}
			})

			if !strings.Contains(captured, `"status":"saved"`) {
				t.Errorf("expected saved status for auth-type %q, got: %s", authType, captured)
			}

			// Auth type must be lowercased in the saved config.
			cfg, err := config.LoadFrom(configPath)
			if err != nil {
				t.Fatal(err)
			}
			got := cfg.Profiles["default"].Auth.Type
			if got != strings.ToLower(authType) {
				t.Errorf("expected saved auth type %q, got %q", strings.ToLower(authType), got)
			}
		})
	}
}

// --- runConfigure: save ---

func TestConfigure_SaveSuccess(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("base-url", "https://test.atlassian.net/")
	_ = cmd.Flags().Set("token", "mytoken")
	_ = cmd.Flags().Set("username", "user@test.com")
	_ = cmd.Flags().Set("profile", "myprofile")

	captured := captureStdout(t, func() {
		if err := runConfigure(cmd, nil); err != nil {
			t.Fatalf("runConfigure error: %v", err)
		}
	})

	if !strings.Contains(captured, `"saved"`) {
		t.Errorf("expected saved status, got: %s", captured)
	}

	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatal(err)
	}
	profile := cfg.Profiles["myprofile"]
	// Trailing slash must be stripped.
	if profile.BaseURL != "https://test.atlassian.net" {
		t.Errorf("expected normalized base URL, got: %s", profile.BaseURL)
	}
	if profile.Auth.Token != "mytoken" {
		t.Errorf("expected token 'mytoken', got: %s", profile.Auth.Token)
	}
	// DefaultProfile should be set when config was previously empty.
	if cfg.DefaultProfile != "myprofile" {
		t.Errorf("expected default profile 'myprofile', got: %s", cfg.DefaultProfile)
	}
}

func TestConfigure_BearerAuth(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("base-url", "https://test.atlassian.net")
	_ = cmd.Flags().Set("token", "bearer-token")
	_ = cmd.Flags().Set("auth-type", "bearer")

	captureStdout(t, func() {
		if err := runConfigure(cmd, nil); err != nil {
			t.Fatalf("runConfigure error: %v", err)
		}
	})

	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profiles["default"].Auth.Type != "bearer" {
		t.Errorf("expected auth type 'bearer', got: %s", cfg.Profiles["default"].Auth.Type)
	}
}

func TestConfigureNormalizesTrailingSlash(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	origPath := os.Getenv("JR_CONFIG_PATH")
	os.Setenv("JR_CONFIG_PATH", configPath)
	defer os.Setenv("JR_CONFIG_PATH", origPath)

	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("base-url", "https://example.atlassian.net/")
	_ = cmd.Flags().Set("token", "test-token")
	_ = cmd.Flags().Set("username", "user@example.com")

	captureStdout(t, func() {
		if err := runConfigure(cmd, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.Profiles["default"].BaseURL
	if strings.HasSuffix(got, "/") {
		t.Errorf("expected trailing slash to be stripped, got: %s", got)
	}
}

func TestConfigureShortProfileFlag(t *testing.T) {
	// The configure command's local flag set must expose -p as a shorthand for --profile.
	// LocalFlags() is used to avoid interference from persistent parent flags.
	f := configureCmd.LocalFlags().Lookup("profile")
	if f == nil {
		t.Fatal("configure command does not have a local 'profile' flag")
	}
	if f.Shorthand != "p" {
		t.Errorf("configure local 'profile' flag shorthand = %q, want %q", f.Shorthand, "p")
	}
}

// --- runConfigure: --test flag ---

func TestConfigureTest_SingleError(t *testing.T) {
	// A failed connection test must return AlreadyWrittenError so that the
	// Cobra runner does not print the error a second time.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("base-url", ts.URL)
	_ = cmd.Flags().Set("token", "faketoken")
	_ = cmd.Flags().Set("test", "true")

	err := runConfigure(cmd, nil)
	if err == nil {
		t.Fatal("expected error for failed connection test")
	}
	if _, ok := err.(*jrerrors.AlreadyWrittenError); !ok {
		t.Errorf("expected AlreadyWrittenError to prevent duplicate errors, got %T: %v", err, err)
	}
}

func TestConfigureTest_JQFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"accountId":"abc123"}`)
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: ts.URL,
				Auth:    config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("JR_CONFIG_PATH")
	os.Setenv("JR_CONFIG_PATH", configPath)
	defer os.Setenv("JR_CONFIG_PATH", origPath)

	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("test", "true")
	_ = cmd.Flags().Set("jq", ".status")

	captured := captureStdout(t, func() {
		err := runConfigure(cmd, nil)
		if err != nil {
			t.Fatalf("runConfigure error: %v", err)
		}
	})

	got := strings.TrimSpace(captured)
	if got != `"ok"` {
		t.Errorf("expected jq-filtered output %q, got %q", `"ok"`, got)
	}
}

func TestConfigureTest_PrettyPrint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"accountId":"abc123"}`)
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: ts.URL,
				Auth:    config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("JR_CONFIG_PATH")
	os.Setenv("JR_CONFIG_PATH", configPath)
	defer os.Setenv("JR_CONFIG_PATH", origPath)

	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("test", "true")
	_ = cmd.Flags().Set("pretty", "true")

	captured := captureStdout(t, func() {
		err := runConfigure(cmd, nil)
		if err != nil {
			t.Fatalf("runConfigure error: %v", err)
		}
	})

	if !strings.Contains(captured, "\n") || !strings.Contains(captured, "  ") {
		t.Errorf("expected pretty-printed output, got: %s", captured)
	}
}

// --- testExistingProfile ---

func TestConfigure_TestExistingProfile_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"accountId":"abc123"}`)
	}))
	defer ts.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "myprofile",
		Profiles: map[string]config.Profile{
			"myprofile": {
				BaseURL: ts.URL,
				Auth:    config.AuthConfig{Type: "basic", Username: "u", Token: "t"},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	// Set --test but NOT --base-url or --token to trigger test-only (existing profile) mode.
	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("test", "true")

	captured := captureStdout(t, func() {
		if err := runConfigure(cmd, nil); err != nil {
			t.Fatalf("runConfigure error: %v", err)
		}
	})

	if !strings.Contains(captured, `"ok"`) {
		t.Errorf("expected status ok, got: %s", captured)
	}
	if !strings.Contains(captured, `"myprofile"`) {
		t.Errorf("expected profile name in output, got: %s", captured)
	}
}

func TestConfigure_TestExistingProfile_EmptyBaseURL(t *testing.T) {
	// A profile with an empty base_url must fail validation, not panic.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "empty",
		Profiles: map[string]config.Profile{
			"empty": {BaseURL: ""},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("test", "true")
	_ = cmd.Flags().Set("profile", "empty")

	err := runConfigure(cmd, nil)
	if err == nil {
		t.Fatal("expected error for empty base_url profile")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitValidation {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitValidation, aw.Code)
	}
}

func TestConfigure_TestExistingProfile_ProfileNotFound(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		Profiles: map[string]config.Profile{},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newConfigureCmd()
	_ = cmd.Flags().Set("test", "true")
	_ = cmd.Flags().Set("profile", "nonexistent")

	err := runConfigure(cmd, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
	aw, ok := err.(*jrerrors.AlreadyWrittenError)
	if !ok {
		t.Fatalf("expected AlreadyWrittenError, got %T", err)
	}
	if aw.Code != jrerrors.ExitNotFound {
		t.Errorf("expected exit %d, got %d", jrerrors.ExitNotFound, aw.Code)
	}
}

// --- testConnection ---

func TestTestConnection_OAuth2FallsToBasic(t *testing.T) {
	// runConfigure rejects oauth2 before reaching testConnection, so oauth2
	// passed directly to testConnection falls through to basic auth.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"accountId":"abc123"}`)
	}))
	defer ts.Close()

	err := testConnection(ts.URL, "oauth2", "user", "token")
	if err != nil {
		t.Errorf("expected testConnection to succeed with oauth2 falling through to basic, got: %v", err)
	}
}

func TestTestConnection_TrailingSlashNormalized(t *testing.T) {
	// A trailing slash in the base URL must not produce a double-slash in the
	// outgoing request path.
	var receivedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"accountId":"abc123"}`)
	}))
	defer ts.Close()

	err := testConnection(ts.URL+"/", "basic", "user", "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(receivedPath, "//") {
		t.Errorf("testConnection produced double-slash in path: %s", receivedPath)
	}
}

func TestTestConnection_NormalizesAuthType(t *testing.T) {
	// testConnection must normalize auth type case-insensitively so that
	// "Bearer" and "BASIC" work the same as their lowercase counterparts.
	bearerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("expected Bearer auth header, got: %q", auth)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"accountId":"test"}`)
	}))
	defer bearerServer.Close()

	err := ExportTestConnection(bearerServer.URL, "Bearer", "", "mytoken")
	if err != nil {
		t.Errorf("testConnection with 'Bearer' (uppercase) should work, got: %v", err)
	}

	basicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := r.BasicAuth()
		if !ok || user != "testuser" {
			t.Errorf("expected basic auth with user 'testuser', got ok=%v user=%q", ok, user)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"accountId":"test"}`)
	}))
	defer basicServer.Close()

	err = ExportTestConnection(basicServer.URL, "BASIC", "testuser", "mytoken")
	if err != nil {
		t.Errorf("testConnection with 'BASIC' (uppercase) should work, got: %v", err)
	}
}

func TestTestConnection_NoOAuth2Branch(t *testing.T) {
	// After removing the dead oauth2 branch, testConnection with authType="oauth2"
	// should fall through to basic auth (default case), not return a special error.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"accountId":"test"}`)
	}))
	defer ts.Close()

	err := ExportTestConnection(ts.URL, "oauth2", "user", "token")
	if err != nil {
		t.Errorf("expected testConnection to succeed with oauth2 falling through to basic, got: %v", err)
	}
}
