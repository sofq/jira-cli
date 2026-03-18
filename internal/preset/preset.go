package preset

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

// Preset defines a named combination of fields and jq filter.
type Preset struct {
	Fields string `json:"fields"`
	JQ     string `json:"jq"`
}

// builtinPresets contains the default presets shipped with jr.
var builtinPresets = map[string]Preset{
	"agent": {
		Fields: "key,summary,status,assignee,issuetype,priority",
		JQ:     "",
	},
	"detail": {
		Fields: "key,summary,status,assignee,issuetype,priority,description,comment,subtasks,issuelinks",
		JQ:     "",
	},
	"triage": {
		Fields: "key,summary,status,priority,created,updated,reporter",
		JQ:     "",
	},
	"board": {
		Fields: "key,summary,status,assignee,customfield_10020,customfield_10028,issuetype",
		JQ:     "",
	},
}

// userPresetsPath returns the path to the user-defined presets config file.
// It is a var so tests can override it.
var userPresetsPath = func() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		// Fallback: use home directory.
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "jr", "presets.json")
	}
	return filepath.Join(dir, "jr", "presets.json")
}

// loadUserPresets reads user-defined presets from disk.
// Returns (nil, nil) if the file doesn't exist.
func loadUserPresets() (map[string]Preset, error) {
	data, err := os.ReadFile(userPresetsPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var presets map[string]Preset
	if err := json.Unmarshal(data, &presets); err != nil {
		return nil, err
	}
	return presets, nil
}

// Lookup resolves a preset name to its definition. User-defined presets
// override built-in presets with the same name. Returns the preset and
// true if found, or a zero Preset and false if not found.
// Returns an error if the user presets file exists but cannot be read or parsed.
func Lookup(name string) (Preset, bool, error) {
	user, err := loadUserPresets()
	if err != nil {
		return Preset{}, false, err
	}
	if p, ok := user[name]; ok {
		return p, true, nil
	}
	p, ok := builtinPresets[name]
	return p, ok, nil
}

// presetEntry is used for listing presets with their source.
type presetEntry struct {
	Name   string `json:"name"`
	Fields string `json:"fields"`
	JQ     string `json:"jq,omitempty"`
	Source string `json:"source"`
}

// List returns all available presets (built-in + user-defined) as JSON bytes.
// User-defined presets override built-in ones with the same name.
func List() ([]byte, error) {
	merged := make(map[string]presetEntry)

	// Start with built-in presets.
	for name, p := range builtinPresets {
		merged[name] = presetEntry{
			Name:   name,
			Fields: p.Fields,
			JQ:     p.JQ,
			Source: "builtin",
		}
	}

	// Overlay user-defined presets (errors are surfaced to caller).
	user, err := loadUserPresets()
	if err != nil {
		return nil, err
	}
	for name, p := range user {
		merged[name] = presetEntry{
			Name:   name,
			Fields: p.Fields,
			JQ:     p.JQ,
			Source: "user",
		}
	}

	// Sort by name for stable output.
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]presetEntry, 0, len(names))
	for _, name := range names {
		result = append(result, merged[name])
	}

	return json.Marshal(result)
}
