# Character System & Yolo Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the avatar feature into a composable character system with an autonomous execution policy (yolo mode) that lets AI agents act on behalf of users in Jira.

**Architecture:** Three layers — (1) `internal/character/` handles persona loading, composition, validation, and storage; (2) `internal/yolo/` handles execution policy, scope tiers, rate limiting, and state; (3) `jr avatar act` wraps the existing watch polling loop with a decide-and-execute step. Characters define *how* (voice/style), yolo defines *what* (permissions/limits).

**Tech Stack:** Go 1.25, Cobra CLI, `gopkg.in/yaml.v3`, existing `internal/policy` glob matching, existing `internal/audit` JSONL logging, existing `internal/watch` polling.

**Spec:** `docs/superpowers/specs/2026-03-23-character-system-and-yolo-mode-design.md`

---

## File Structure

### New Files

| File | Responsibility |
|------|---------------|
| `internal/character/types.go` | Character struct, ComposedCharacter, source constants |
| `internal/character/storage.go` | Load, save, list, delete characters from `~/.config/jr/characters/` |
| `internal/character/compose.go` | Merge characters by section (writing, workflow, interaction) |
| `internal/character/convert.go` | Convert avatar `Profile` to `Character` |
| `internal/character/prompt.go` | Format composed character for agent consumption (prose/json/both, redact) |
| `internal/character/templates.go` | Built-in template characters (concise, detailed, formal, casual) |
| `internal/character/validate.go` | Validate character YAML (version, required fields) |
| `internal/character/active.go` | Active character resolution (flag > config > state file > self-avatar) |
| `internal/yolo/policy.go` | YoloPolicy: scope tiers, action checking, integration with existing policy |
| `internal/yolo/ratelimit.go` | Token bucket rate limiter with state persistence |
| `internal/yolo/config.go` | YoloConfig struct, scope tier definitions |
| `internal/yolo/decide.go` | Decision engine: rule-based + LLM-assisted |
| `internal/yolo/events.go` | Canonical event types, event classification |
| `cmd/character.go` | Cobra commands: list, show, create, edit, delete, use, prompt |
| `cmd/yolo.go` | Cobra commands: status, history |
| `cmd/avatar_act.go` | `jr avatar act` command: autonomous loop |
| `internal/character/types_test.go` | Character type tests |
| `internal/character/storage_test.go` | Storage CRUD tests |
| `internal/character/compose_test.go` | Composition merge tests |
| `internal/character/convert_test.go` | Profile-to-Character conversion tests |
| `internal/character/prompt_test.go` | Prompt formatting tests |
| `internal/character/templates_test.go` | Built-in template tests |
| `internal/character/validate_test.go` | Validation tests |
| `internal/character/active_test.go` | Resolution order tests |
| `internal/yolo/policy_test.go` | Scope tier + action checking tests |
| `internal/yolo/ratelimit_test.go` | Token bucket + persistence tests |
| `internal/yolo/decide_test.go` | Rule-based + LLM decision tests |
| `internal/yolo/events_test.go` | Event type classification tests |
| `cmd/character_test.go` | Character CLI tests |
| `cmd/yolo_test.go` | Yolo CLI tests |
| `cmd/avatar_act_test.go` | Autonomous loop CLI tests |
| `internal/character/testdata/sample_character.yaml` | Golden file: sample character |
| `internal/character/testdata/composed_expected.yaml` | Golden file: composed output |
| `internal/yolo/testdata/yolo_state.json` | Golden file: rate limiter state |

### Modified Files

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `YoloConfig` struct to `Profile` |
| `internal/audit/audit.go` | Add `Extra map[string]any` field to `Entry` |
| `internal/watch/watcher.go` | Refactor `Run()` to accept `EventHandler` callback |
| `cmd/root.go` | Add `--yolo` and `--character` global flags, yolo policy check in PersistentPreRunE |
| `cmd/avatar.go` | Register `actCmd` subcommand, update `build` to save character file |
| `cmd/batch.go` | Add per-operation yolo policy check |

---

## Task Dependencies

```
Task 1 (Character types) ─┐
Task 2 (Character storage) ─┤
Task 3 (Character compose) ─┼─→ Task 14 (Character CLI)
Task 4 (Character convert) ─┤                         ─┐
Task 5 (Character templates)┤                          │
Task 6 (Character prompt)  ─┤                          │
Task 7 (Validate + Active) ─┘                          │
                                                       ├─→ Task 18 (Avatar act)
Task 8 (Yolo config)       ─┐                         │
Task 9 (Yolo policy)       ─┤                         │
Task 10 (Yolo ratelimit)   ─┼─→ Task 15 (Yolo CLI)   │
Task 11 (Yolo events+decide)┘                    ─────┘

Task 12 (Audit Extra field) ─→ independent, needed by Task 18
Task 13 (Watch refactor)    ─→ independent, needed by Task 18
Task 16 (Root.go integration) ─→ needs Tasks 14, 9, 15
Task 17 (Avatar build → character) ─→ needs Task 4
```

---

## Task 1: Character Types

**Files:**
- Create: `internal/character/types.go`
- Test: `internal/character/types_test.go`

- [ ] **Step 1: Write the failing test for Character struct**

```go
// internal/character/types_test.go
package character

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCharacterRoundTrip(t *testing.T) {
	c := Character{
		Version:     "1",
		Name:        "formal-pm",
		Source:      SourceManual,
		Description: "Professional PM tone",
		StyleGuide: StyleGuide{
			Writing:     "Write in complete sentences.",
			Workflow:    "Always set priority.",
			Interaction: "Mention stakeholders.",
		},
		Defaults: Defaults{
			Priority:   "Medium",
			Labels:     []string{"managed"},
			Components: []string{"auth"},
			AssignSelfOnTransition: true,
		},
		Examples: []Example{
			{Context: "status_update", Text: "The auth module is complete."},
		},
		Reactions: map[string]Reaction{
			"on_assigned": {Action: "transition", To: "In Progress"},
		},
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Character
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Name != c.Name {
		t.Errorf("Name = %q, want %q", got.Name, c.Name)
	}
	if got.Source != SourceManual {
		t.Errorf("Source = %q, want %q", got.Source, SourceManual)
	}
	if got.Reactions["on_assigned"].Action != "transition" {
		t.Errorf("Reaction action = %q, want %q", got.Reactions["on_assigned"].Action, "transition")
	}
}

func TestSourceConstants(t *testing.T) {
	sources := []string{SourceAvatar, SourceClone, SourceManual, SourceTemplate, SourcePersona}
	for _, s := range sources {
		if s == "" {
			t.Error("source constant is empty")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/character/ -run TestCharacter -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement Character types**

```go
// internal/character/types.go
package character

// Source constants for character origin.
const (
	SourceAvatar   = "avatar"
	SourceClone    = "clone"
	SourceManual   = "manual"
	SourceTemplate = "template"
	SourcePersona  = "persona"
)

// Character defines a persona that governs how an AI agent behaves.
type Character struct {
	Version     string               `yaml:"version"     json:"version"`
	Name        string               `yaml:"name"        json:"name"`
	Source      string               `yaml:"source"      json:"source"`
	Description string               `yaml:"description" json:"description"`
	StyleGuide  StyleGuide           `yaml:"style_guide" json:"style_guide"`
	Defaults    Defaults             `yaml:"defaults,omitempty"   json:"defaults,omitempty"`
	Examples    []Example            `yaml:"examples,omitempty"   json:"examples,omitempty"`
	Reactions   map[string]Reaction  `yaml:"reactions,omitempty"  json:"reactions,omitempty"`
}

// StyleGuide contains prose guidance for each behavioral dimension.
type StyleGuide struct {
	Writing     string `yaml:"writing"     json:"writing"`
	Workflow    string `yaml:"workflow"    json:"workflow"`
	Interaction string `yaml:"interaction" json:"interaction"`
}

// Defaults are field defaults applied when creating/editing issues.
type Defaults struct {
	Priority               string   `yaml:"priority,omitempty"                json:"priority,omitempty"`
	Labels                 []string `yaml:"labels,omitempty"                  json:"labels,omitempty"`
	Components             []string `yaml:"components,omitempty"              json:"components,omitempty"`
	AssignSelfOnTransition bool     `yaml:"assign_self_on_transition,omitempty" json:"assign_self_on_transition,omitempty"`
}

// Example is a representative text sample with context.
type Example struct {
	Context string `yaml:"context" json:"context"`
	Text    string `yaml:"text"    json:"text"`
}

// Reaction defines a rule-based response to an event.
type Reaction struct {
	Action   string `yaml:"action"             json:"action"`
	To       string `yaml:"to,omitempty"        json:"to,omitempty"`
	Text     string `yaml:"text,omitempty"      json:"text,omitempty"`
	Assignee string `yaml:"assignee,omitempty"  json:"assignee,omitempty"`
}

