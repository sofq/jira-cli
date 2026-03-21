package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/sofq/jira-cli/internal/config"
	"github.com/spf13/cobra"
)

// newDoctorCmd returns a fresh cobra.Command wired to runDoctor with all flags
// that the real doctorCmd registers. Using a local command avoids mutating the
// global doctorCmd state between tests.
func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "doctor", RunE: runDoctor}
	f := cmd.Flags()
	f.StringP("profile", "p", "", "profile to check")
	f.String("jq", "", "")
	f.Bool("pretty", false, "")
	return cmd
}

// parseDoctorResult unmarshals JSON string into a doctorResult.
func parseDoctorResult(t *testing.T, raw string) doctorResult {
	t.Helper()
	var result doctorResult
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	if err := dec.Decode(&result); err != nil {
		t.Fatalf("unmarshal doctor result: %v\nraw: %s", err, raw)
	}
	return result
}

// findCheck returns the DoctorCheck with the given name, or fails the test.
func findCheck(t *testing.T, checks []DoctorCheck, name string) DoctorCheck {
	t.Helper()
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("check %q not found in result", name)
	return DoctorCheck{}
}

// TestDoctorHealthy verifies that all checks pass when the config is valid and
// the mock server returns 200 OK for /rest/api/3/myself.
func TestDoctorHealthy(t *testing.T) {
	// Start a mock Jira server that accepts requests to /rest/api/3/myself.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/myself" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"accountId":"user1","displayName":"Test User"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: srv.URL,
				Auth: config.AuthConfig{
					Type:     "basic",
					Username: "user@example.com",
					Token:    "test-token",
				},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newDoctorCmd()

	var runErr error
	output := captureStdout(t, func() {
		runErr = runDoctor(cmd, nil)
	})

	if runErr != nil {
		t.Fatalf("runDoctor returned error: %v", runErr)
	}

	result := parseDoctorResult(t, output)

	if result.Status != "healthy" {
		t.Errorf("expected status=healthy, got %q", result.Status)
	}

	expectedChecks := []string{"config_file", "profile_load", "base_url", "auth_configured", "connectivity"}
	for _, name := range expectedChecks {
		c := findCheck(t, result.Checks, name)
		if c.Status != "pass" {
			t.Errorf("check %q: expected pass, got %q (message: %s)", name, c.Status, c.Message)
		}
	}
}

// TestDoctorUnhealthy_NoConfigFile verifies that the command reports a fail on
// config_file when no config file exists and stops after that check.
func TestDoctorUnhealthy_NoConfigFile(t *testing.T) {
	// Point JR_CONFIG_PATH at a path that does not exist.
	t.Setenv("JR_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent", "config.json"))

	cmd := newDoctorCmd()

	var runErr error
	output := captureStdout(t, func() {
		runErr = runDoctor(cmd, nil)
	})

	if runErr != nil {
		t.Fatalf("runDoctor returned error: %v", runErr)
	}

	result := parseDoctorResult(t, output)

	if result.Status != "unhealthy" {
		t.Errorf("expected status=unhealthy, got %q", result.Status)
	}

	if len(result.Checks) != 1 {
		t.Errorf("expected exactly 1 check, got %d", len(result.Checks))
	}

	c := findCheck(t, result.Checks, "config_file")
	if c.Status != "fail" {
		t.Errorf("config_file check: expected fail, got %q", c.Status)
	}
}

// TestDoctorUnhealthy_AuthFailure verifies that the connectivity check reports
// fail when the server returns 401.
func TestDoctorUnhealthy_AuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessages":["unauthorized"]}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				BaseURL: srv.URL,
				Auth: config.AuthConfig{
					Type:     "basic",
					Username: "user@example.com",
					Token:    "bad-token",
				},
			},
		},
	}
	if err := config.SaveTo(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JR_CONFIG_PATH", configPath)

	cmd := newDoctorCmd()

	var runErr error
	output := captureStdout(t, func() {
		runErr = runDoctor(cmd, nil)
	})

	if runErr != nil {
		t.Fatalf("runDoctor returned error: %v", runErr)
	}

	result := parseDoctorResult(t, output)

	if result.Status != "unhealthy" {
		t.Errorf("expected status=unhealthy, got %q", result.Status)
	}

	c := findCheck(t, result.Checks, "connectivity")
	if c.Status != "fail" {
		t.Errorf("connectivity check: expected fail, got %q", c.Status)
	}
}
