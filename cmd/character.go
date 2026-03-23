package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sofq/jira-cli/internal/character"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// characterStateFile returns the path to the character state file, which
// records the active character selection per profile.
func characterStateFile() string {
	if v := os.Getenv("JR_CHARACTER_BASE"); v != "" {
		return filepath.Join(v, "state.json")
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			base = filepath.Join(home, ".config")
		} else {
			base = ".config"
		}
	}
	return filepath.Join(base, "jr", "character-state.json")
}

var characterCmd = &cobra.Command{
	Use:   "character",
	Short: "Manage character profiles for agent style guidance",
}

var characterListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all character profiles",
	RunE:  runCharacterList,
}

var characterShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a character profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runCharacterShow,
}

var characterCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new character profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runCharacterCreate,
}

var characterEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Edit a character profile in $EDITOR",
	Args:  cobra.ExactArgs(1),
	RunE:  runCharacterEdit,
}

var characterDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a character profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runCharacterDelete,
}

var characterUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set the active character for the current profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runCharacterUse,
}

var characterPromptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Output the active character as an agent-consumable prompt",
	RunE:  runCharacterPrompt,
}

func init() {
	// create flags
	characterCreateCmd.Flags().String("template", "", "built-in template to use as a starting point (concise, detailed, formal, casual)")
	characterCreateCmd.Flags().String("from-user", "", "derive character from a Jira user (not yet implemented)")
	characterCreateCmd.Flags().String("persona", "", "generate character from a persona description (not yet implemented)")

	// use flags — section-level composition overrides
	characterUseCmd.Flags().String("writing", "", "override writing section with this character")
	characterUseCmd.Flags().String("workflow", "", "override workflow section with this character")
	characterUseCmd.Flags().String("interaction", "", "override interaction section with this character")

	// prompt flags
	characterPromptCmd.Flags().String("format", "prose", "output format: prose, json, or both")
	characterPromptCmd.Flags().StringSlice("section", nil, "filter to specific sections (writing, workflow, interaction, examples)")
	characterPromptCmd.Flags().Bool("redact", false, "redact PII (emails, issue keys)")

	// Register subcommands.
	characterCmd.AddCommand(characterListCmd)
	characterCmd.AddCommand(characterShowCmd)
	characterCmd.AddCommand(characterCreateCmd)
	characterCmd.AddCommand(characterEditCmd)
	characterCmd.AddCommand(characterDeleteCmd)
	characterCmd.AddCommand(characterUseCmd)
	characterCmd.AddCommand(characterPromptCmd)
}

func runCharacterList(cmd *cobra.Command, args []string) error {
	dir := character.DefaultDir()
	names, err := character.List(dir)
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "failed to list characters: " + err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}
	if names == nil {
		names = []string{}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(names); err != nil {
		return err
	}
	return nil
}

func runCharacterShow(cmd *cobra.Command, args []string) error {
	dir := character.DefaultDir()
	name := args[0]

	ch, err := character.Load(dir, name)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   fmt.Sprintf("character %q not found", name),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(ch); err != nil {
		return err
	}
	return nil
}

func runCharacterCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	templateName, _ := cmd.Flags().GetString("template")
	fromUser, _ := cmd.Flags().GetString("from-user")
	persona, _ := cmd.Flags().GetString("persona")

	// Stub: --from-user is not yet implemented.
	if fromUser != "" {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_implemented",
			Message:   "--from-user is not yet implemented",
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	// Stub: --persona is not yet implemented.
	if persona != "" {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_implemented",
			Message:   "--persona is not yet implemented",
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	var ch *character.Character

	if templateName != "" {
		t, err := character.Template(templateName)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "validation_error",
				Message:   fmt.Sprintf("unknown template %q; available: %s", templateName, strings.Join(character.TemplateNames(), ", ")),
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
		}
		ch = t
		ch.Name = name
		ch.Source = character.SourceTemplate
	} else {
		// Create a minimal scaffold for the user to fill in.
		ch = &character.Character{
			Version:     "1",
			Name:        name,
			Source:      character.SourceManual,
			Description: "",
			StyleGuide: character.StyleGuide{
				Writing:     "Describe writing style here.",
				Workflow:    "Describe workflow preferences here.",
				Interaction: "Describe interaction style here.",
			},
		}
	}

	dir := character.DefaultDir()
	if saveErr := character.Save(dir, ch); saveErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "failed to save character: " + saveErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	out, _ := marshalNoEscape(map[string]string{
		"status": "created",
		"name":   ch.Name,
	})
	fmt.Fprintf(os.Stdout, "%s\n", out)
	return nil
}