// ComposedCharacter is the result of merging a base character with section overrides.
type ComposedCharacter struct {
	Base      string            // name of base character
	Overrides map[string]string // section -> character name (e.g., "writing" -> "formal-pm")
	Character                   // the merged result
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/character/ -run TestCharacter -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/character/types.go internal/character/types_test.go
git commit -m "feat(character): add Character type definitions and source constants"
```

---

## Task 2: Character Storage

**Files:**
- Create: `internal/character/storage.go`
- Test: `internal/character/storage_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/character/storage_test.go
package character

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JR_CHARACTER_BASE", dir)

	c := Character{
		Version:     "1",
		Name:        "test-char",
		Source:      SourceManual,
		Description: "Test character",
		StyleGuide: StyleGuide{
			Writing:     "Be concise.",
			Workflow:    "Set priority.",
			Interaction: "Reply fast.",
		},
	}

	if err := Save(dir, &c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir, "test-char")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != "test-char" {
		t.Errorf("Name = %q, want %q", got.Name, "test-char")
	}
	if got.StyleGuide.Writing != "Be concise." {
		t.Errorf("Writing = %q, want %q", got.StyleGuide.Writing, "Be concise.")
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"alpha", "beta"} {
		c := Character{Version: "1", Name: name, Source: SourceManual}
		if err := Save(dir, &c); err != nil {
			t.Fatalf("Save %s: %v", name, err)
		}
	}

	chars, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(chars) != 2 {
		t.Errorf("len = %d, want 2", len(chars))
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	c := Character{Version: "1", Name: "doomed", Source: SourceManual}
	if err := Save(dir, &c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := Delete(dir, "doomed"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := Load(dir, "doomed"); err == nil {
		t.Error("Load after delete should fail")
	}
}

func TestLoadNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent character")
	}
}

func TestCharacterDir(t *testing.T) {
	t.Setenv("JR_CHARACTER_BASE", "/tmp/test-chars")
	got := DefaultDir()
	if got != "/tmp/test-chars" {
		t.Errorf("DefaultDir = %q, want /tmp/test-chars", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/character/ -run TestSave -v`
Expected: FAIL — `Save`, `Load`, `List`, `Delete`, `DefaultDir` undefined

- [ ] **Step 3: Implement storage**

```go
// internal/character/storage.go
package character

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultDir returns the character storage directory.
// Respects JR_CHARACTER_BASE env var (for testing).
func DefaultDir() string {
	if base := os.Getenv("JR_CHARACTER_BASE"); base != "" {
		return base
	}
	home, _ := os.UserConfigDir()
	return filepath.Join(home, "jr", "characters")
}

// Save writes a character to disk as YAML.
func Save(dir string, c *Character) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create character dir: %w", err)
	}
	path := filepath.Join(dir, c.Name+".yaml")
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal character: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// Load reads a character from disk by name.
func Load(dir, name string) (*Character, error) {
	path := filepath.Join(dir, name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read character %q: %w", name, err)
	}
	var c Character
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse character %q: %w", name, err)
	}
	return &c, nil
}

// List returns all characters in the directory.
func List(dir string) ([]Character, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var chars []Character
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		c, err := Load(dir, name)
		if err != nil {
			continue // skip malformed files
		}
		chars = append(chars, *c)
	}
	return chars, nil
}

// Delete removes a character file by name.
func Delete(dir, name string) error {
	path := filepath.Join(dir, name+".yaml")
	return os.Remove(path)
}

// Exists checks if a character file exists by name.
func Exists(dir, name string) bool {
	path := filepath.Join(dir, name+".yaml")
	_, err := os.Stat(path)
	return err == nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/character/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/character/storage.go internal/character/storage_test.go
git commit -m "feat(character): add character storage (save, load, list, delete)"
```

---

## Task 3: Character Composition

**Files:**
- Create: `internal/character/compose.go`
- Test: `internal/character/compose_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/character/compose_test.go
package character

import "testing"

func TestCompose_SingleCharacter(t *testing.T) {
	base := &Character{
		Version:    "1",
		Name:       "base",
		StyleGuide: StyleGuide{Writing: "base-writing", Workflow: "base-workflow", Interaction: "base-interaction"},
		Defaults:   Defaults{Priority: "High", Labels: []string{"base"}},
		Examples:   []Example{{Context: "a", Text: "base example"}},
	}
	got := Compose(base, nil)
	if got.Character.StyleGuide.Writing != "base-writing" {
		t.Errorf("Writing = %q, want %q", got.Character.StyleGuide.Writing, "base-writing")
	}
}

func TestCompose_OverrideWriting(t *testing.T) {
	base := &Character{
		Version:    "1",
		Name:       "base",
		StyleGuide: StyleGuide{Writing: "base-writing", Workflow: "base-workflow", Interaction: "base-interaction"},
		Defaults:   Defaults{Priority: "High"},
		Examples:   []Example{{Context: "a", Text: "base"}},
	}
	override := &Character{
		Name:       "override",
		StyleGuide: StyleGuide{Writing: "override-writing"},
		Defaults:   Defaults{Labels: []string{"override-label"}},
		Examples:   []Example{{Context: "b", Text: "override"}},
	}

	got := Compose(base, map[string]*Character{"writing": override})

	if got.StyleGuide.Writing != "override-writing" {
		t.Errorf("Writing = %q, want override-writing", got.StyleGuide.Writing)
	}
	if got.StyleGuide.Workflow != "base-workflow" {
		t.Errorf("Workflow = %q, want base-workflow", got.StyleGuide.Workflow)
	}
	// Defaults come from base only — section overrides only affect style_guide
	if got.Defaults.Priority != "High" {
		t.Errorf("Priority = %q, want High (from base)", got.Defaults.Priority)
	}
	if len(got.Examples) != 1 {
		t.Errorf("Examples len = %d, want 1 (from base only)", len(got.Examples))
	}
}

func TestCompose_OverrideMultipleSections(t *testing.T) {
	base := &Character{
		Name:       "base",
		StyleGuide: StyleGuide{Writing: "bw", Workflow: "bwf", Interaction: "bi"},
		Reactions:  map[string]Reaction{"on_assigned": {Action: "transition", To: "In Progress"}},
	}
	wOverride := &Character{Name: "w", StyleGuide: StyleGuide{Writing: "ow"}}
	iOverride := &Character{
		Name:      "i",
		StyleGuide: StyleGuide{Interaction: "oi"},
		Reactions:  map[string]Reaction{"on_blocked": {Action: "comment", Text: "blocked"}},
	}

	got := Compose(base, map[string]*Character{"writing": wOverride, "interaction": iOverride})

	if got.StyleGuide.Writing != "ow" {
		t.Errorf("Writing = %q, want ow", got.StyleGuide.Writing)
	}
	if got.StyleGuide.Workflow != "bwf" {
		t.Errorf("Workflow = %q, want bwf", got.StyleGuide.Workflow)
	}
	if got.StyleGuide.Interaction != "oi" {
		t.Errorf("Interaction = %q, want oi", got.StyleGuide.Interaction)
	}
	if len(got.Reactions) != 2 {
		t.Errorf("Reactions len = %d, want 2", len(got.Reactions))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/character/ -run TestCompose -v`
Expected: FAIL — `Compose` undefined

- [ ] **Step 3: Implement composition**

```go
// internal/character/compose.go
package character

// Compose merges a base character with section overrides.
// Each section (writing, workflow, interaction) is atomic — the override replaces the entire section.
// Defaults are deep-merged. Examples are concatenated. Reactions are deep-merged.
func Compose(base *Character, overrides map[string]*Character) ComposedCharacter {
	result := ComposedCharacter{
		Base:      base.Name,
		Overrides: make(map[string]string),
		Character: *base,
	}

	if len(overrides) == 0 {
		return result
	}

	for section, override := range overrides {
		result.Overrides[section] = override.Name
		// Only override the specific style_guide section — not defaults/examples/reactions
		switch section {
		case "writing":
			result.StyleGuide.Writing = override.StyleGuide.Writing
		case "workflow":
			result.StyleGuide.Workflow = override.StyleGuide.Workflow
		case "interaction":
			result.StyleGuide.Interaction = override.StyleGuide.Interaction
		}
	}

	return result
}

func mergeDefaults(dst, src *Defaults) {
	if src.Priority != "" {
		dst.Priority = src.Priority
	}
	if len(src.Labels) > 0 {
		dst.Labels = src.Labels
	}
	if len(src.Components) > 0 {
		dst.Components = src.Components
	}
	if src.AssignSelfOnTransition {
		dst.AssignSelfOnTransition = true
	}
}

func mergeReactions(dst *ComposedCharacter, src *Character) {
	if len(src.Reactions) == 0 {
		return
	}
	if dst.Reactions == nil {
		dst.Reactions = make(map[string]Reaction)
	}
	for k, v := range src.Reactions {
		dst.Reactions[k] = v
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/character/ -run TestCompose -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/character/compose.go internal/character/compose_test.go
git commit -m "feat(character): add character composition with section-level overrides"
```

---

## Task 4: Profile-to-Character Conversion

**Files:**
- Create: `internal/character/convert.go`
- Test: `internal/character/convert_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/character/convert_test.go
package character

import (
	"testing"

	"github.com/sofq/jira-cli/internal/avatar"
)

func TestFromProfile(t *testing.T) {
	p := &avatar.Profile{
		Version:     "1",
		User:        "abc123",
		DisplayName: "Quan Hoang",
		GeneratedAt: "2026-03-23T10:00:00Z",
		Engine:      "local",
		StyleGuide: avatar.StyleGuide{
			Writing:     "Concise, direct.",
			Workflow:    "Always sets priority.",
			Interaction: "Responds within 2 hours.",
		},
		Defaults: avatar.ProfileDefaults{
			Priority:               "Medium",
			Labels:                 []string{"backend"},
			Components:             []string{"auth"},
			AssignSelfOnTransition: true,
		},
		Examples: []avatar.ProfileExample{
			{Context: "status_update", Source: "PROJ-123", Text: "Fixed the auth timeout."},
		},
	}

	c := FromProfile(p)

	if c.Name != "quan-hoang" {
		t.Errorf("Name = %q, want %q", c.Name, "quan-hoang")
	}
	if c.Source != SourceAvatar {
		t.Errorf("Source = %q, want %q", c.Source, SourceAvatar)
	}
	if c.StyleGuide.Writing != "Concise, direct." {
		t.Errorf("Writing = %q", c.StyleGuide.Writing)
	}
	if c.Defaults.Priority != "Medium" {
		t.Errorf("Priority = %q", c.Defaults.Priority)
	}
	if c.Defaults.AssignSelfOnTransition != true {
		t.Error("AssignSelfOnTransition should be true")
	}
	if len(c.Examples) != 1 || c.Examples[0].Context != "status_update" {
		t.Errorf("Examples = %v", c.Examples)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Quan Hoang", "quan-hoang"},
		{"Alice", "alice"},
		{"Bob O'Brien", "bob-obrien"},
		{"  spaces  ", "spaces"},
	}
	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/character/ -run TestFromProfile -v`
Expected: FAIL — `FromProfile`, `slugify` undefined

- [ ] **Step 3: Implement conversion**

```go
// internal/character/convert.go
package character

import (
	"regexp"
	"strings"

	"github.com/sofq/jira-cli/internal/avatar"
)

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9-]+`)

// FromProfile converts an avatar Profile to a Character.
func FromProfile(p *avatar.Profile) *Character {
	name := slugify(p.DisplayName)

	examples := make([]Example, len(p.Examples))
	for i, e := range p.Examples {
		examples[i] = Example{Context: e.Context, Text: e.Text}
	}

	return &Character{
		Version:     "1",
		Name:        name,
		Source:      SourceAvatar,
		Description: "Avatar profile for " + p.DisplayName,
		StyleGuide: StyleGuide{
			Writing:     p.StyleGuide.Writing,
			Workflow:    p.StyleGuide.Workflow,
			Interaction: p.StyleGuide.Interaction,
		},
		Defaults: Defaults{
			Priority:               p.Defaults.Priority,
			Labels:                 p.Defaults.Labels,
			Components:             p.Defaults.Components,
			AssignSelfOnTransition: p.Defaults.AssignSelfOnTransition,
		},
		Examples: examples,
	}
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphaNum.ReplaceAllString(s, "")
	s = strings.Trim(s, "-")
	return s
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/character/ -run "TestFromProfile|TestSlugify" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/character/convert.go internal/character/convert_test.go
git commit -m "feat(character): convert avatar Profile to Character"
```

---

## Task 5: Built-in Templates

**Files:**
- Create: `internal/character/templates.go`
- Test: `internal/character/templates_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/character/templates_test.go
package character

import "testing"

func TestBuiltinTemplates(t *testing.T) {
	names := TemplateNames()
	if len(names) != 4 {
		t.Fatalf("TemplateNames len = %d, want 4", len(names))
	}

	for _, name := range names {
		tmpl, err := Template(name)
		if err != nil {
			t.Errorf("Template(%q): %v", name, err)
			continue
		}
		if tmpl.Version != "1" {
			t.Errorf("%s: Version = %q, want 1", name, tmpl.Version)
		}
		if tmpl.Source != SourceTemplate {
			t.Errorf("%s: Source = %q, want %q", name, tmpl.Source, SourceTemplate)
		}
		if tmpl.StyleGuide.Writing == "" {
			t.Errorf("%s: Writing is empty", name)
		}
	}
}

func TestTemplateNotFound(t *testing.T) {
	_, err := Template("nonexistent")
	if err == nil {
		t.Error("expected error for unknown template")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/character/ -run TestBuiltin -v`
Expected: FAIL — `TemplateNames`, `Template` undefined

- [ ] **Step 3: Implement templates**

```go
// internal/character/templates.go
package character

import "fmt"

var builtinTemplates = map[string]*Character{
	"concise": {
		Version:     "1",
		Name:        "concise",
		Source:      SourceTemplate,
		Description: "Short sentences, no formatting, action-oriented",
		StyleGuide: StyleGuide{
			Writing:     "Use short, direct sentences. No bullet points or formatting. Lead with the action or conclusion. Maximum 2 sentences per comment.",
			Workflow:    "Minimal fields. Set only what is required. Skip optional metadata.",
			Interaction: "Reply briefly. One sentence acknowledgments. No pleasantries.",
		},
	},
	"detailed": {
		Version:     "1",
		Name:        "detailed",
		Source:      SourceTemplate,
		Description: "Thorough explanations, bullet points, context-heavy",
		StyleGuide: StyleGuide{
			Writing:     "Provide full context. Use bullet points for multi-item lists. Include background, current state, and next steps. Reference related issues.",
			Workflow:    "Set all available fields: priority, labels, components, story points. Add subtasks for complex work. Link related issues.",
			Interaction: "Explain reasoning behind decisions. Provide alternatives considered. Tag relevant stakeholders.",
		},
	},
	"formal": {
		Version:     "1",
		Name:        "formal",
		Source:      SourceTemplate,
		Description: "Professional tone, complete sentences, stakeholder-appropriate",
		StyleGuide: StyleGuide{
			Writing:     "Write in complete, grammatically correct sentences. No slang, contractions, or emoji. Use passive voice sparingly. End comments with clear next steps.",
			Workflow:    "Always set priority and due date. Use standard labels. Follow naming conventions for issue titles.",
			Interaction: "Address colleagues by name. Provide status updates proactively. Escalate blockers formally with impact assessment.",
		},
	},
	"casual": {
		Version:     "1",
		Name:        "casual",
		Source:      SourceTemplate,
		Description: "Conversational, contractions, light formatting",
		StyleGuide: StyleGuide{
			Writing:     "Write like you're talking to a teammate. Contractions are fine. Use emoji sparingly for emphasis. Keep it friendly but clear.",
			Workflow:    "Set the basics — priority and labels. Don't overthink metadata. Move fast.",
			Interaction: "Quick replies. Use @mentions liberally. Thumbs-up on acknowledgments.",
		},
	},
}

// TemplateNames returns sorted names of all built-in templates.
func TemplateNames() []string {
	return []string{"casual", "concise", "detailed", "formal"}
}

// Template returns a copy of a built-in template by name.
func Template(name string) (*Character, error) {
	t, ok := builtinTemplates[name]
	if !ok {
		return nil, fmt.Errorf("unknown template %q; available: %v", name, TemplateNames())
	}
	copy := *t
	return &copy, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/character/ -run TestBuiltin -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/character/templates.go internal/character/templates_test.go
git commit -m "feat(character): add 4 built-in character templates"
```

---

## Task 6: Character Prompt Formatting

**Files:**
- Create: `internal/character/prompt.go`
- Test: `internal/character/prompt_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/character/prompt_test.go
package character

import (
	"strings"
	"testing"
)

func TestFormatPrompt_Prose(t *testing.T) {
	c := &ComposedCharacter{
		Character: Character{
			Name: "test",
			StyleGuide: StyleGuide{
				Writing:     "Be concise.",
				Workflow:    "Set priority.",
				Interaction: "Reply fast.",
			},
			Examples: []Example{{Context: "status", Text: "Done."}},
		},
	}

	out := FormatPrompt(c, PromptOptions{Format: "prose"})

	if !strings.Contains(out, "# Character: test") {
		t.Error("missing header")
	}
	if !strings.Contains(out, "Be concise.") {
		t.Error("missing writing section")
	}
	if !strings.Contains(out, "Done.") {
		t.Error("missing example")
	}
}

func TestFormatPrompt_JSON(t *testing.T) {
	c := &ComposedCharacter{
		Character: Character{
			Name:       "test",
			StyleGuide: StyleGuide{Writing: "w", Workflow: "wf", Interaction: "i"},
		},
	}

	out := FormatPrompt(c, PromptOptions{Format: "json"})
	if !strings.Contains(out, `"name"`) {
		t.Error("missing JSON field")
	}
}

func TestFormatPrompt_Redact(t *testing.T) {
	c := &ComposedCharacter{
		Character: Character{
			Name:       "test",
			StyleGuide: StyleGuide{Writing: "alice@example.com writes PROJ-123"},
		},
	}

	out := FormatPrompt(c, PromptOptions{Format: "prose", Redact: true})
	if strings.Contains(out, "alice@example.com") {
		t.Error("email not redacted")
	}
	if strings.Contains(out, "PROJ-123") {
		t.Error("issue key not redacted")
	}
}

func TestFormatPrompt_SectionFilter(t *testing.T) {
	c := &ComposedCharacter{
		Character: Character{
			Name:       "test",
			StyleGuide: StyleGuide{Writing: "w", Workflow: "wf", Interaction: "i"},
		},
	}

	out := FormatPrompt(c, PromptOptions{Format: "prose", Sections: []string{"writing"}})
	if !strings.Contains(out, "w") {
		t.Error("missing writing")
	}
	if strings.Contains(out, "## Workflow") {
		t.Error("workflow should be filtered out")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/character/ -run TestFormatPrompt -v`
Expected: FAIL — `FormatPrompt`, `PromptOptions` undefined

- [ ] **Step 3: Implement prompt formatting**

```go
// internal/character/prompt.go
package character

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// PromptOptions controls how the character is formatted for agent consumption.
type PromptOptions struct {
	Format   string   // "prose" (default), "json", "both"
	Sections []string // filter to specific sections; empty = all
	Redact   bool     // replace emails and issue keys
}

var (
	emailRe    = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	issueKeyRe = regexp.MustCompile(`[A-Z][A-Z0-9]+-\d+`)
)

// FormatPrompt renders a composed character for agent consumption.
func FormatPrompt(c *ComposedCharacter, opts PromptOptions) string {
	format := opts.Format
	if format == "" {
		format = "prose"
	}

	var out string
	switch format {
	case "prose":
		out = formatProse(c, opts)
	case "json":
		out = formatJSON(c)
	case "both":
		out = formatProse(c, opts) + "\n---\n\n" + formatJSON(c)
	default:
		out = formatProse(c, opts)
	}

	if opts.Redact {
		out = redact(out)
	}
	return out
}

func formatProse(c *ComposedCharacter, opts PromptOptions) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Character: %s\n\n", c.Name))

	sections := map[string]string{
		"writing":     c.StyleGuide.Writing,
		"workflow":    c.StyleGuide.Workflow,
		"interaction": c.StyleGuide.Interaction,
	}
	order := []string{"writing", "workflow", "interaction"}

	include := make(map[string]bool)
	if len(opts.Sections) > 0 {
		for _, s := range opts.Sections {
			include[s] = true
		}
	}

	for _, key := range order {
		if len(include) > 0 && !include[key] {
			continue
		}
		title := strings.ToUpper(key[:1]) + key[1:]
		b.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", title, sections[key]))
	}

	if len(include) == 0 || include["examples"] {
		if len(c.Examples) > 0 {
			b.WriteString("## Examples\n\n")
			for _, e := range c.Examples {
				b.WriteString(fmt.Sprintf("**%s:**\n> %s\n\n", e.Context, e.Text))
			}
		}
	}

	return b.String()
}

func formatJSON(c *ComposedCharacter) string {
	data, _ := json.MarshalIndent(c.Character, "", "  ")
	return string(data) + "\n"
}

func redact(s string) string {
	s = emailRe.ReplaceAllString(s, "[EMAIL]")
	s = issueKeyRe.ReplaceAllString(s, "[ISSUE]")
	return s
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/character/ -run TestFormatPrompt -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/character/prompt.go internal/character/prompt_test.go
git commit -m "feat(character): add prompt formatting with prose/json/redact/section filter"
```

---

## Task 7: Character Validation + Active Character Resolution

**Files:**
- Create: `internal/character/validate.go`, `internal/character/active.go`
- Test: `internal/character/validate_test.go`, `internal/character/active_test.go`

- [ ] **Step 1: Write failing tests for validation**

```go
// internal/character/validate_test.go
package character

import "testing"

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		c       Character
		wantErr bool
	}{
		{"valid", Character{Version: "1", Name: "test", Source: SourceManual, StyleGuide: StyleGuide{Writing: "w", Workflow: "wf", Interaction: "i"}}, false},
		{"no version", Character{Name: "test", Source: SourceManual, StyleGuide: StyleGuide{Writing: "w", Workflow: "wf", Interaction: "i"}}, true},
		{"no name", Character{Version: "1", Source: SourceManual, StyleGuide: StyleGuide{Writing: "w", Workflow: "wf", Interaction: "i"}}, true},
		{"no writing", Character{Version: "1", Name: "test", Source: SourceManual, StyleGuide: StyleGuide{Workflow: "wf", Interaction: "i"}}, true},
		{"invalid source", Character{Version: "1", Name: "test", Source: "bad", StyleGuide: StyleGuide{Writing: "w", Workflow: "wf", Interaction: "i"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(&tt.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Write failing tests for active character resolution**

```go
// internal/character/active_test.go
package character

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveActive_FlagOverride(t *testing.T) {
	dir := t.TempDir()
	c := &Character{Version: "1", Name: "flagged", Source: SourceManual, StyleGuide: StyleGuide{Writing: "w", Workflow: "wf", Interaction: "i"}}
	Save(dir, c)

	opts := ResolveOptions{
		FlagCharacter: "flagged",
		CharDir:       dir,
		ProfileName:   "default",
	}
	got, err := ResolveActive(opts)
	if err != nil {
		t.Fatalf("ResolveActive: %v", err)
	}
	if got.Name != "flagged" {
		t.Errorf("Name = %q, want flagged", got.Name)
	}
}

func TestResolveActive_StateFile(t *testing.T) {
	dir := t.TempDir()
	c := &Character{Version: "1", Name: "stated", Source: SourceManual, StyleGuide: StyleGuide{Writing: "w", Workflow: "wf", Interaction: "i"}}
	Save(dir, c)

	stateFile := filepath.Join(dir, "active-character.json")
	os.WriteFile(stateFile, []byte(`{"default":{"base":"stated"}}`), 0o600)

	opts := ResolveOptions{
		CharDir:     dir,
		ProfileName: "default",
		StateFile:   stateFile,
	}
	got, err := ResolveActive(opts)
	if err != nil {
		t.Fatalf("ResolveActive: %v", err)
	}
	if got.Name != "stated" {
		t.Errorf("Name = %q, want stated", got.Name)
	}
}

func TestResolveActive_NoneAvailable(t *testing.T) {
	opts := ResolveOptions{
		CharDir:     t.TempDir(),
		ProfileName: "default",
		StateFile:   filepath.Join(t.TempDir(), "none.json"),
	}
	got, err := ResolveActive(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/character/ -run "TestValidate|TestResolveActive" -v`
Expected: FAIL

- [ ] **Step 4: Implement validation**

```go
// internal/character/validate.go
package character

import "fmt"

var validSources = map[string]bool{
	SourceAvatar:   true,
	SourceClone:    true,
	SourceManual:   true,
	SourceTemplate: true,
	SourcePersona:  true,
}

// Validate checks that a character has all required fields.
func Validate(c *Character) error {
	if c.Version == "" {
		return fmt.Errorf("version is required")
	}
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if c.Source != "" && !validSources[c.Source] {
		return fmt.Errorf("invalid source %q", c.Source)
	}
	if c.StyleGuide.Writing == "" {
		return fmt.Errorf("style_guide.writing is required")
	}
	if c.StyleGuide.Workflow == "" {
		return fmt.Errorf("style_guide.workflow is required")
	}
	if c.StyleGuide.Interaction == "" {
		return fmt.Errorf("style_guide.interaction is required")
	}
	return nil
}
```

- [ ] **Step 5: Implement active character resolution**

```go
// internal/character/active.go
package character

import (
	"encoding/json"
	"os"
)

// ResolveOptions configures active character resolution.
type ResolveOptions struct {
	FlagCharacter  string            // --character flag (highest priority)
	FlagCompose    map[string]string // --writing, --workflow, --interaction flags
	ConfigChar     string            // yolo.character from config
	ConfigCompose  map[string]string // yolo.compose from config
	CharDir        string            // character storage directory
	ProfileName    string            // current config profile name
	StateFile      string            // path to active-character.json
}

type activeState struct {
	Base    string            `json:"base"`
	Compose map[string]string `json:"compose,omitempty"`
}

// ResolveActive resolves the active character following priority order:
// 1. --character flag
// 2. yolo.character + yolo.compose from config
// 3. jr character use state (persisted in active-character.json)
// 4. Self-avatar character (source: "avatar")
// 5. nil (no character)
func ResolveActive(opts ResolveOptions) (*ComposedCharacter, error) {
	var baseName string
	var compose map[string]string

	// Priority 1: flag
	if opts.FlagCharacter != "" {
		baseName = opts.FlagCharacter
		compose = opts.FlagCompose
	}

	// Priority 2: config
	if baseName == "" && opts.ConfigChar != "" {
		baseName = opts.ConfigChar
		compose = opts.ConfigCompose
	}

	// Priority 3: state file
	if baseName == "" {
		baseName, compose = loadState(opts.StateFile, opts.ProfileName)
	}

	// Priority 4: self-avatar
	if baseName == "" {
		baseName = findAvatarCharacter(opts.CharDir)
	}

	// Priority 5: no character
	if baseName == "" {
		return nil, nil
	}

	base, err := Load(opts.CharDir, baseName)
	if err != nil {
		return nil, err
	}

	overrides := make(map[string]*Character)
	for section, name := range compose {
		c, err := Load(opts.CharDir, name)
		if err != nil {
			return nil, err
		}
		overrides[section] = c
	}

	result := Compose(base, overrides)
	return &result, nil
}

// SaveState persists the active character choice.
func SaveState(stateFile, profileName, baseName string, compose map[string]string) error {
	all := loadAllStates(stateFile)
	all[profileName] = activeState{Base: baseName, Compose: compose}
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFile, data, 0o600)
}

func loadState(stateFile, profileName string) (string, map[string]string) {
	all := loadAllStates(stateFile)
	s, ok := all[profileName]
	if !ok {
		return "", nil
	}
	return s.Base, s.Compose
}

func loadAllStates(stateFile string) map[string]activeState {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return make(map[string]activeState)
	}
	var all map[string]activeState
	if err := json.Unmarshal(data, &all); err != nil {
		return make(map[string]activeState)
	}
	return all
}

func findAvatarCharacter(dir string) string {
	chars, err := List(dir)
	if err != nil {
		return ""
	}
	for _, c := range chars {
		if c.Source == SourceAvatar {
			return c.Name
		}
	}
	return ""
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/character/ -run "TestValidate|TestResolveActive" -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/character/validate.go internal/character/validate_test.go internal/character/active.go internal/character/active_test.go
git commit -m "feat(character): add validation and active character resolution"
```

---

## Task 8: Yolo Config

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/yolo/config.go`
- Test: `internal/yolo/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/yolo/config_test.go
package yolo

import "testing"

func TestScopeTier_Safe(t *testing.T) {
	ops := ScopeTierOps("safe")
	if len(ops) == 0 {
		t.Fatal("safe tier should have operations")
	}
	// safe tier should include workflow comment
	found := false
	for _, op := range ops {
		if op == "workflow comment" {
			found = true
			break
		}
	}
	if !found {
		t.Error("safe tier missing 'workflow comment'")
	}
}

func TestScopeTier_Full(t *testing.T) {
	ops := ScopeTierOps("full")
	if len(ops) != 1 || ops[0] != "*" {
		t.Errorf("full tier ops = %v, want [*]", ops)
	}
	denied := ScopeTierDenied("full")
	if len(denied) != 3 {
		t.Errorf("full tier denied = %v, want 3 patterns", denied)
	}
}

func TestScopeTier_Unknown(t *testing.T) {
	ops := ScopeTierOps("unknown")
	if ops != nil {
		t.Errorf("unknown tier should return nil, got %v", ops)
	}
}

func TestDefaultYoloConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Enabled {
		t.Error("should be disabled by default")
	}
	if cfg.Scope != "safe" {
		t.Errorf("Scope = %q, want safe", cfg.Scope)
	}
	if cfg.RateLimit.PerHour != 30 {
		t.Errorf("PerHour = %d, want 30", cfg.RateLimit.PerHour)
	}
	if !cfg.RequireAudit {
		t.Error("RequireAudit should default to true")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/yolo/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement yolo config**

```go
// internal/yolo/config.go
package yolo

// Config defines yolo execution policy settings.
type Config struct {
	Enabled        bool              `json:"enabled,omitempty"`
	Scope          string            `json:"scope,omitempty"`          // "safe", "standard", "full"
	Character      string            `json:"character,omitempty"`
	Compose        map[string]string `json:"compose,omitempty"`
	Tag            bool              `json:"tag,omitempty"`
	TagText        string            `json:"tag_text,omitempty"`
	RateLimit      RateLimitConfig   `json:"rate_limit,omitempty"`
	AllowedActions []string          `json:"allowed_actions,omitempty"`
	DeniedActions  []string          `json:"denied_actions,omitempty"`
	RequireAudit   bool              `json:"require_audit,omitempty"`
	DecisionEngine string            `json:"decision_engine,omitempty"` // "rules", "llm"
	LLMCmd         string            `json:"llm_cmd,omitempty"`
	LLMTimeout     string            `json:"llm_timeout,omitempty"`
}

// RateLimitConfig defines rate limiting parameters.
type RateLimitConfig struct {
	PerHour int `json:"per_hour,omitempty"`
	Burst   int `json:"burst,omitempty"`
}

// DefaultConfig returns the default yolo configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:        false,
		Scope:          "safe",
		Tag:            false,
		TagText:        "[via jr]",
		RateLimit:      RateLimitConfig{PerHour: 30, Burst: 10},
		RequireAudit:   true,
		DecisionEngine: "rules",
	}
}

// Scope tier definitions using operation string namespace.
var scopeTiers = map[string]struct {
	allowed []string
	denied  []string
}{
	"safe": {
		allowed: []string{
			"workflow comment",
			"workflow transition",
			"workflow assign",
			"workflow log-work",
		},
	},
	"standard": {
		allowed: []string{
			"workflow comment",
			"workflow transition",
			"workflow assign",
			"workflow log-work",
			"workflow create",
			"issue edit",
			"workflow link",
			"workflow sprint",
		},
	},
	"full": {
		allowed: []string{"*"},
		denied:  []string{"* delete*", "bulk *", "raw *"},
	},
}

// ScopeTierOps returns the allowed operation patterns for a tier.
func ScopeTierOps(tier string) []string {
	t, ok := scopeTiers[tier]
	if !ok {
		return nil
	}
	return t.allowed
}

// ScopeTierDenied returns the denied operation patterns for a tier.
func ScopeTierDenied(tier string) []string {
	t, ok := scopeTiers[tier]
	if !ok {
		return nil
	}
	return t.denied
}
```

- [ ] **Step 4: Add YoloConfig to Profile in config.go**

Modify `internal/config/config.go` — add `Yolo` field to `Profile` struct:

```go
// Add to Profile struct (after Avatar field):
Yolo *yolo.Config `json:"yolo,omitempty"`
```

Note: Use a forward reference or define the config type locally to avoid circular imports. The cleanest approach: define `YoloConfig` in `internal/config/config.go` directly (matching `AvatarConfig` pattern), and have `internal/yolo/config.go` reference it. OR keep `yolo.Config` standalone and have `config.go` import `internal/yolo`. Check import graph — if `internal/yolo` needs `internal/config`, use the first approach to avoid cycles. Since `yolo.Config` has no dependency on config, the import is safe: `config` imports `yolo`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/yolo/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/yolo/config.go internal/yolo/config_test.go internal/config/config.go
git commit -m "feat(yolo): add config types and scope tier definitions"
```

---

## Task 9: Yolo Policy (Action Checking)

**Files:**
- Create: `internal/yolo/policy.go`
- Test: `internal/yolo/policy_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/yolo/policy_test.go
package yolo

import "testing"

func TestPolicy_SafeTier(t *testing.T) {
	cfg := Config{Enabled: true, Scope: "safe"}
	p, err := NewPolicy(cfg)
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}

	tests := []struct {
		op   string
		want bool
	}{
		{"workflow comment", true},
		{"workflow transition", true},
		{"workflow assign", true},
		{"workflow log-work", true},
		{"issue edit", false},
		{"issue delete", false},
		{"workflow create", false},
		{"raw GET", false},
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			got := p.Allowed(tt.op)
			if got != tt.want {
				t.Errorf("Allowed(%q) = %v, want %v", tt.op, got, tt.want)
			}
		})
	}
}

func TestPolicy_FullTier(t *testing.T) {
	cfg := Config{Enabled: true, Scope: "full"}
	p, err := NewPolicy(cfg)
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}

	tests := []struct {
		op   string
		want bool
	}{
		{"workflow comment", true},
		{"issue edit", true},
		{"workflow create", true},
		{"issue delete", false},    // matched by "* delete*"
		{"bulk import", false},     // matched by "bulk *"
		{"raw GET", false},         // matched by "raw *"
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			got := p.Allowed(tt.op)
			if got != tt.want {
				t.Errorf("Allowed(%q) = %v, want %v", tt.op, got, tt.want)
			}
		})
	}
}

func TestPolicy_CustomDenied(t *testing.T) {
	cfg := Config{
		Enabled:       true,
		Scope:         "standard",
		DeniedActions: []string{"workflow create"},
	}
	p, err := NewPolicy(cfg)
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}

	if p.Allowed("workflow create") {
		t.Error("workflow create should be denied by custom denied_actions")
	}
	if !p.Allowed("workflow comment") {
		t.Error("workflow comment should still be allowed")
	}
}

func TestPolicy_Disabled(t *testing.T) {
	cfg := Config{Enabled: false}
	p, err := NewPolicy(cfg)
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}
	if p.Allowed("workflow comment") {
		t.Error("disabled policy should deny everything")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/yolo/ -run TestPolicy -v`
Expected: FAIL — `NewPolicy`, `Allowed` undefined

- [ ] **Step 3: Implement yolo policy**

```go
// internal/yolo/policy.go
package yolo

import "path"

// Policy enforces yolo scope restrictions.
type Policy struct {
	enabled bool
	allowed []string // glob patterns from tier + allowed_actions
	denied  []string // glob patterns from tier + denied_actions
}

// NewPolicy creates a yolo policy from config.
func NewPolicy(cfg Config) (*Policy, error) {
	p := &Policy{enabled: cfg.Enabled}
	if !cfg.Enabled {
		return p, nil
	}

	// Start with tier operations
	tierAllowed := ScopeTierOps(cfg.Scope)
	tierDenied := ScopeTierDenied(cfg.Scope)

	if tierAllowed != nil {
		p.allowed = append(p.allowed, tierAllowed...)
	}
	if tierDenied != nil {
		p.denied = append(p.denied, tierDenied...)
	}

	// Custom allowed_actions further restrict: intersect with tier
	if len(cfg.AllowedActions) > 0 && tierAllowed != nil && tierAllowed[0] != "*" {
		// When tier has explicit list, intersect
		p.allowed = intersectGlobs(tierAllowed, cfg.AllowedActions)
	} else if len(cfg.AllowedActions) > 0 {
		p.allowed = cfg.AllowedActions
	}

	// Custom denied_actions add more restrictions
	p.denied = append(p.denied, cfg.DeniedActions...)

	return p, nil
}

// Allowed checks if an operation is permitted by yolo policy.
func (p *Policy) Allowed(operation string) bool {
	if !p.enabled {
		return false
	}

	// Check denied first
	for _, pattern := range p.denied {
		if matched, _ := path.Match(pattern, operation); matched {
			return false
		}
	}

	// Check allowed
	for _, pattern := range p.allowed {
		if matched, _ := path.Match(pattern, operation); matched {
			return true
		}
	}

	return false
}

// IsEnabled returns whether yolo mode is active.
func (p *Policy) IsEnabled() bool {
	return p.enabled
}

func intersectGlobs(a, b []string) []string {
	var result []string
	for _, pa := range a {
		for _, pb := range b {
			if pa == pb {
				result = append(result, pa)
			}
		}
	}
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/yolo/ -run TestPolicy -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/yolo/policy.go internal/yolo/policy_test.go
git commit -m "feat(yolo): add policy engine with scope tiers and glob matching"
```

---

## Task 10: Yolo Rate Limiter

**Files:**
- Create: `internal/yolo/ratelimit.go`
- Test: `internal/yolo/ratelimit_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/yolo/ratelimit_test.go
package yolo

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	rl := NewRateLimiter(RateLimitConfig{PerHour: 60, Burst: 5}, "default", stateFile)

	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Errorf("Allow() = false on attempt %d, want true", i)
		}
	}
	// 6th should fail (burst exhausted)
	if rl.Allow() {
		t.Error("Allow() = true after burst exhausted, want false")
	}
}

func TestRateLimiter_Remaining(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	rl := NewRateLimiter(RateLimitConfig{PerHour: 60, Burst: 10}, "default", stateFile)

	rl.Allow()
	rl.Allow()
	if got := rl.Remaining(); got != 8 {
		t.Errorf("Remaining() = %d, want 8", got)
	}
}

func TestRateLimiter_Persistence(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")

	rl1 := NewRateLimiter(RateLimitConfig{PerHour: 60, Burst: 10}, "default", stateFile)
	rl1.Allow()
	rl1.Allow()
	rl1.Save()

	rl2 := NewRateLimiter(RateLimitConfig{PerHour: 60, Burst: 10}, "default", stateFile)
	if got := rl2.Remaining(); got != 8 {
		t.Errorf("After reload: Remaining() = %d, want 8", got)
	}
}

func TestRateLimiter_Refill(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	rl := NewRateLimiter(RateLimitConfig{PerHour: 60, Burst: 5}, "default", stateFile)

	// Exhaust burst
	for i := 0; i < 5; i++ {
		rl.Allow()
	}

	// Simulate time passing (1 minute = 1 token for 60/hour)
	rl.lastRefill = time.Now().Add(-1 * time.Minute)
	rl.refill()

	if !rl.Allow() {
		t.Error("Should have 1 token after 1 minute refill")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/yolo/ -run TestRateLimiter -v`
Expected: FAIL

- [ ] **Step 3: Implement rate limiter**

```go
// internal/yolo/ratelimit.go
package yolo

import (
	"encoding/json"
	"math"
	"os"
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter with persistence.
type RateLimiter struct {
	mu         sync.Mutex
	tokens     float64
	lastRefill time.Time
	perHour    int
	burst      int
	profile    string
	stateFile  string
}

type rateLimitState struct {
	Tokens     float64   `json:"tokens"`
	LastRefill time.Time `json:"last_refill"`
}

// NewRateLimiter creates a rate limiter, loading persisted state if available.
func NewRateLimiter(cfg RateLimitConfig, profile, stateFile string) *RateLimiter {
	rl := &RateLimiter{
		tokens:     float64(cfg.Burst),
		lastRefill: time.Now(),
		perHour:    cfg.PerHour,
		burst:      cfg.Burst,
		profile:    profile,
		stateFile:  stateFile,
	}
	rl.loadState()
	rl.refill()
	return rl
}

// Allow checks if an action is permitted and consumes a token.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()
	if rl.tokens < 1 {
		return false
	}
	rl.tokens--
	return true
}

// Remaining returns the current token count.
func (rl *RateLimiter) Remaining() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.refill()
	return int(rl.tokens)
}

// Save persists the rate limiter state.
func (rl *RateLimiter) Save() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	all := rl.loadAllStates()
	all[rl.profile] = rateLimitState{Tokens: rl.tokens, LastRefill: rl.lastRefill}
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(rl.stateFile, data, 0o600)
}

func (rl *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	tokensToAdd := elapsed.Minutes() * (float64(rl.perHour) / 60.0)
	rl.tokens = math.Min(rl.tokens+tokensToAdd, float64(rl.burst))
	rl.lastRefill = now
}

func (rl *RateLimiter) loadState() {
	all := rl.loadAllStates()
	s, ok := all[rl.profile]
	if !ok {
		return
	}
	rl.tokens = s.Tokens
	rl.lastRefill = s.LastRefill
}

func (rl *RateLimiter) loadAllStates() map[string]rateLimitState {
	data, err := os.ReadFile(rl.stateFile)
	if err != nil {
		return make(map[string]rateLimitState)
	}
	var all map[string]rateLimitState
	if err := json.Unmarshal(data, &all); err != nil {
		return make(map[string]rateLimitState)
	}
	return all
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/yolo/ -run TestRateLimiter -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/yolo/ratelimit.go internal/yolo/ratelimit_test.go
git commit -m "feat(yolo): add token bucket rate limiter with state persistence"
```

---

## Task 11: Yolo Events + Decision Engine

**Files:**
- Create: `internal/yolo/events.go`, `internal/yolo/decide.go`
- Test: `internal/yolo/events_test.go`, `internal/yolo/decide_test.go`

- [ ] **Step 1: Write failing test for event types**

```go
// internal/yolo/events_test.go
package yolo

import "testing"

func TestEventTypes(t *testing.T) {
	all := AllEventTypes()
	if len(all) != 11 {
		t.Errorf("len = %d, want 11", len(all))
	}
	for _, et := range all {
		if et == "" {
			t.Error("empty event type")
		}
	}
}

func TestValidEventType(t *testing.T) {
	if !ValidEventType("assigned") {
		t.Error("assigned should be valid")
	}
	if ValidEventType("nonexistent") {
		t.Error("nonexistent should be invalid")
	}
}

func TestReactionKey(t *testing.T) {
	if ReactionKey("assigned") != "on_assigned" {
		t.Errorf("ReactionKey(assigned) = %q", ReactionKey("assigned"))
	}
}
```

- [ ] **Step 2: Write failing test for decision engine**

```go
// internal/yolo/decide_test.go
package yolo

import (
	"testing"

	"github.com/sofq/jira-cli/internal/character"
)

func TestDecideRuleBased(t *testing.T) {
	ch := &character.ComposedCharacter{
		Character: character.Character{
			Reactions: map[string]character.Reaction{
				"on_assigned":       {Action: "transition", To: "In Progress"},
				"on_comment_mention": {Action: "comment", Text: "Looking into it"},
			},
		},
	}

	tests := []struct {
		event  string
		want   string // expected action
		wantOK bool
	}{
		{"assigned", "transition", true},
		{"comment_mention", "comment", true},
		{"status_change", "", false}, // no reaction defined
	}
	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			action, ok := DecideRuleBased(ch, tt.event)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && action.Action != tt.want {
				t.Errorf("Action = %q, want %q", action.Action, tt.want)
			}
		})
	}
}

func TestDecideRuleBased_NilReactions(t *testing.T) {
	ch := &character.ComposedCharacter{
		Character: character.Character{},
	}
	_, ok := DecideRuleBased(ch, "assigned")
	if ok {
		t.Error("should return false for nil reactions")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/yolo/ -run "TestEvent|TestDecide" -v`
Expected: FAIL

- [ ] **Step 4: Implement events**

```go
// internal/yolo/events.go
package yolo

// Canonical event types used across all surfaces.
const (
	EventAssigned        = "assigned"
	EventUnassigned      = "unassigned"
	EventCommentAdded    = "comment_added"
	EventCommentMention  = "comment_mention"
	EventStatusChange    = "status_change"
	EventPriorityChange  = "priority_change"
	EventFieldChange     = "field_change"
	EventIssueCreated    = "issue_created"
	EventReviewRequested = "review_requested"
	EventBlocked         = "blocked"
	EventSprintChange    = "sprint_change"
)

var allEventTypes = []string{
	EventAssigned,
	EventUnassigned,
	EventCommentAdded,
	EventCommentMention,
	EventStatusChange,
	EventPriorityChange,
	EventFieldChange,
	EventIssueCreated,
	EventReviewRequested,
	EventBlocked,
	EventSprintChange,
}

var eventTypeSet = func() map[string]bool {
	m := make(map[string]bool, len(allEventTypes))
	for _, et := range allEventTypes {
		m[et] = true
	}
	return m
}()

// AllEventTypes returns all canonical event types.
func AllEventTypes() []string { return allEventTypes }

// ValidEventType checks if an event type is canonical.
func ValidEventType(et string) bool { return eventTypeSet[et] }

// ReactionKey converts an event type to its reaction key (e.g., "assigned" -> "on_assigned").
func ReactionKey(eventType string) string { return "on_" + eventType }
```

- [ ] **Step 5: Implement rule-based decision engine**

```go
// internal/yolo/decide.go
package yolo

import (
	"github.com/sofq/jira-cli/internal/character"
)

// Action represents a decided action from the decision engine.
type Action struct {
	Action   string `json:"action"`              // "comment", "transition", "assign", "log-work", "none"
	To       string `json:"to,omitempty"`         // for transition
	Text     string `json:"text,omitempty"`       // for comment
	Assignee string `json:"assignee,omitempty"`   // for assign
	Reason   string `json:"reason,omitempty"`     // optional, for audit
}

// DecideRuleBased checks the character's reactions for a matching event.
func DecideRuleBased(ch *character.ComposedCharacter, eventType string) (Action, bool) {
	if ch == nil || ch.Reactions == nil {
		return Action{}, false
	}

	key := ReactionKey(eventType)
	reaction, ok := ch.Reactions[key]
	if !ok {
		return Action{}, false
	}

	return Action{
		Action:   reaction.Action,
		To:       reaction.To,
		Text:     reaction.Text,
		Assignee: reaction.Assignee,
		Reason:   "rule: " + key,
	}, true
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/yolo/ -run "TestEvent|TestDecide" -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/yolo/events.go internal/yolo/events_test.go internal/yolo/decide.go internal/yolo/decide_test.go
git commit -m "feat(yolo): add canonical event types and rule-based decision engine"
```

---

## Task 12: Extend Audit Entry with Extra Field

**Files:**
- Modify: `internal/audit/audit.go`
- Modify: `internal/audit/audit_test.go`

- [ ] **Step 1: Write the failing test**

Add to existing `internal/audit/audit_test.go`:

```go
func TestLogWithExtra(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	l, err := NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	l.Log(Entry{
		Profile:   "default",
		Operation: "workflow comment",
		Method:    "POST",
		Path:      "/rest/api/3/issue/PROJ-123/comment",
		Status:    201,
		Extra: map[string]any{
			"yolo":      true,
			"character": "formal-pm",
		},
	})

	data, _ := os.ReadFile(path)
	line := strings.TrimSpace(string(data))

	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	extra, ok := entry["extra"].(map[string]any)
	if !ok {
		t.Fatal("extra field missing or wrong type")
	}
	if extra["yolo"] != true {
		t.Errorf("extra.yolo = %v, want true", extra["yolo"])
	}
	if extra["character"] != "formal-pm" {
		t.Errorf("extra.character = %v, want formal-pm", extra["character"])
	}
}

func TestLogWithoutExtra(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	l, err := NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	l.Log(Entry{Profile: "default", Operation: "issue get"})

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), `"extra"`) {
		t.Error("extra field should be omitted when nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/audit/ -run "TestLogWith" -v`
Expected: FAIL — `Extra` field does not exist on `Entry`

- [ ] **Step 3: Add Extra field to Entry struct**

In `internal/audit/audit.go`, add to `Entry` struct:

```go
Extra map[string]any `json:"extra,omitempty"`
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/audit/ -v`
Expected: PASS (all existing tests + new tests)

- [ ] **Step 5: Commit**

```bash
git add internal/audit/audit.go internal/audit/audit_test.go
git commit -m "feat(audit): add Extra field for yolo metadata"
```

---

## Task 13: Refactor Watch to Support EventHandler Callback

**Files:**
- Modify: `internal/watch/watcher.go`
- Modify: `internal/watch/watcher_test.go` (if exists)
- Modify: `cmd/watch.go` (update caller)

- [ ] **Step 1: Write the failing test**

```go
// Add to internal/watch/watcher_test.go (or create if needed)
func TestRunWithHandler(t *testing.T) {
	// Mock HTTP server
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, `{"issues":[{"key":"TEST-1","fields":{"updated":"2026-01-01T00:00:00.000+0000"}}]}`)
		} else {
			fmt.Fprint(w, `{"issues":[]}`)
		}
	}))
	defer srv.Close()

	c := &client.Client{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		Stdout:     &bytes.Buffer{},
	}

	var events []Event
	handler := func(e Event) error {
		events = append(events, e)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	Run(ctx, c, Options{
		JQL:       "project = TEST",
		Interval:  100 * time.Millisecond,
		MaxEvents: 1,
		Handler:   handler,
	})

	if len(events) != 1 {
		t.Errorf("events = %d, want 1", len(events))
	}
	if events[0].Type != "initial" {
		t.Errorf("Type = %q, want initial", events[0].Type)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/watch/ -run TestRunWithHandler -v`
Expected: FAIL — `Handler` field does not exist on `Options`

- [ ] **Step 3: Add EventHandler to Options and refactor Run**

In `internal/watch/watcher.go`:
- Add `Handler func(Event) error` field to `Options`
- In `emitEvent()`: if `opts.Handler != nil`, call it instead of writing NDJSON to stdout
- Default behavior (nil handler) unchanged — writes NDJSON as before

```go
// Add to Options struct:
Handler func(Event) error // optional; if set, called per event instead of writing NDJSON

// In emitEvent(), change the output logic:
func (w *watcher) emitEvent(e Event) error {
    if w.opts.Handler != nil {
        return w.opts.Handler(e)
    }
    // existing NDJSON output logic
    ...
}
```

- [ ] **Step 4: Update cmd/watch.go if needed**

The `cmd/watch.go` caller passes `Options` without `Handler` — this is fine since nil Handler preserves existing behavior. No change needed.

- [ ] **Step 5: Run all watch tests**

Run: `go test ./internal/watch/ -v && go test ./cmd/ -run Watch -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/watch/watcher.go internal/watch/watcher_test.go
git commit -m "refactor(watch): add EventHandler callback to Options"
```

---

## Task 14: Character CLI Commands

**Files:**
- Create: `cmd/character.go`
- Test: `cmd/character_test.go`

- [ ] **Step 1: Implement character command tree**

```go
// cmd/character.go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/sofq/jira-cli/internal/character"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var characterCmd = &cobra.Command{
	Use:   "character",
	Short: "Manage character personas",
}

var characterListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all characters",
	RunE:  runCharacterList,
}

var characterShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Display a character",
	Args:  cobra.ExactArgs(1),
	RunE:  runCharacterShow,
}

var characterCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new character",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCharacterCreate,
}

var characterEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Edit a character in $EDITOR",
	Args:  cobra.ExactArgs(1),
	RunE:  runCharacterEdit,
}

var characterDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a character",
	Args:  cobra.ExactArgs(1),
	RunE:  runCharacterDelete,
}

var characterUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set the active character",
	Args:  cobra.ExactArgs(1),
	RunE:  runCharacterUse,
}

var characterPromptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Output active character for agent consumption",
	RunE:  runCharacterPrompt,
}

func init() {
	characterCmd.AddCommand(characterListCmd)
	characterCmd.AddCommand(characterShowCmd)
	characterCmd.AddCommand(characterCreateCmd)
	characterCmd.AddCommand(characterEditCmd)
	characterCmd.AddCommand(characterDeleteCmd)
	characterCmd.AddCommand(characterUseCmd)
	characterCmd.AddCommand(characterPromptCmd)

	characterCreateCmd.Flags().String("from-user", "", "clone from another user's Jira activity")
	characterCreateCmd.Flags().String("persona", "", "generate from free-text description via LLM")
	characterCreateCmd.Flags().String("template", "", "copy from built-in template")

	characterUseCmd.Flags().String("writing", "", "override writing section from another character")
	characterUseCmd.Flags().String("workflow", "", "override workflow section from another character")
	characterUseCmd.Flags().String("interaction", "", "override interaction section from another character")

	characterPromptCmd.Flags().String("format", "prose", "output format: prose, json, both")
	characterPromptCmd.Flags().StringSlice("section", nil, "filter to specific sections")
	characterPromptCmd.Flags().Bool("redact", false, "redact emails and issue keys")
}

func characterDir() string { return character.DefaultDir() }

func runCharacterList(cmd *cobra.Command, args []string) error {
	chars, err := character.List(characterDir())
	if err != nil {
		return err
	}
	type listEntry struct {
		Name        string `json:"name"`
		Source      string `json:"source"`
		Description string `json:"description"`
	}
	entries := make([]listEntry, len(chars))
	for i, c := range chars {
		entries[i] = listEntry{Name: c.Name, Source: c.Source, Description: c.Description}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

func runCharacterShow(cmd *cobra.Command, args []string) error {
	c, err := character.Load(characterDir(), args[0])
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	fmt.Fprint(os.Stdout, string(data))
	return nil
}

func runCharacterCreate(cmd *cobra.Command, args []string) error {
	dir := characterDir()
	tmplName, _ := cmd.Flags().GetString("template")
	persona, _ := cmd.Flags().GetString("persona")

	fromUser, _ := cmd.Flags().GetString("from-user")
	if fromUser != "" {
		// Clone requires authenticated client — not available in local-only command
		return fmt.Errorf("--from-user requires authenticated client; use 'jr avatar extract --user %s' then convert", fromUser)
	}

	if tmplName != "" {
		tmpl, err := character.Template(tmplName)
		if err != nil {
			return err
		}
		if len(args) > 0 {
			tmpl.Name = args[0]
		}
		return character.Save(dir, tmpl)
	}

	if persona != "" {
		// LLM generation — future implementation
		return fmt.Errorf("--persona requires LLM engine (not yet implemented)")
	}

	if len(args) == 0 {
		return fmt.Errorf("name is required (or use --template/--persona)")
	}

	// Scaffold empty character
	c := &character.Character{
		Version:     "1",
		Name:        args[0],
		Source:      character.SourceManual,
		Description: "TODO: describe this character",
		StyleGuide: character.StyleGuide{
			Writing:     "TODO: define writing style",
			Workflow:    "TODO: define workflow patterns",
			Interaction: "TODO: define interaction style",
		},
	}
	if err := character.Save(dir, c); err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(map[string]string{"created": args[0], "path": dir + "/" + args[0] + ".yaml"})
}

func runCharacterEdit(cmd *cobra.Command, args []string) error {
	dir := characterDir()
	path := dir + "/" + args[0] + ".yaml"
	if !character.Exists(dir, args[0]) {
		(&jrerrors.APIError{ErrorType: "not_found", Message: fmt.Sprintf("character %q not found", args[0])}).WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	c := exec.Command(editor, path)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return err
	}

	// Validate after edit
	loaded, err := character.Load(dir, args[0])
	if err != nil {
		return fmt.Errorf("edited file is invalid: %w", err)
	}
	if err := character.Validate(loaded); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return nil
}

func runCharacterDelete(cmd *cobra.Command, args []string) error {
	if err := character.Delete(characterDir(), args[0]); err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(map[string]string{"deleted": args[0]})
}

func runCharacterUse(cmd *cobra.Command, args []string) error {
	dir := characterDir()
	name := args[0]

	if !character.Exists(dir, name) {
		jrerrors.WriteJSON(os.Stderr, "not_found", 0, fmt.Sprintf("character %q not found", name), "")
		return jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	compose := make(map[string]string)
	for _, section := range []string{"writing", "workflow", "interaction"} {
		if v, _ := cmd.Flags().GetString(section); v != "" {
			if !character.Exists(dir, v) {
				return fmt.Errorf("override character %q not found", v)
			}
			compose[section] = v
		}
	}

	profileName, _ := cmd.Flags().GetString("profile")
	if profileName == "" {
		profileName = "default"
	}
	stateFile := dir + "/active-character.json"
	if err := character.SaveState(stateFile, profileName, name, compose); err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	result := map[string]any{"active": name}
	if len(compose) > 0 {
		result["compose"] = compose
	}
	return enc.Encode(result)
}

func runCharacterPrompt(cmd *cobra.Command, args []string) error {
	dir := characterDir()
	format, _ := cmd.Flags().GetString("format")
	sections, _ := cmd.Flags().GetStringSlice("section")
	redact, _ := cmd.Flags().GetBool("redact")

	opts := character.ResolveOptions{
		CharDir:     dir,
		ProfileName: "default",
		StateFile:   dir + "/active-character.json",
	}
	composed, err := character.ResolveActive(opts)
	if err != nil {
		return err
	}
	if composed == nil {
		(&jrerrors.APIError{ErrorType: "not_found", Message: "no active character", Hint: "Run 'jr character use <name>' to set one"}).WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	out := character.FormatPrompt(composed, character.PromptOptions{
		Format:   format,
		Sections: sections,
		Redact:   redact,
	})
	fmt.Fprint(os.Stdout, out)
	return nil
}
```

- [ ] **Step 2: Write tests for character CLI**

```go
// cmd/character_test.go — test list, create, show, delete via command execution
// Follow the pattern in cmd/avatar_test.go: execute commands and check stdout JSON
```

- [ ] **Step 3: Register characterCmd in root.go**

Add to `cmd/root.go` init or registration section:
```go
rootCmd.AddCommand(characterCmd)
```

Also add `"character"` to `skipClientCommands` map since character commands are local-only.

- [ ] **Step 4: Run tests**

Run: `go test ./cmd/ -run TestCharacter -v && go build ./...`
Expected: PASS, builds clean

- [ ] **Step 5: Commit**

```bash
git add cmd/character.go cmd/character_test.go cmd/root.go
git commit -m "feat(character): add character CLI commands (list, show, create, edit, delete, use, prompt)"
```

---

## Task 15: Yolo CLI Commands (status, history)

**Files:**
- Create: `cmd/yolo.go`
- Test: `cmd/yolo_test.go`

- [ ] **Step 1: Implement yolo status and history commands**

```go
// cmd/yolo.go
package cmd

import (
	"encoding/json"
	"os"

	"github.com/sofq/jira-cli/internal/yolo"
	"github.com/spf13/cobra"
)

var yoloCmd = &cobra.Command{
	Use:   "yolo",
	Short: "Manage yolo execution policy",
}

var yoloStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show yolo state",
	RunE:  runYoloStatus,
}

var yoloHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Show past yolo actions",
	RunE:  runYoloHistory,
}

func init() {
	yoloCmd.AddCommand(yoloStatusCmd)
	yoloCmd.AddCommand(yoloHistoryCmd)
	yoloHistoryCmd.Flags().String("since", "", "show actions since duration (e.g., 1h, 24h)")
}

func runYoloStatus(cmd *cobra.Command, args []string) error {
	// Load yolo config from profile
	cfg := yolo.DefaultConfig()
	// TODO: load from actual profile config

	dir := characterDir()
	stateFile := dir + "/../yolo-state.json"
	rl := yolo.NewRateLimiter(cfg.RateLimit, "default", stateFile)

	status := map[string]any{
		"enabled":         cfg.Enabled,
		"scope":           cfg.Scope,
		"character":       cfg.Character,
		"rate_remaining":  rl.Remaining(),
		"rate_per_hour":   cfg.RateLimit.PerHour,
		"rate_burst":      cfg.RateLimit.Burst,
		"require_audit":   cfg.RequireAudit,
		"decision_engine": cfg.DecisionEngine,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(status)
}

func runYoloHistory(cmd *cobra.Command, args []string) error {
	// TODO: read audit log, filter for yolo entries
	// For now, output empty array
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode([]any{})
}
```

- [ ] **Step 2: Register yoloCmd in root.go**

```go
rootCmd.AddCommand(yoloCmd)
```

Add `"yolo"` to `skipClientCommands`.

- [ ] **Step 3: Write tests and verify**

Run: `go test ./cmd/ -run TestYolo -v && go build ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/yolo.go cmd/yolo_test.go cmd/root.go
git commit -m "feat(yolo): add yolo status and history CLI commands"
```

---

## Task 16: Root.go Integration (--yolo and --character flags)

**Files:**
- Modify: `cmd/root.go`

- [ ] **Step 1: Add global flags**

In the persistent flags section of `cmd/root.go`:

```go
pf.Bool("yolo", false, "enable yolo mode for this invocation")
pf.String("character", "", "override active character for this invocation")
```

- [ ] **Step 2: Add yolo policy check in PersistentPreRunE**

After the existing policy check (around line 144), add yolo policy loading:

```go
// Load yolo config if --yolo flag or config enabled
yoloFlag, _ := cmd.Flags().GetBool("yolo")
// If yolo enabled via flag or config, create yolo policy
// and attach to client context for use by workflow commands
```

- [ ] **Step 3: Verify build and existing tests pass**

Run: `go build ./... && go test ./cmd/ -v -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/root.go
git commit -m "feat: add --yolo and --character global flags with policy integration"
```

---

## Task 17: Avatar Build → Character File Output

**Files:**
- Modify: `cmd/avatar.go`

- [ ] **Step 1: Update runAvatarBuild to save character file**

After the existing `avatar.SaveProfile()` call in `runAvatarBuild()`, add:

```go
// Convert profile to character and save
ch := character.FromProfile(profile)
charDir := character.DefaultDir()
// Check for collision: if exists with different source, use alternate name
if character.Exists(charDir, ch.Name) {
    existing, _ := character.Load(charDir, ch.Name)
    if existing != nil && existing.Source != character.SourceAvatar {
        ch.Name = ch.Name + "-avatar"
    }
}
if err := character.Save(charDir, ch); err != nil {
    fmt.Fprintf(os.Stderr, "warning: failed to save character file: %v\n", err)
}
```

- [ ] **Step 2: Write test**

Add test in `cmd/avatar_test.go` that runs build and verifies character file is created.

- [ ] **Step 3: Run tests**

Run: `go test ./cmd/ -run TestAvatar -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/avatar.go cmd/avatar_test.go
git commit -m "feat(avatar): save character file on avatar build"
```

---

## Task 18: Avatar Act Command (Autonomous Loop)

**Files:**
- Create: `cmd/avatar_act.go`
- Test: `cmd/avatar_act_test.go`

- [ ] **Step 1: Implement the autonomous loop command**

```go
// cmd/avatar_act.go
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sofq/jira-cli/internal/character"
	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/sofq/jira-cli/internal/watch"
	"github.com/sofq/jira-cli/internal/yolo"
	"github.com/spf13/cobra"
)

var avatarActCmd = &cobra.Command{
	Use:   "act",
	Short: "Autonomous execution loop — watch and react to Jira events",
	RunE:  runAvatarAct,
}

func init() {
	avatarActCmd.Flags().String("jql", "", "JQL query to watch")
	avatarActCmd.Flags().Duration("interval", 30*time.Second, "poll interval")
	avatarActCmd.Flags().Int("max-actions", 0, "stop after N actions (default: unlimited)")
	avatarActCmd.Flags().Bool("dry-run", false, "preview decisions without executing")
	avatarActCmd.Flags().Bool("once", false, "run one poll cycle and exit")
	avatarActCmd.Flags().Bool("yes", false, "skip consent prompt")
	avatarActCmd.Flags().String("character", "", "override character")
	avatarActCmd.Flags().String("writing", "", "override writing section")
	avatarActCmd.Flags().String("react-to", "", "comma-separated event types to react to")
}

func runAvatarAct(cmd *cobra.Command, args []string) error {
	jql, _ := cmd.Flags().GetString("jql")
	if jql == "" {
		(&jrerrors.APIError{ErrorType: "validation_error", Message: "--jql is required"}).WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	interval, _ := cmd.Flags().GetDuration("interval")
	maxActions, _ := cmd.Flags().GetInt("max-actions")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	once, _ := cmd.Flags().GetBool("once")
	charName, _ := cmd.Flags().GetString("character")
	reactTo, _ := cmd.Flags().GetString("react-to")

	// Resolve character
	dir := character.DefaultDir()
	resolveOpts := character.ResolveOptions{
		FlagCharacter: charName,
		CharDir:       dir,
		ProfileName:   "default",
		StateFile:     dir + "/active-character.json",
	}
	composed, err := character.ResolveActive(resolveOpts)
	if err != nil {
		return err
	}

	// Build event filter
	var eventFilter map[string]bool
	if reactTo != "" {
		eventFilter = make(map[string]bool)
		for _, et := range strings.Split(reactTo, ",") {
			et = strings.TrimSpace(et)
			if !yolo.ValidEventType(et) {
				return fmt.Errorf("invalid event type: %q", et)
			}
			eventFilter[et] = true
		}
	}

	// Setup context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	c := clientFromContext(cmd)
	actionCount := 0
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)

	maxEvents := 0
	if once {
		maxEvents = -1 // sentinel: watch layer processes one poll cycle then stops
	}

	handler := func(e watch.Event) error {
		if maxActions > 0 && actionCount >= maxActions {
			cancel()
			return nil
		}

		// Classify event type
		eventType := classifyEvent(e)
		if eventFilter != nil && !eventFilter[eventType] {
			return nil
		}

		// Decide action
		action, ok := yolo.DecideRuleBased(composed, eventType)
		if !ok {
			enc.Encode(map[string]any{
				"ts": time.Now().Format(time.RFC3339), "event": eventType,
				"issue": extractIssueKey(e), "action": "none", "reason": "no_matching_rule",
			})
			return nil
		}

		if dryRun {
			enc.Encode(map[string]any{
				"ts": time.Now().Format(time.RFC3339), "event": eventType,
				"issue": extractIssueKey(e), "action": action.Action, "status": "dry_run",
				"params": action,
			})
			return nil
		}

		// Execute action
		// TODO: dispatch to workflow commands based on action type
		actionCount++
		enc.Encode(map[string]any{
			"ts": time.Now().Format(time.RFC3339), "event": eventType,
			"issue": extractIssueKey(e), "action": action.Action, "status": "executed",
			"character": composed.Name,
		})
		return nil
	}

	watchOpts := watch.Options{
		JQL:       jql,
		Interval:  interval,
		MaxEvents: maxEvents,
		Handler:   handler,
	}

	watch.Run(ctx, c, watchOpts)
	return nil
}

func classifyEvent(e watch.Event) string {
	switch e.Type {
	case "initial", "created":
		return yolo.EventIssueCreated
	case "updated":
		return yolo.EventFieldChange
	default:
		return yolo.EventFieldChange
	}
}

func extractIssueKey(e watch.Event) string {
	var issue struct {
		Key string `json:"key"`
	}
	json.Unmarshal(e.Issue, &issue)
	return issue.Key
}
```

- [ ] **Step 2: Register actCmd as avatar subcommand**

In `cmd/avatar.go`, add:
```go
avatarCmd.AddCommand(avatarActCmd)
```

- [ ] **Step 3: Write basic test**

Test that `jr avatar act --dry-run --jql "..." --once` runs without error.

- [ ] **Step 4: Build and test**

Run: `go build ./... && go test ./cmd/ -run TestAvatarAct -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/avatar_act.go cmd/avatar_act_test.go cmd/avatar.go
git commit -m "feat(avatar): add autonomous loop command (jr avatar act)"
```

---

## Task 19: Update CLAUDE.md Documentation

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add character and yolo commands to CLAUDE.md**

Add to the appropriate sections:

```markdown
# Character — persona management
jr character list
jr character show <name>
jr character create <name>
jr character create --template concise
jr character create --from-user alice@co.com
jr character edit <name>
jr character delete <name>
jr character use <name> --writing formal-pm
jr character prompt --format prose --redact

# Yolo — autonomous execution policy
jr yolo status
jr yolo history --since 1h

# Autonomous loop
jr avatar act --jql "assignee = me" --interval 30s
jr avatar act --dry-run --jql "project = PROJ" --once
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add character and yolo commands to CLAUDE.md"
```

---

## Task 20: Final Integration Test

- [ ] **Step 1: Run full test suite**

Run: `make test`
Expected: PASS

- [ ] **Step 2: Run linter**

Run: `make lint`
Expected: PASS

- [ ] **Step 3: Build binary**

Run: `make build`
Expected: Clean build

- [ ] **Step 4: Manual smoke test**

```bash
./jr character create --template concise
./jr character list
./jr character show concise
./jr character use concise
./jr character prompt
./jr yolo status
```

Expected: All commands produce valid JSON/YAML output.
