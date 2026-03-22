package character

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultDir returns the directory used to store character files.
// If the JR_CHARACTER_BASE environment variable is set, its value is returned
// directly. Otherwise the directory defaults to
// $XDG_CONFIG_HOME/jr/characters (falling back to ~/.config/jr/characters).
func DefaultDir() string {
	if v := os.Getenv("JR_CHARACTER_BASE"); v != "" {
		return v
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
	return filepath.Join(base, "jr", "characters")
}

// filePath returns the path to a character's YAML file inside dir.
func filePath(dir, name string) string {
	return filepath.Join(dir, name+".yaml")
}

// Save marshals c to YAML and writes it to {dir}/{c.Name}.yaml with 0o600
// permissions. The directory is created if it does not exist.
func Save(dir string, c *Character) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("character: create dir %s: %w", dir, err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("character: marshal %s: %w", c.Name, err)
	}
	path := filePath(dir, c.Name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("character: write %s: %w", path, err)
	}
	return nil
}

// Load reads {dir}/{name}.yaml and returns the decoded Character.
func Load(dir, name string) (*Character, error) {
	path := filePath(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("character %q not found", name)
		}
		return nil, fmt.Errorf("character: read %s: %w", path, err)
	}
	var c Character
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("character: unmarshal %s: %w", path, err)
	}
	return &c, nil
}

// List returns the names of all characters stored in dir (files ending in
// .yaml, without the extension). Returns an empty slice for an empty dir.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("character: list dir %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if n := e.Name(); strings.HasSuffix(n, ".yaml") {
			names = append(names, strings.TrimSuffix(n, ".yaml"))
		}
	}
	return names, nil
}

// Delete removes the character file for name from dir.
// It returns an error if the file does not exist.
func Delete(dir, name string) error {
	path := filePath(dir, name)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("character %q not found", name)
		}
		return fmt.Errorf("character: delete %s: %w", path, err)
	}
	return nil
}

// Exists reports whether a character named name exists in dir.
func Exists(dir, name string) bool {
	_, err := os.Stat(filePath(dir, name))
	return err == nil
}
