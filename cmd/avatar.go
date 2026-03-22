package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sofq/jira-cli/internal/avatar"
	"github.com/sofq/jira-cli/internal/character"
	"github.com/sofq/jira-cli/internal/client"
	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// avatarNeedsClient lists the subcommand names that require an authenticated
// Jira client. All others are local-only operations.
var avatarNeedsClient = map[string]bool{
	"extract": true,
	"build":   true,
	"refresh": true,
}

var avatarCmd = &cobra.Command{
	Use:   "avatar",
	Short: "User style profiling for AI agents",
	// Override the root PersistentPreRunE: only inject the client for
	// subcommands that actually need it.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if !avatarNeedsClient[cmd.Name()] {
			return nil
		}
		// Delegate to the root PersistentPreRunE logic.
		return rootCmd.PersistentPreRunE(cmd, args)
	},
}

// avatarExtractCmd runs the extraction pipeline.
var avatarExtractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract Jira activity data for the avatar profile",
	RunE:  runAvatarExtract,
}

// avatarBuildCmd builds a profile from an extraction.
var avatarBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build an avatar profile from extracted data",
	RunE:  runAvatarBuild,
}

// avatarPromptCmd outputs the profile for agent consumption.
var avatarPromptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Output avatar profile as agent-consumable prompt text",
	RunE:  runAvatarPrompt,
}

// avatarShowCmd outputs the profile as JSON.
var avatarShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show avatar profile as JSON",
	RunE:  runAvatarShow,
}

// avatarEditCmd opens the profile in $EDITOR.
var avatarEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open avatar profile in $EDITOR",
	RunE:  runAvatarEdit,
}

// avatarRefreshCmd is a shortcut for extract + build.
var avatarRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Re-extract data and rebuild the avatar profile",
	RunE:  runAvatarRefresh,
}

// avatarStatusCmd checks the profile state.
var avatarStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current status of the avatar profile",
	RunE:  runAvatarStatus,
}

func init() {
	// extract flags
	avatarExtractCmd.Flags().String("user", "", "Jira user to analyse (empty = authenticated user)")
	avatarExtractCmd.Flags().Int("min-comments", 50, "minimum number of comments to collect")
	avatarExtractCmd.Flags().Int("min-updates", 30, "minimum number of updated issues to collect")
	avatarExtractCmd.Flags().String("max-window", "6m", "maximum lookback window (e.g. 6m, 2w)")

	// build flags
	avatarBuildCmd.Flags().String("user", "", "Jira user (empty = authenticated user; used to locate avatar dir)")
	avatarBuildCmd.Flags().String("engine", "", "profile engine to use (local or llm; default: from config)")
	avatarBuildCmd.Flags().String("llm-cmd", "", "command to run for LLM engine")
	avatarBuildCmd.Flags().Bool("yes", false, "suppress confirmation prompts")

	// refresh flags (same as extract + build)
	avatarRefreshCmd.Flags().String("user", "", "Jira user to analyse (empty = authenticated user)")
	avatarRefreshCmd.Flags().Int("min-comments", 50, "minimum number of comments to collect")
	avatarRefreshCmd.Flags().Int("min-updates", 30, "minimum number of updated issues to collect")
	avatarRefreshCmd.Flags().String("max-window", "6m", "maximum lookback window (e.g. 6m, 2w)")
	avatarRefreshCmd.Flags().String("engine", "", "profile engine to use (local or llm; default: from config)")
	avatarRefreshCmd.Flags().String("llm-cmd", "", "command to run for LLM engine")
	avatarRefreshCmd.Flags().Bool("yes", false, "suppress confirmation prompts")

	// prompt flags
	avatarPromptCmd.Flags().String("format", "prose", "output format: prose, json, or both")
	avatarPromptCmd.Flags().StringSlice("section", nil, "filter to specific sections (writing, workflow, interaction)")
	avatarPromptCmd.Flags().Bool("redact", false, "redact PII (emails, issue keys)")

	// Register subcommands.
	avatarCmd.AddCommand(avatarExtractCmd)
	avatarCmd.AddCommand(avatarBuildCmd)
	avatarCmd.AddCommand(avatarPromptCmd)
	avatarCmd.AddCommand(avatarShowCmd)
	avatarCmd.AddCommand(avatarEditCmd)
	avatarCmd.AddCommand(avatarRefreshCmd)
	avatarCmd.AddCommand(avatarStatusCmd)
}

// resolveAvatarDir resolves the avatar directory for the given user flag.
// It uses the client to resolve the user's accountID to generate the hash.
func resolveAvatarDir(c *client.Client, userFlag string) (string, string, error) {
	user, err := avatar.ResolveUser(c, userFlag)
	if err != nil {
		return "", "", err
	}
	hash := avatar.UserHash(user.AccountID)
	dir := avatar.AvatarDir(hash)
	return dir, user.AccountID, nil
}

