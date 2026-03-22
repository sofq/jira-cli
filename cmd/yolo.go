package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/sofq/jira-cli/internal/yolo"
	"github.com/spf13/cobra"
)

var yoloCmd = &cobra.Command{
	Use:   "yolo",
	Short: "Yolo execution policy status and history",
}

var yoloStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current yolo policy status",
	RunE:  runYoloStatus,
}

var yoloHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Show yolo action history",
	RunE:  runYoloHistory,
}

func init() {
	yoloCmd.AddCommand(yoloStatusCmd)
	yoloCmd.AddCommand(yoloHistoryCmd)
}

func runYoloStatus(cmd *cobra.Command, args []string) error {
	profileName, _ := cmd.Root().PersistentFlags().GetString("profile")

	// Load config to detect the active profile name when not supplied by flag.
	// We use the default config values for yolo since the profile struct does
	// not yet carry a yolo section; a future task will add that field.
	cfg, cfgErr := config.LoadFrom(config.DefaultPath())
	if cfgErr == nil && profileName == "" {
		profileName = cfg.DefaultProfile
	}
	if profileName == "" {
		profileName = "default"
	}

	yloCfg := yolo.DefaultConfig()

	// Build a rate limiter to get the current remaining tokens.
	rl := yolo.NewRateLimiter(yloCfg.RateLimit, profileName, "")
	remaining := rl.Remaining()

	result := map[string]any{
		"enabled":   yloCfg.Enabled,
		"scope":     yloCfg.Scope,
		"character": yloCfg.Character,
		"rate": map[string]any{
			"per_hour":  yloCfg.RateLimit.PerHour,
			"burst":     yloCfg.RateLimit.Burst,
			"remaining": remaining,
		},
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if encErr := enc.Encode(result); encErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "failed to encode status: " + encErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}
	return nil
}

func runYoloHistory(cmd *cobra.Command, args []string) error {
	// TODO: filter audit log entries where yolo=true and return them here.
	// For now, return an empty array as a stub.
	fmt.Fprintf(os.Stdout, "[]\n")
	return nil
}
