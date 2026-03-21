package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sofq/jira-cli/internal/config"
	"github.com/spf13/cobra"
)

// DoctorCheck holds the result of a single diagnostic check.
type DoctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "pass", "fail", or "warn"
	Message string `json:"message"`
}

// doctorResult is the top-level JSON output for `jr doctor`.
type doctorResult struct {
	Status string        `json:"status"` // "healthy" or "unhealthy"
	Checks []DoctorCheck `json:"checks"`
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check configuration and connectivity, output a JSON health report",
	RunE:  runDoctor,
}

func init() {
	f := doctorCmd.Flags()
	f.StringP("profile", "p", "", "profile to check (defaults to the default profile)")
	f.String("jq", "", "jq filter expression to apply to the response")
	f.Bool("pretty", false, "pretty-print JSON output")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	profileName, _ := cmd.Flags().GetString("profile")

	var checks []DoctorCheck

	// --- check 1: config file exists ---
	configPath := config.DefaultPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		checks = append(checks, DoctorCheck{
			Name:    "config_file",
			Status:  "fail",
			Message: fmt.Sprintf("config file not found at %s; run `jr configure` to create one", configPath),
		})
		return writeDoctorResult(cmd, checks)
	}
	checks = append(checks, DoctorCheck{
		Name:    "config_file",
		Status:  "pass",
		Message: configPath,
	})

	// --- check 2: profile loads ---
	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		checks = append(checks, DoctorCheck{
			Name:    "profile_load",
			Status:  "fail",
			Message: "failed to parse config file: " + err.Error(),
		})
		return writeDoctorResult(cmd, checks)
	}

	// Resolve which profile name to use.
	resolvedName := profileName
	if resolvedName == "" {
		resolvedName = cfg.DefaultProfile
	}
	if resolvedName == "" {
		resolvedName = "default"
	}

	profile, ok := cfg.Profiles[resolvedName]
	if !ok {
		checks = append(checks, DoctorCheck{
			Name:    "profile_load",
			Status:  "fail",
			Message: fmt.Sprintf("profile %q not found in config file", resolvedName),
		})
		return writeDoctorResult(cmd, checks)
	}
	checks = append(checks, DoctorCheck{
		Name:    "profile_load",
		Status:  "pass",
		Message: fmt.Sprintf("profile %q loaded", resolvedName),
	})

	// --- check 3: base_url set ---
	if strings.TrimSpace(profile.BaseURL) == "" {
		checks = append(checks, DoctorCheck{
			Name:    "base_url",
			Status:  "fail",
			Message: fmt.Sprintf("profile %q has no base_url configured; run `jr configure --base-url <url>`", resolvedName),
		})
		return writeDoctorResult(cmd, checks)
	}
	checks = append(checks, DoctorCheck{
		Name:    "base_url",
		Status:  "pass",
		Message: profile.BaseURL,
	})

	// --- check 4: auth configured ---
	authType := strings.ToLower(profile.Auth.Type)
	if authType == "" {
		authType = "basic"
	}
	switch authType {
	case "bearer":
		if strings.TrimSpace(profile.Auth.Token) == "" {
			checks = append(checks, DoctorCheck{
				Name:    "auth_configured",
				Status:  "fail",
				Message: "auth type is bearer but no token is set",
			})
			return writeDoctorResult(cmd, checks)
		}
	case "oauth2":
		var missing []string
		if profile.Auth.ClientID == "" {
			missing = append(missing, "client_id")
		}
		if profile.Auth.ClientSecret == "" {
			missing = append(missing, "client_secret")
		}
		if profile.Auth.TokenURL == "" {
			missing = append(missing, "token_url")
		}
		if len(missing) > 0 {
			checks = append(checks, DoctorCheck{
				Name:    "auth_configured",
				Status:  "fail",
				Message: fmt.Sprintf("auth type is oauth2 but missing required fields: %s", strings.Join(missing, ", ")),
			})
			return writeDoctorResult(cmd, checks)
		}
	default: // basic
		if strings.TrimSpace(profile.Auth.Token) == "" {
			checks = append(checks, DoctorCheck{
				Name:    "auth_configured",
				Status:  "warn",
				Message: "auth type is basic but no token is set",
			})
		}
	}
	if checks[len(checks)-1].Name != "auth_configured" {
		checks = append(checks, DoctorCheck{
			Name:    "auth_configured",
			Status:  "pass",
			Message: fmt.Sprintf("auth type: %s", authType),
		})
	}

	// --- check 5: connectivity ---
	if err := testConnection(profile.BaseURL, authType, profile.Auth.Username, profile.Auth.Token); err != nil {
		checks = append(checks, DoctorCheck{
			Name:    "connectivity",
			Status:  "fail",
			Message: "GET /rest/api/3/myself failed: " + err.Error(),
		})
		return writeDoctorResult(cmd, checks)
	}
	checks = append(checks, DoctorCheck{
		Name:    "connectivity",
		Status:  "pass",
		Message: "GET /rest/api/3/myself succeeded",
	})

	return writeDoctorResult(cmd, checks)
}

// writeDoctorResult encodes and writes the doctor result JSON to stdout.
// Overall status is "healthy" only when all checks pass; any fail or warn
// makes it "unhealthy".
func writeDoctorResult(cmd *cobra.Command, checks []DoctorCheck) error {
	overall := "healthy"
	for _, c := range checks {
		if c.Status == "fail" || c.Status == "warn" {
			overall = "unhealthy"
			break
		}
	}

	result := doctorResult{
		Status: overall,
		Checks: checks,
	}

	data, err := marshalNoEscape(result)
	if err != nil {
		// Should not happen for a well-typed struct, but handle gracefully.
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(result)
		return nil
	}

	return schemaOutput(cmd, data)
}