// loadAvatarConfig loads the avatar config from the config file for the given profile.
func loadAvatarConfig(profileName string) (*config.AvatarConfig, error) {
	cfg, err := config.LoadFrom(config.DefaultPath())
	if err != nil {
		return nil, err
	}
	name := profileName
	if name == "" {
		name = cfg.DefaultProfile
	}
	if name == "" {
		name = "default"
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return nil, nil
	}
	return p.Avatar, nil
}

func runAvatarExtract(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	userFlag, _ := cmd.Flags().GetString("user")
	minComments, _ := cmd.Flags().GetInt("min-comments")
	minUpdates, _ := cmd.Flags().GetInt("min-updates")
	maxWindow, _ := cmd.Flags().GetString("max-window")

	opts := avatar.ExtractOptions{
		UserFlag:    userFlag,
		MinComments: minComments,
		MinUpdates:  minUpdates,
		MaxWindow:   maxWindow,
	}

	extraction, extractErr := avatar.Extract(c, opts)
	if extractErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "extraction_error", Message: extractErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	// Resolve avatar directory using the account ID from the extraction meta.
	hash := avatar.UserHash(extraction.Meta.User)
	dir := avatar.AvatarDir(hash)
	if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "failed to create avatar dir: " + mkErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	if saveErr := avatar.SaveExtraction(dir, extraction); saveErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "failed to save extraction: " + saveErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	out, _ := json.Marshal(extraction)
	fmt.Fprintf(os.Stdout, "%s\n", out)
	return nil
}

func runAvatarBuild(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	userFlag, _ := cmd.Flags().GetString("user")
	engine, _ := cmd.Flags().GetString("engine")
	llmCmd, _ := cmd.Flags().GetString("llm-cmd")
	yes, _ := cmd.Flags().GetBool("yes")

	// Resolve avatar dir using the client.
	dir, _, resolveErr := resolveAvatarDir(c, userFlag)
	if resolveErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: "failed to resolve user: " + resolveErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	extraction, loadErr := avatar.LoadExtraction(dir)
	if loadErr != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   "no extraction found; run `jr avatar extract` first",
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	// Load avatar config from profile (--profile is a persistent flag on rootCmd).
	profileName, _ := cmd.Root().PersistentFlags().GetString("profile")
	avatarCfg, _ := loadAvatarConfig(profileName)

	buildOpts := avatar.BuildOptions{
		Engine: engine,
		LLMCmd: llmCmd,
		Yes:    yes,
		Stderr: os.Stderr,
	}

	profile, buildErr := avatar.Build(extraction, avatarCfg, buildOpts)
	if buildErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "build_error", Message: buildErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "failed to create avatar dir: " + mkErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	if saveErr := avatar.SaveProfile(dir, profile); saveErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "failed to save profile: " + saveErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	// Convert profile to character and save it locally (non-fatal on failure).
	ch := character.FromProfile(profile)
	charDir := character.DefaultDir()
	// If a character with this name already exists but has a different source,
	// append "-avatar" to avoid overwriting user-managed characters.
	if character.Exists(charDir, ch.Name) {
		existing, loadErr := character.Load(charDir, ch.Name)
		if loadErr == nil && existing.Source != character.SourceAvatar {
			ch.Name = ch.Name + "-avatar"
		}
	}
	if charSaveErr := character.Save(charDir, ch); charSaveErr != nil {
		warnMsg := map[string]string{
			"type":    "warning",
			"message": "failed to save character file: " + charSaveErr.Error(),
		}
		warnJSON, _ := json.Marshal(warnMsg)
		fmt.Fprintf(os.Stderr, "%s\n", warnJSON)
	}

	out, _ := json.Marshal(profile)
	fmt.Fprintf(os.Stdout, "%s\n", out)
	return nil
}

