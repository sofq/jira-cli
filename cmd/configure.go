package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Save connection settings to the config file (flag-driven, no prompts)",
	RunE:  runConfigure,
}

func init() {
	f := configureCmd.Flags()
	f.String("base-url", "", "Jira base URL (required unless --delete)")
	f.String("token", "", "API token or bearer token (required unless --delete)")
	f.String("profile", "default", "profile name to save settings under")
	f.String("auth-type", "basic", "auth type: basic or bearer")
	f.String("username", "", "username for basic auth")
	f.Bool("test", false, "test connection via GET /rest/api/3/myself before saving")
	f.Bool("delete", false, "delete the named profile")
}

func runConfigure(cmd *cobra.Command, args []string) error {
	baseURL, _ := cmd.Flags().GetString("base-url")
	token, _ := cmd.Flags().GetString("token")
	profileName, _ := cmd.Flags().GetString("profile")
	authType, _ := cmd.Flags().GetString("auth-type")
	username, _ := cmd.Flags().GetString("username")
	testConn, _ := cmd.Flags().GetBool("test")
	deleteProfile, _ := cmd.Flags().GetBool("delete")

	if deleteProfile {
		// Require explicit --profile when deleting to prevent accidental
		// deletion of the default profile.
		if !cmd.Flags().Changed("profile") {
			apiErr := &jrerrors.APIError{
				ErrorType: "validation_error",
				Message:   "--profile is required when using --delete (to prevent accidental deletion of the default profile)",
			}
			apiErr.WriteJSON(os.Stderr)
			return &errAlreadyWritten{code: jrerrors.ExitValidation}
		}
		return deleteProfileByName(cmd, profileName)
	}

	// Validate profile name is not empty/whitespace.
	if strings.TrimSpace(profileName) == "" {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   "--profile must not be empty or whitespace-only",
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitValidation}
	}

	// Test-only mode: when --test is set but no --base-url/--token are provided,
	// load the existing profile and test its saved credentials.
	testOnly := testConn && !cmd.Flags().Changed("base-url") && !cmd.Flags().Changed("token")
	if testOnly {
		return testExistingProfile(cmd, profileName)
	}

	// Validate required fields are not empty/whitespace.
	if strings.TrimSpace(baseURL) == "" {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   "--base-url must not be empty",
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitValidation}
	}
	if strings.TrimSpace(token) == "" {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   "--token must not be empty",
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitValidation}
	}

	if testConn {
		if err := testConnection(baseURL, authType, username, token); err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "connection_error",
				Status:    0,
				Message:   "connection test failed: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &errAlreadyWritten{code: jrerrors.ExitError}
		}
	}

	configPath := config.DefaultPath()
	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "config_error",
			Status:    0,
			Message:   "failed to load config: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitError}
	}

	cfg.Profiles[profileName] = config.Profile{
		BaseURL: baseURL,
		Auth: config.AuthConfig{
			Type:     authType,
			Username: username,
			Token:    token,
		},
	}

	if cfg.DefaultProfile == "" {
		cfg.DefaultProfile = profileName
	}

	if err := config.SaveTo(cfg, configPath); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "config_error",
			Status:    0,
			Message:   "failed to save config: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitError}
	}

	out, _ := json.Marshal(map[string]string{
		"status":  "saved",
		"profile": profileName,
		"path":    configPath,
	})
	return schemaOutput(cmd, out)
}

// testExistingProfile loads a saved profile and tests its connection.
func testExistingProfile(cmd *cobra.Command, profileName string) error {
	configPath := config.DefaultPath()
	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "config_error",
			Status:    0,
			Message:   "failed to load config: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitError}
	}

	// Resolve profile name: use default if not explicitly set.
	name := profileName
	if name == "default" {
		if cfg.DefaultProfile != "" {
			name = cfg.DefaultProfile
		}
	}

	profile, ok := cfg.Profiles[name]
	if !ok {
		availableNames := make([]string, 0, len(cfg.Profiles))
		for k := range cfg.Profiles {
			availableNames = append(availableNames, k)
		}
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   fmt.Sprintf("profile %q not found; available profiles: %s", name, strings.Join(availableNames, ", ")),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitNotFound}
	}

	if strings.TrimSpace(profile.BaseURL) == "" {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   fmt.Sprintf("profile %q has no base_url configured", name),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitValidation}
	}

	if err := testConnection(profile.BaseURL, profile.Auth.Type, profile.Auth.Username, profile.Auth.Token); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Status:    0,
			Message:   "connection test failed: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitError}
	}

	out, _ := json.Marshal(map[string]string{
		"status":  "ok",
		"profile": name,
	})
	return schemaOutput(cmd, out)
}

// deleteProfileByName removes a profile from the config file.
func deleteProfileByName(cmd *cobra.Command, name string) error {
	configPath := config.DefaultPath()
	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "config_error",
			Message:   "failed to load config: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitError}
	}

	if _, ok := cfg.Profiles[name]; !ok {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   fmt.Sprintf("profile %q not found", name),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitNotFound}
	}

	delete(cfg.Profiles, name)
	if cfg.DefaultProfile == name {
		cfg.DefaultProfile = ""
	}

	if err := config.SaveTo(cfg, configPath); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "config_error",
			Message:   "failed to save config: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitError}
	}

	out, _ := json.Marshal(map[string]string{
		"status":  "deleted",
		"profile": name,
		"path":    configPath,
	})
	return schemaOutput(cmd, out)
}

// testConnection performs a GET /rest/api/3/myself against baseURL to verify credentials.
func testConnection(baseURL, authType, username, token string) error {
	url := baseURL + "/rest/api/3/myself"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	switch authType {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+token)
	default: // basic
		req.SetBasicAuth(username, token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	return nil
}
