package avatar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// llmTimeout is the maximum duration for an LLM command. Tests can override
// this to verify the timeout branch without waiting 5 minutes.
var llmTimeout = 5 * time.Minute

// BuildLLM generates a Profile by calling an external LLM command.
//
// The extraction is marshalled to JSON and written to the command's stdin.
// The command is expected to write a valid YAML Profile to stdout.
// overrides is a list of "key=value" strings applied to profile.Overrides.
func BuildLLM(extraction *Extraction, llmCmd string, overrides []string) (*Profile, error) {
	// json.Marshal cannot fail for *Extraction (all fields are primitive
	// types, strings, slices, and maps — no channels or funcs).
	extractionJSON, _ := json.Marshal(extraction)

	// Split llmCmd into command and arguments.
	fields := strings.Fields(llmCmd)
	if len(fields) == 0 {
		return nil, fmt.Errorf("llmCmd is empty")
	}
	cmd, args := fields[0], fields[1:]

	// Execute the command with a timeout and extraction JSON on stdin.
	ctx, cancel := context.WithTimeout(context.Background(), llmTimeout)
	defer cancel()

	c := exec.CommandContext(ctx, cmd, args...) // #nosec G204 -- cmd is user-configured LLM command
	c.Stdin = bytes.NewReader(extractionJSON)
	var stdout bytes.Buffer
	c.Stdout = &stdout

	if err := c.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("llm command timed out after 5m")
		}
		return nil, fmt.Errorf("llm command failed: %w", err)
	}

	// Unmarshal stdout as YAML into Profile.
	var profile Profile
	if err := yaml.Unmarshal(stdout.Bytes(), &profile); err != nil {
		return nil, fmt.Errorf("unmarshal profile YAML: %w", err)
	}

	// Validate: Version must be >= 1.
	v, err := strconv.Atoi(strings.TrimSpace(profile.Version))
	if err != nil || v < 1 {
		return nil, fmt.Errorf("profile validation failed: version %q must be >= 1", profile.Version)
	}

	// Validate: StyleGuide sections must be non-empty.
	if strings.TrimSpace(profile.StyleGuide.Writing) == "" {
		return nil, fmt.Errorf("profile validation failed: style_guide.writing is empty")
	}
	if strings.TrimSpace(profile.StyleGuide.Workflow) == "" {
		return nil, fmt.Errorf("profile validation failed: style_guide.workflow is empty")
	}
	if strings.TrimSpace(profile.StyleGuide.Interaction) == "" {
		return nil, fmt.Errorf("profile validation failed: style_guide.interaction is empty")
	}

	// Apply overrides.
	if len(overrides) > 0 {
		if profile.Overrides == nil {
			profile.Overrides = make(map[string]string)
		}
		for _, o := range overrides {
			idx := strings.IndexByte(o, '=')
			if idx < 0 {
				return nil, fmt.Errorf("invalid override %q: must be key=value", o)
			}
			profile.Overrides[o[:idx]] = o[idx+1:]
		}
	}

	// Set Engine to "llm".
	profile.Engine = "llm"

	return &profile, nil
}
