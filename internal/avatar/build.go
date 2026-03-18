package avatar

import (
	"fmt"
	"io"

	"github.com/sofq/jira-cli/internal/config"
)

// BuildOptions controls how Build selects and drives the profile-generation engine.
type BuildOptions struct {
	// Engine overrides avatarCfg.Engine when non-empty.
	Engine string
	// LLMCmd is the command used when Engine is "llm".
	LLMCmd string
	// Yes suppresses interactive confirmation prompts.
	Yes bool
	// Stderr is where progress/diagnostic messages are written.
	Stderr io.Writer
}

// Build generates a Profile from an Extraction using the configured engine.
//
// Engine selection priority (first non-empty wins):
//  1. opts.Engine
//  2. avatarCfg.Engine
//  3. "local" (default)
//
// Overrides are collected from avatarCfg.Overrides (key=value pairs passed to BuildLocal).
func Build(extraction *Extraction, avatarCfg *config.AvatarConfig, opts BuildOptions) (*Profile, error) {
	engine := opts.Engine
	if engine == "" && avatarCfg != nil {
		engine = avatarCfg.Engine
	}
	if engine == "" {
		engine = "local"
	}

	// Collect overrides from avatarCfg as "key=value" strings.
	var overrides []string
	if avatarCfg != nil {
		for k, v := range avatarCfg.Overrides {
			overrides = append(overrides, k+"="+v)
		}
	}

	switch engine {
	case "local":
		return BuildLocal(extraction, overrides)
	case "llm":
		return nil, fmt.Errorf("llm engine not yet implemented")
	default:
		return nil, fmt.Errorf("unknown engine %q: valid engines are \"local\" and \"llm\"", engine)
	}
}
