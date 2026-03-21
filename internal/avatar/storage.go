package avatar

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	extractionFile = "extraction.json"
	profileFile    = "profile.yaml"
	cacheDir       = "cache"
)

// avatarsBaseDir returns the base directory for all avatar data.
// It uses os.UserConfigDir() with a fallback to ~/.config/jr/avatars.
var avatarsBaseDir = func() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "jr", "avatars")
	}
	return filepath.Join(dir, "jr", "avatars")
}

// AvatarDir returns the directory path for a specific user's avatar data.
func AvatarDir(userHash string) string {
	return filepath.Join(avatarsBaseDir(), userHash)
}

// UserHash returns a 16-character hex string derived from the SHA-256 of accountID.
func UserHash(accountID string) string {
	sum := sha256.Sum256([]byte(accountID))
	return fmt.Sprintf("%x", sum[:])[:16]
}

// SaveExtraction writes an Extraction to extraction.json in dir with 0o600 permissions.
func SaveExtraction(dir string, e *Extraction) error {
	// json.MarshalIndent cannot fail for *Extraction (all fields are
	// primitive types, strings, slices, and maps — no channels or funcs).
	data, _ := json.MarshalIndent(e, "", "  ")
	path := filepath.Join(dir, extractionFile)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write extraction: %w", err)
	}
	return nil
}

// LoadExtraction reads extraction.json from dir and returns the Extraction.
func LoadExtraction(dir string) (*Extraction, error) {
	path := filepath.Join(dir, extractionFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read extraction: %w", err)
	}
	var e Extraction
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("unmarshal extraction: %w", err)
	}
	return &e, nil
}

// SaveProfile writes a Profile to profile.yaml in dir with 0o600 permissions.
func SaveProfile(dir string, p *Profile) error {
	// yaml.Marshal cannot fail for *Profile (all fields are primitive types,
	// strings, slices, and maps — no channels or funcs).
	data, _ := yaml.Marshal(p)
	path := filepath.Join(dir, profileFile)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}
	return nil
}

// LoadProfile reads profile.yaml from dir and returns the Profile.
func LoadProfile(dir string) (*Profile, error) {
	path := filepath.Join(dir, profileFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("unmarshal profile: %w", err)
	}
	return &p, nil
}

// ProfileExists reports whether profile.yaml exists in dir.
func ProfileExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, profileFile))
	return err == nil
}

// ExtractionExists reports whether extraction.json exists in dir.
func ExtractionExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, extractionFile))
	return err == nil
}

// ProfileAgeDays returns the number of whole days since the profile was generated.
func ProfileAgeDays(dir string) (int, error) {
	p, err := LoadProfile(dir)
	if err != nil {
		return 0, err
	}
	t, err := time.Parse(time.RFC3339, p.GeneratedAt)
	if err != nil {
		return 0, fmt.Errorf("parse generated_at %q: %w", p.GeneratedAt, err)
	}
	age := int(time.Since(t).Hours() / 24)
	return age, nil
}

// CacheDir returns the cache subdirectory path within an avatar directory.
func CacheDir(avatarDir string) string {
	return filepath.Join(avatarDir, cacheDir)
}
