package avatar

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAvatarDir(t *testing.T) {
	hash := UserHash("some-account-id")
	dir := AvatarDir(hash)
	if dir == "" {
		t.Fatal("AvatarDir returned empty string")
	}
	parent := filepath.Base(filepath.Dir(dir))
	if parent != "avatars" {
		t.Errorf("expected parent dir to be 'avatars', got %q", parent)
	}
}

func TestUserHash(t *testing.T) {
	// Deterministic: same input yields same output.
	h1 := UserHash("accountID-123")
	h2 := UserHash("accountID-123")
	if h1 != h2 {
		t.Errorf("UserHash is not deterministic: %q != %q", h1, h2)
	}

	// Different inputs yield different outputs.
	h3 := UserHash("accountID-456")
	if h1 == h3 {
		t.Errorf("UserHash collision: different inputs produced the same hash %q", h1)
	}

	// Length is exactly 16.
	if len(h1) != 16 {
		t.Errorf("UserHash length = %d, want 16", len(h1))
	}
}

func TestSaveAndLoadExtraction(t *testing.T) {
	dir := t.TempDir()

	e := &Extraction{
		Version: "1.0",
		Meta: ExtractionMeta{
			User:        "user@example.com",
			DisplayName: "Test User",
			ExtractedAt: time.Now().UTC().Truncate(time.Second),
		},
	}

	if err := SaveExtraction(dir, e); err != nil {
		t.Fatalf("SaveExtraction: %v", err)
	}

	got, err := LoadExtraction(dir)
	if err != nil {
		t.Fatalf("LoadExtraction: %v", err)
	}

	if got.Version != e.Version {
		t.Errorf("Version: got %q, want %q", got.Version, e.Version)
	}
	if got.Meta.User != e.Meta.User {
		t.Errorf("Meta.User: got %q, want %q", got.Meta.User, e.Meta.User)
	}
	if got.Meta.DisplayName != e.Meta.DisplayName {
		t.Errorf("Meta.DisplayName: got %q, want %q", got.Meta.DisplayName, e.Meta.DisplayName)
	}
}

func TestSaveAndLoadProfile(t *testing.T) {
	dir := t.TempDir()

	p := &Profile{
		Version:     "1.0",
		User:        "user@example.com",
		DisplayName: "Test User",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Engine:      "gpt-4",
		StyleGuide: StyleGuide{
			Writing:     "Be concise.",
			Workflow:    "Assign before transitioning.",
			Interaction: "Be direct.",
		},
		Defaults: ProfileDefaults{
			Priority: "Medium",
		},
	}

	if err := SaveProfile(dir, p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	got, err := LoadProfile(dir)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	if got.Version != p.Version {
		t.Errorf("Version: got %q, want %q", got.Version, p.Version)
	}
	if got.User != p.User {
		t.Errorf("User: got %q, want %q", got.User, p.User)
	}
	if got.StyleGuide.Writing != p.StyleGuide.Writing {
		t.Errorf("StyleGuide.Writing: got %q, want %q", got.StyleGuide.Writing, p.StyleGuide.Writing)
	}
	if got.Defaults.Priority != p.Defaults.Priority {
		t.Errorf("Defaults.Priority: got %q, want %q", got.Defaults.Priority, p.Defaults.Priority)
	}
}

func TestLoadExtractionNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadExtraction(dir)
	if err == nil {
		t.Fatal("expected error when extraction.json does not exist, got nil")
	}
}

func TestProfileAge(t *testing.T) {
	dir := t.TempDir()

	// Use a date 10 days ago.
	tenDaysAgo := time.Now().UTC().AddDate(0, 0, -10).Format(time.RFC3339)
	p := &Profile{
		Version:     "1.0",
		GeneratedAt: tenDaysAgo,
	}

	if err := SaveProfile(dir, p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	age, err := ProfileAgeDays(dir)
	if err != nil {
		t.Fatalf("ProfileAgeDays: %v", err)
	}
	if age < 9 {
		t.Errorf("ProfileAgeDays = %d, want >= 9", age)
	}
}

func TestProfileExists(t *testing.T) {
	dir := t.TempDir()

	if ProfileExists(dir) {
		t.Error("ProfileExists returned true for empty dir")
	}

	p := &Profile{Version: "1.0"}
	if err := SaveProfile(dir, p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	if !ProfileExists(dir) {
		t.Error("ProfileExists returned false after saving profile")
	}
}

func TestExtractionExists(t *testing.T) {
	dir := t.TempDir()

	if ExtractionExists(dir) {
		t.Error("ExtractionExists returned true for empty dir")
	}

	e := &Extraction{Version: "1.0"}
	if err := SaveExtraction(dir, e); err != nil {
		t.Fatalf("SaveExtraction: %v", err)
	}

	if !ExtractionExists(dir) {
		t.Error("ExtractionExists returned false after saving extraction")
	}
}

func TestCacheDir(t *testing.T) {
	avatarDir := "/some/path/avatars/abc123"
	cd := CacheDir(avatarDir)
	if cd == "" {
		t.Fatal("CacheDir returned empty string")
	}
	if filepath.Base(cd) != "cache" {
		t.Errorf("CacheDir base = %q, want 'cache'", filepath.Base(cd))
	}
	if filepath.Dir(cd) != avatarDir {
		t.Errorf("CacheDir parent = %q, want %q", filepath.Dir(cd), avatarDir)
	}
}

func TestSaveExtractionFilePermissions(t *testing.T) {
	dir := t.TempDir()
	e := &Extraction{Version: "1.0"}
	if err := SaveExtraction(dir, e); err != nil {
		t.Fatalf("SaveExtraction: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, extractionFile))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("extraction.json perms = %o, want 0600", info.Mode().Perm())
	}
}

func TestSaveProfileFilePermissions(t *testing.T) {
	dir := t.TempDir()
	p := &Profile{Version: "1.0"}
	if err := SaveProfile(dir, p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, profileFile))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("profile.yaml perms = %o, want 0600", info.Mode().Perm())
	}
}
