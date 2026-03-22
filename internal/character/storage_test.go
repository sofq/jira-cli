package character

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func makeTestCharacter(name string) *Character {
	return &Character{
		Version:     "1",
		Name:        name,
		Source:      SourceManual,
		Description: "test",
		StyleGuide: StyleGuide{
			Writing:     "Be concise.",
			Workflow:    "Move fast.",
			Interaction: "Be direct.",
		},
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	c := makeTestCharacter("alice")

	if err := Save(dir, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir, "alice")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Name != c.Name {
		t.Errorf("Name: got %q, want %q", got.Name, c.Name)
	}
	if got.Version != c.Version {
		t.Errorf("Version: got %q, want %q", got.Version, c.Version)
	}
	if got.StyleGuide.Writing != c.StyleGuide.Writing {
		t.Errorf("Writing: got %q, want %q", got.StyleGuide.Writing, c.StyleGuide.Writing)
	}

	// Verify file permissions are 0o600.
	info, err := os.Stat(filepath.Join(dir, "alice.yaml"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file perm: got %o, want %o", info.Mode().Perm(), 0o600)
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"charlie", "alice", "bob"} {
		if err := Save(dir, makeTestCharacter(name)); err != nil {
			t.Fatalf("Save %s: %v", name, err)
		}
	}

	names, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	sort.Strings(names)
	want := []string{"alice", "bob", "charlie"}
	if len(names) != len(want) {
		t.Fatalf("List: got %v, want %v", names, want)
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("List[%d]: got %q, want %q", i, n, want[i])
		}
	}
}

func TestListEmptyDir(t *testing.T) {
	dir := t.TempDir()
	names, err := List(dir)
	if err != nil {
		t.Fatalf("List empty dir: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	c := makeTestCharacter("to-delete")

	if err := Save(dir, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !Exists(dir, "to-delete") {
		t.Fatal("expected character to exist after Save")
	}

	if err := Delete(dir, "to-delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if Exists(dir, "to-delete") {
		t.Error("expected character to be gone after Delete")
	}
}

func TestDeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	if err := Delete(dir, "nonexistent"); err == nil {
		t.Error("expected error deleting nonexistent character, got nil")
	}
}

func TestLoadNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir, "nonexistent")
	if err == nil {
		t.Error("expected error loading nonexistent character, got nil")
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()

	if Exists(dir, "missing") {
		t.Error("Exists: expected false for missing character")
	}

	if err := Save(dir, makeTestCharacter("present")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !Exists(dir, "present") {
		t.Error("Exists: expected true for saved character")
	}
}

func TestDefaultDir(t *testing.T) {
	// Without env override, should return a non-empty path.
	os.Unsetenv("JR_CHARACTER_BASE")
	d := DefaultDir()
	if d == "" {
		t.Error("DefaultDir: expected non-empty path")
	}

	// With env override.
	want := "/tmp/jr-chars"
	t.Setenv("JR_CHARACTER_BASE", want)
	d = DefaultDir()
	if d != want {
		t.Errorf("DefaultDir with env: got %q, want %q", d, want)
	}
}

func TestListIgnoresNonYAML(t *testing.T) {
	dir := t.TempDir()

	// Write a non-yaml file and a valid character file.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := Save(dir, makeTestCharacter("valid")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	names, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 1 || names[0] != "valid" {
		t.Errorf("List: got %v, want [valid]", names)
	}
}
