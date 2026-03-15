package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

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
	f.String("base-url", "", "Jira base URL (required)")
	f.String("token", "", "API token or bearer token (required)")
	f.String("profile", "default", "profile name to save settings under")
	f.String("auth-type", "basic", "auth type: basic or bearer")
	f.String("username", "", "username for basic auth")
	f.Bool("test", false, "test connection via GET /rest/api/3/myself before saving")

	_ = configureCmd.MarkFlagRequired("base-url")
	_ = configureCmd.MarkFlagRequired("token")
}

func runConfigure(cmd *cobra.Command, args []string) error {
	baseURL, _ := cmd.Flags().GetString("base-url")
	token, _ := cmd.Flags().GetString("token")
	profileName, _ := cmd.Flags().GetString("profile")
	authType, _ := cmd.Flags().GetString("auth-type")
	username, _ := cmd.Flags().GetString("username")
	testConn, _ := cmd.Flags().GetBool("test")

	if testConn {
		if err := testConnection(baseURL, authType, username, token); err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "connection_error",
				Status:    0,
				Message:   "connection test failed: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return err
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
		return err
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
		return err
	}

	out, _ := json.Marshal(map[string]string{
		"status":  "saved",
		"profile": profileName,
		"path":    configPath,
	})
	fmt.Fprintf(os.Stdout, "%s\n", out)
	return nil
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
