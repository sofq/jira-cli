package character

import (
	"encoding/json"
	"fmt"
	"os"
)

// ResolveOptions carries all the inputs needed to determine which character
// is active for the current invocation.
type ResolveOptions struct {
	// FlagCharacter is the --character flag value (highest priority).
	FlagCharacter string

	// FlagCompose is the --compose flag value: section -> character name.
	FlagCompose map[string]string

	// ConfigChar is the character name from the profile config file.
	ConfigChar string

	// ConfigCompose is the compose map from the profile config file.
	ConfigCompose map[string]string

	// CharDir is the directory that holds character YAML files.
	CharDir string

	// ProfileName identifies the active profile, used to look up state.
	ProfileName string

	// StateFile is the path to the JSON state file.
	StateFile string
}

// stateEntry is the per-profile entry persisted in the state file.
type stateEntry struct {
	Base    string            `json:"base"`
	Compose map[string]string `json:"compose,omitempty"`
}

// ResolveActive determines the active ComposedCharacter following the
// priority order:
//
//  1. FlagCharacter (+ FlagCompose)
//  2. ConfigChar (+ ConfigCompose)
//  3. State file entry for ProfileName
//  4. First character with source == "avatar"
//  5. nil (no character configured)
func ResolveActive(opts ResolveOptions) (*ComposedCharacter, error) {
	// Priority 1: explicit flag.
	if opts.FlagCharacter != "" {
		return loadComposed(opts.CharDir, opts.FlagCharacter, opts.FlagCompose)
	}

	// Priority 2: config file.
	if opts.ConfigChar != "" {
		return loadComposed(opts.CharDir, opts.ConfigChar, opts.ConfigCompose)
	}

	// Priority 3: state file.
	if opts.StateFile != "" && opts.ProfileName != "" {
		entry, err := readStateEntry(opts.StateFile, opts.ProfileName)
		if err == nil && entry != nil {
			return loadComposed(opts.CharDir, entry.Base, entry.Compose)
		}
	}

	// Priority 4: any character whose source is "avatar".
	names, err := List(opts.CharDir)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		c, err := Load(opts.CharDir, name)
		if err != nil {
			continue
		}
		if c.Source == SourceAvatar {
			cc := Compose(c, nil)
			return &cc, nil
		}
	}

	// Priority 5: nothing found.
	return nil, nil
}

// SaveState persists the active character selection for profileName into
// stateFile. It reads the existing state (if any) and merges the new entry
// so that other profiles are preserved.
func SaveState(stateFile, profileName, baseName string, compose map[string]string) error {
	state := make(map[string]stateEntry)

	// Load existing state if present.
	if data, err := os.ReadFile(stateFile); err == nil {
		_ = json.Unmarshal(data, &state) // ignore parse errors; start fresh
	}

	state[profileName] = stateEntry{Base: baseName, Compose: compose}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("character: marshal state: %w", err)
	}
	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
		return fmt.Errorf("character: write state %s: %w", stateFile, err)
	}
	return nil
}

// loadComposed loads baseName from dir, applies compose overrides, and
// returns the resulting ComposedCharacter.
func loadComposed(dir, baseName string, compose map[string]string) (*ComposedCharacter, error) {
	base, err := Load(dir, baseName)
	if err != nil {
		return nil, err
	}

	overrides := make(map[string]*Character, len(compose))
	for section, ovName := range compose {
		ov, err := Load(dir, ovName)
		if err != nil {
			return nil, fmt.Errorf("character: load compose override %q: %w", ovName, err)
		}
		overrides[section] = ov
	}

	cc := Compose(base, overrides)
	return &cc, nil
}

// readStateEntry reads the state file and returns the entry for profileName.
// Returns nil (no error) when the file is absent or has no entry for that profile.
func readStateEntry(stateFile, profileName string) (*stateEntry, error) {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("character: read state %s: %w", stateFile, err)
	}

	var state map[string]stateEntry
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("character: parse state %s: %w", stateFile, err)
	}

	entry, ok := state[profileName]
	if !ok {
		return nil, nil
	}
	return &entry, nil
}
