package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRun(t *testing.T) {
	outDir := t.TempDir()
	err := run("../spec/jira-v3.json", outDir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Verify output files exist.
	for _, name := range []string{"init.go", "schema_data.go", "issue.go"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}
}

func TestRunBadSpec(t *testing.T) {
	outDir := t.TempDir()
	err := run("nonexistent.json", outDir)
	if err == nil {
		t.Fatal("expected error for nonexistent spec, got nil")
	}
}

func TestRunMkdirAllError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based test not reliable on Windows")
	}
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	// Don't create outDir — RemoveAll on nonexistent is a no-op.
	// Make parent read-only so MkdirAll fails.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	err := run("../spec/jira-v3.json", outDir)
	if err == nil {
		t.Skip("MkdirAll did not fail (may require non-root)")
	}
}

func TestRunRemoveAllError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based test not reliable on Windows")
	}
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Create a file inside outDir and make it non-removable by removing
	// write+execute on the outDir itself.
	blocker := filepath.Join(outDir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Remove write+execute on outDir so its contents can't be deleted.
	if err := os.Chmod(outDir, 0o444); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() {
		os.Chmod(outDir, 0o755)
	})

	err := run("../spec/jira-v3.json", outDir)
	if err == nil {
		t.Skip("RemoveAll did not fail (may require non-root)")
	}
}

func TestRunGenerateResourceError(t *testing.T) {
	old := loadTemplateFn
	callCount := 0
	loadTemplateFn = func(name string) (string, error) {
		callCount++
		// Fail on the first template load (resource generation).
		if name == "resource.go.tmpl" {
			return "", fmt.Errorf("injected resource error")
		}
		return old(name)
	}
	t.Cleanup(func() { loadTemplateFn = old })

	err := run("../spec/jira-v3.json", t.TempDir())
	if err == nil {
		t.Fatal("expected error from GenerateResource failure")
	}
}

func TestRunGenerateSchemaDataError(t *testing.T) {
	old := loadTemplateFn
	loadTemplateFn = func(name string) (string, error) {
		if name == "schema_data.go.tmpl" {
			return "", fmt.Errorf("injected schema error")
		}
		return old(name)
	}
	t.Cleanup(func() { loadTemplateFn = old })

	err := run("../spec/jira-v3.json", t.TempDir())
	if err == nil {
		t.Fatal("expected error from GenerateSchemaData failure")
	}
}

func TestRunGenerateInitError(t *testing.T) {
	old := loadTemplateFn
	loadTemplateFn = func(name string) (string, error) {
		if name == "init.go.tmpl" {
			return "", fmt.Errorf("injected init error")
		}
		return old(name)
	}
	t.Cleanup(func() { loadTemplateFn = old })

	err := run("../spec/jira-v3.json", t.TempDir())
	if err == nil {
		t.Fatal("expected error from GenerateInit failure")
	}
}

func TestMainSuccess(t *testing.T) {
	// Run main() from the repo root where spec/jira-v3.json exists.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(".."); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		os.Chdir(origDir)
		// Clean up generated files.
		os.RemoveAll(filepath.Join("cmd", "generated"))
	})

	// Override exitFn so os.Exit isn't called.
	exitCalled := false
	exitCode := 0
	oldExit := exitFn
	exitFn = func(code int) { exitCalled = true; exitCode = code }
	t.Cleanup(func() { exitFn = oldExit })

	main()

	if exitCalled {
		t.Fatalf("main() called exit with code %d", exitCode)
	}
}

func TestMainError(t *testing.T) {
	// Run main() from a temp dir where spec/jira-v3.json doesn't exist.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	exitCalled := false
	exitCode := 0
	oldExit := exitFn
	exitFn = func(code int) { exitCalled = true; exitCode = code }
	t.Cleanup(func() { exitFn = oldExit })

	main()

	if !exitCalled {
		t.Fatal("expected main() to call exit on error")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}