func runAvatarPrompt(cmd *cobra.Command, args []string) error {
	dir, err := resolveAvatarDirFromDisk()
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "not_found", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	if !avatar.ProfileExists(dir) {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   "no avatar profile found; run `jr avatar extract` then `jr avatar build`",
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	profile, loadErr := avatar.LoadProfile(dir)
	if loadErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "failed to load profile: " + loadErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	format, _ := cmd.Flags().GetString("format")
	sections, _ := cmd.Flags().GetStringSlice("section")
	redact, _ := cmd.Flags().GetBool("redact")

	opts := avatar.PromptOptions{
		Format:   format,
		Sections: sections,
		Redact:   redact,
	}

	result := avatar.FormatPrompt(profile, opts)
	fmt.Fprint(os.Stdout, result)
	return nil
}

func runAvatarShow(cmd *cobra.Command, args []string) error {
	dir, err := resolveAvatarDirFromDisk()
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "not_found", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	if !avatar.ProfileExists(dir) {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   "no avatar profile found; run `jr avatar extract` then `jr avatar build`",
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	profile, loadErr := avatar.LoadProfile(dir)
	if loadErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "failed to load profile: " + loadErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	out, _ := json.MarshalIndent(profile, "", "  ")
	fmt.Fprintf(os.Stdout, "%s\n", out)
	return nil
}

func runAvatarEdit(cmd *cobra.Command, args []string) error {
	dir, err := resolveAvatarDirFromDisk()
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "not_found", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	if !avatar.ProfileExists(dir) {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   "no avatar profile found; run `jr avatar extract` then `jr avatar build`",
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	profilePath := filepath.Join(dir, "profile.yaml")

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Split editor string to handle values like "code --wait" or "vim -p".
	editorParts := strings.Fields(editor)
	editorCmd := exec.Command(editorParts[0], append(editorParts[1:], profilePath)...) // #nosec G204 G702 -- editor comes from $EDITOR, standard Unix pattern
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if runErr := editorCmd.Run(); runErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "editor exited with error: " + runErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	// Validate the YAML after editing.
	data, readErr := os.ReadFile(profilePath)
	if readErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "failed to read profile: " + readErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	var p avatar.Profile
	if yamlErr := yaml.Unmarshal(data, &p); yamlErr != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   "profile YAML is invalid after edit: " + yamlErr.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	out, _ := marshalNoEscape(map[string]string{"status": "saved", "path": profilePath})
	fmt.Fprintf(os.Stdout, "%s\n", out)
	return nil
}

func runAvatarRefresh(cmd *cobra.Command, args []string) error {
	if err := runAvatarExtract(cmd, args); err != nil {
		return err
	}
	return runAvatarBuild(cmd, args)
}

func runAvatarStatus(cmd *cobra.Command, args []string) error {
	dir, err := resolveAvatarDirFromDisk()
	if err != nil || !avatar.ProfileExists(dir) {
		fmt.Fprintf(os.Stdout, "{\"exists\":false}\n")
		return nil
	}

	profile, loadErr := avatar.LoadProfile(dir)
	if loadErr != nil {
		fmt.Fprintf(os.Stdout, "{\"exists\":false}\n")
		return nil
	}

	ageDays, _ := avatar.ProfileAgeDays(dir)

	dp := avatar.DataPoints{}
	if avatar.ExtractionExists(dir) {
		if ext, extErr := avatar.LoadExtraction(dir); extErr == nil {
			dp = ext.Meta.DataPoints
		}
	}

	const staleDays = 30
	stale := ageDays >= staleDays

	result := map[string]any{
		"exists":       true,
		"user":         profile.User,
		"display_name": profile.DisplayName,
		"generated_at": profile.GeneratedAt,
		"age_days":     ageDays,
		"engine":       profile.Engine,
		"data_points":  dp,
		"stale":        stale,
	}

	out, _ := marshalNoEscape(result)
	fmt.Fprintf(os.Stdout, "%s\n", out)
	return nil
}

// resolveAvatarDirFromDisk finds the avatar directory for the current user by
// scanning the avatars base directory. If exactly one user directory exists,
// it is used. With multiple, the most recently modified profile is picked.
//
// This is used by commands that don't have a client (prompt, show, edit, status).
//
// If the JR_AVATAR_BASE environment variable is set, it is used as the base
// directory instead of the OS-default location. This allows tests to redirect
// avatar storage to a temporary directory.
func resolveAvatarDirFromDisk() (string, error) {
	var base string
	if override := os.Getenv("JR_AVATAR_BASE"); override != "" {
		base = override
	} else {
		cfgDir, err := os.UserConfigDir()
		if err != nil {
			home, _ := os.UserHomeDir()
			cfgDir = filepath.Join(home, ".config")
		}
		base = filepath.Join(cfgDir, "jr", "avatars")
	}

	entries, readErr := os.ReadDir(base)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return "", fmt.Errorf("no avatar data found; run `jr avatar extract` first")
		}
		return "", fmt.Errorf("failed to read avatar directory: %w", readErr)
	}

	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(base, e.Name()))
		}
	}

	if len(dirs) == 0 {
		return "", fmt.Errorf("no avatar data found; run `jr avatar extract` first")
	}

	if len(dirs) == 1 {
		return dirs[0], nil
	}

	// Multiple users: pick the one with the most recently modified profile.
	best := dirs[0]
	bestTime := time.Time{}
	for _, d := range dirs {
		info, statErr := os.Stat(filepath.Join(d, "profile.yaml")) // #nosec G703 -- d comes from os.ReadDir on controlled base path
		if statErr != nil {
			continue
		}
		if info.ModTime().After(bestTime) {
			bestTime = info.ModTime()
			best = d
		}
	}
	return best, nil
}