func runCharacterEdit(cmd *cobra.Command, args []string) error {
	dir := character.DefaultDir()
	name := args[0]

	if !character.Exists(dir, name) {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   fmt.Sprintf("character %q not found", name),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	charPath := filepath.Join(dir, name+".yaml")

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	editorParts := strings.Fields(editor)
	editorCmd := exec.Command(editorParts[0], append(editorParts[1:], charPath)...) // #nosec G204 -- editor comes from $EDITOR, standard Unix pattern
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if runErr := editorCmd.Run(); runErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "editor exited with error: " + runErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	// Validate the YAML after editing.
	data, readErr := os.ReadFile(charPath)
	if readErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "failed to read character file: " + readErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	var ch character.Character
	if yamlErr := yaml.Unmarshal(data, &ch); yamlErr != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   "character YAML is invalid after edit: " + yamlErr.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	if valErr := character.Validate(&ch); valErr != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   "character validation failed after edit: " + valErr.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	out, _ := marshalNoEscape(map[string]string{"status": "saved", "path": charPath})
	fmt.Fprintf(os.Stdout, "%s\n", out)
	return nil
}

func runCharacterDelete(cmd *cobra.Command, args []string) error {
	dir := character.DefaultDir()
	name := args[0]

	if err := character.Delete(dir, name); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   fmt.Sprintf("character %q not found", name),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	out, _ := marshalNoEscape(map[string]string{"status": "deleted", "name": name})
	fmt.Fprintf(os.Stdout, "%s\n", out)
	return nil
}

func runCharacterUse(cmd *cobra.Command, args []string) error {
	dir := character.DefaultDir()
	name := args[0]

	if !character.Exists(dir, name) {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   fmt.Sprintf("character %q not found", name),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	// Build section-level composition overrides from flags.
	compose := make(map[string]string)
	for _, section := range []string{"writing", "workflow", "interaction"} {
		val, _ := cmd.Flags().GetString(section)
		if val != "" {
			compose[section] = val
		}
	}
	if len(compose) == 0 {
		compose = nil
	}

	profileName, _ := cmd.Root().PersistentFlags().GetString("profile")
	if profileName == "" {
		profileName = "default"
	}

	stateFile := characterStateFile()
	if err := character.SaveState(stateFile, profileName, name, compose); err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "failed to save character state: " + err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	result := map[string]any{
		"status":  "ok",
		"profile": profileName,
		"character": name,
	}
	if compose != nil {
		result["compose"] = compose
	}
	out, _ := marshalNoEscape(result)
	fmt.Fprintf(os.Stdout, "%s\n", out)
	return nil
}

func runCharacterPrompt(cmd *cobra.Command, args []string) error {
	dir := character.DefaultDir()
	profileName, _ := cmd.Root().PersistentFlags().GetString("profile")
	if profileName == "" {
		profileName = "default"
	}

	opts := character.ResolveOptions{
		CharDir:     dir,
		ProfileName: profileName,
		StateFile:   characterStateFile(),
	}

	cc, err := character.ResolveActive(opts)
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "io_error", Message: "failed to resolve active character: " + err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}
	if cc == nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   "no active character; run `jr character use <name>`",
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	format, _ := cmd.Flags().GetString("format")
	sections, _ := cmd.Flags().GetStringSlice("section")
	redact, _ := cmd.Flags().GetBool("redact")

	promptOpts := character.PromptOptions{
		Format:   format,
		Sections: sections,
		Redact:   redact,
	}

	result := character.FormatPrompt(cc, promptOpts)
	fmt.Fprint(os.Stdout, result)
	return nil
}
