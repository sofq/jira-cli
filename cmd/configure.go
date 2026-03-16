package cmd

import (
	"fmt"
	"net/http"
	"os"
	"sort"
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
	f.StringP("profile", "p", "default", "profile name to save settings under")
	f.String("auth-type", "basic", "auth type: basic, bearer, or oauth2")
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
		return testExistingProfile(cmd, profileName, cmd.Flags().Changed("profile"))
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

	// Validate auth-type before saving or testing.
	validAuthTypes := map[string]bool{"basic": true, "bearer": true, "oauth2": true}
	if !validAuthTypes[strings.ToLower(authType)] {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   fmt.Sprintf("invalid --auth-type %q; must be one of: basic, bearer, oauth2", authType),
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitValidation}
	}
	authType = strings.ToLower(authType)

	// Reject oauth2 in configure: required fields (client_id, client_secret,
	// token_url) cannot be set via CLI flags, so saving an oauth2 profile here
	// would always produce an incomplete config that fails at runtime.
	if authType == "oauth2" {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   "--auth-type oauth2 is not supported by the configure command; oauth2 profiles require client_id, client_secret, and token_url which must be set manually in the config file (" + config.DefaultPath() + ")",
		}
		apiErr.WriteJSON(os.Stderr)
		return &errAlreadyWritten{code: jrerrors.ExitValidation}
	}

	// Normalize base URL: strip trailing slashes to avoid double-slash issues.
	baseURL = strings.TrimRight(baseURL, "/")

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

	out, _ := marshalNoEscape(map[string]string{
		"status":  "saved",
		"profile": profileName,
		"path":    configPath,
	})
	return schemaOutput(cmd, out)
}

// testExistingProfile loads a saved profile and tests its connection.
// profileExplicit indicates whether --profile was explicitly passed by the user.
func testExistingProfile(cmd *cobra.Command, profileName string, profileExplicit bool) error {
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

	// Resolve profile name: use default_profile only if --profile was not
	// explicitly set (i.e. we're using the flag's default value of "default").
	name := profileName
	if !profileExplicit && name == "default" {
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
		sort.Strings(availableNames)
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

	out, _ := marshalNoEscape(map[string]string{
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
		availableNames := make([]string, 0, len(cfg.Profiles))
		for k := range cfg.Profiles {
			availableNames = append(availableNames, k)
		}
		sort.Strings(availableNames)
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   fmt.Sprintf("profile %q not found; available profiles: %s", name, strings.Join(availableNames, ", ")),
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

	out, _ := marshalNoEscape(map[string]string{
		"status":  "deleted",
		"profile": name,
		"path":    configPath,
	})
	return schemaOutput(cmd, out)
}

// testConnection performs a GET /rest/api/3/myself against baseURL to verify credentials.
func testConnection(baseURL, authType, username, token string) error {
	baseURL = strings.TrimRight(baseURL, "/")
	testURL := baseURL + "/rest/api/3/myself"
	req, err := http.NewRequest("GET", testURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	switch strings.ToLower(authType) {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+token)
	case "oauth2":
		return fmt.Errorf("oauth2 auth type cannot be tested with --test; configure and verify manually")
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
