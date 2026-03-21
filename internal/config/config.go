package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// goos is the operating system identifier used by DefaultPath.
// It defaults to runtime.GOOS and can be overridden in tests.
var goos = runtime.GOOS

// AuthConfig holds authentication credentials for a profile.
type AuthConfig struct {
	Type         string `json:"type"`
	Username     string `json:"username,omitempty"`
	Token        string `json:"token,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	TokenURL     string `json:"token_url,omitempty"`
	Scopes       string `json:"scopes,omitempty"`
}

// AvatarConfig holds configuration for the avatar (persona) feature.
type AvatarConfig struct {
	Enabled     bool              `json:"enabled,omitempty"`
	Engine      string            `json:"engine,omitempty"`
	LLMCmd      string            `json:"llm_cmd,omitempty"`
	MinComments int               `json:"min_comments,omitempty"`
	MinUpdates  int               `json:"min_updates,omitempty"`
	MaxWindow   string            `json:"max_window,omitempty"`
	Overrides   map[string]string `json:"overrides,omitempty"`
}

// Profile holds the configuration for a named Jira instance.
type Profile struct {
	BaseURL           string        `json:"base_url"`
	Auth              AuthConfig    `json:"auth"`
	AllowedOperations []string      `json:"allowed_operations,omitempty"`
	DeniedOperations  []string      `json:"denied_operations,omitempty"`
	AuditLog          bool          `json:"audit_log,omitempty"`
	Avatar            *AvatarConfig `json:"avatar,omitempty"`
}

// Config is the top-level configuration structure persisted to disk.
type Config struct {
	Profiles       map[string]Profile `json:"profiles"`
	DefaultProfile string             `json:"default_profile"`
}

// FlagOverrides carries values supplied via CLI flags. Empty string means
// "not set by flag".
type FlagOverrides struct {
	BaseURL  string
	AuthType string
	Username string
	Token    string
}

// ResolvedConfig is the final, merged configuration ready for use.
type ResolvedConfig struct {
	BaseURL           string
	Auth              AuthConfig
	ProfileName       string
	AllowedOperations []string
	DeniedOperations  []string
	AuditLog          bool
}

// DefaultPath returns the path to the configuration file. It checks the
// JR_CONFIG_PATH environment variable first; otherwise it falls back to an
// OS-specific default location.
func DefaultPath() string {
	if v := os.Getenv("JR_CONFIG_PATH"); v != "" {
		return v
	}
	switch goos {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "jr", "config.json")
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			home, _ := os.UserHomeDir()
			return filepath.Join(home, "AppData", "Roaming", "jr", "config.json")
		}
		return filepath.Join(appdata, "jr", "config.json")
	default: // linux and others
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "jr", "config.json")
	}
}

// LoadFrom reads and parses the config file at path. If the file does not
// exist, an empty (non-nil) Config is returned without error.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{Profiles: map[string]Profile{}}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	return &cfg, nil
}

// SaveTo serialises cfg as indented JSON and writes it to path with 0o600
// permissions, creating any missing parent directories.
// Config contains only strings and maps, so json.MarshalIndent cannot fail.
func SaveTo(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(path, data, 0o600)
}

// availableProfiles returns a comma-separated list of profile names.
func availableProfiles(cfg *Config) string {
	names := make([]string, 0, len(cfg.Profiles))
	for k := range cfg.Profiles {
		names = append(names, k)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

// validAuthTypes is the set of accepted authentication types.
var validAuthTypes = map[string]bool{"basic": true, "bearer": true, "oauth2": true}

// ValidAuthType reports whether s is a recognized authentication type (case-insensitive).
func ValidAuthType(s string) bool {
	return validAuthTypes[strings.ToLower(s)]
}

// Resolve builds a ResolvedConfig by merging sources in priority order:
// CLI flags > environment variables > config file profile.
//
// The profileName argument selects which profile to load from the config
// file; an empty string falls back to the DefaultProfile, then "default".
func Resolve(configPath, profileName string, flags *FlagOverrides) (*ResolvedConfig, error) {
	// 1. Load from config file (lowest priority).
	cfg, err := LoadFrom(configPath)
	if err != nil {
		return nil, err
	}

	name := profileName
	if name == "" {
		name = cfg.DefaultProfile
	}
	if name == "" {
		name = "default"
	}

	var fileBaseURL, fileAuthType, fileUsername, fileToken string
	var fileClientID, fileClientSecret, fileTokenURL, fileScopes string
	var fileAllowedOps, fileDeniedOps []string
	var fileAuditLog bool
	if p, ok := cfg.Profiles[name]; ok {
		fileBaseURL = p.BaseURL
		fileAuthType = p.Auth.Type
		fileUsername = p.Auth.Username
		fileToken = p.Auth.Token
		fileClientID = p.Auth.ClientID
		fileClientSecret = p.Auth.ClientSecret
		fileTokenURL = p.Auth.TokenURL
		fileScopes = p.Auth.Scopes
		fileAllowedOps = p.AllowedOperations
		fileDeniedOps = p.DeniedOperations
		fileAuditLog = p.AuditLog
	} else if profileName != "" {
		// Explicit --profile that doesn't exist should give a clear error.
		return nil, fmt.Errorf("profile %q not found; available profiles: %s", name, availableProfiles(cfg))
	}

	// 2. Environment variables (override config file).
	envBaseURL := os.Getenv("JR_BASE_URL")
	envAuthType := os.Getenv("JR_AUTH_TYPE")
	envUsername := os.Getenv("JR_AUTH_USER")
	envToken := os.Getenv("JR_AUTH_TOKEN")

	// 3. Merge: start with file values, then layer env vars.
	baseURL := fileBaseURL
	if envBaseURL != "" {
		baseURL = envBaseURL
	}

	authType := fileAuthType
	if envAuthType != "" {
		authType = envAuthType
	}

	username := fileUsername
	if envUsername != "" {
		username = envUsername
	}

	token := fileToken
	if envToken != "" {
		token = envToken
	}

	// 4. CLI flags (highest priority).
	if flags != nil {
		if flags.BaseURL != "" {
			baseURL = flags.BaseURL
		}
		if flags.AuthType != "" {
			authType = flags.AuthType
		}
		if flags.Username != "" {
			username = flags.Username
		}
		if flags.Token != "" {
			token = flags.Token
		}
	}

	// 5. Apply defaults.
	if authType == "" {
		authType = "basic"
	}

	// 6. Validate auth type.
	if !ValidAuthType(authType) {
		return nil, fmt.Errorf("invalid auth type %q; must be one of: basic, bearer, oauth2", authType)
	}
	authType = strings.ToLower(authType)

	// 7. Validate OAuth2 required fields when auth type is oauth2.
	if authType == "oauth2" {
		var missing []string
		if fileClientID == "" {
			missing = append(missing, "client_id")
		}
		if fileClientSecret == "" {
			missing = append(missing, "client_secret")
		}
		if fileTokenURL == "" {
			missing = append(missing, "token_url")
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf("auth type oauth2 requires %s to be set in the config file (%s)", strings.Join(missing, ", "), configPath)
		}
	}

	// 8. Trim trailing slash from BaseURL.
	baseURL = strings.TrimRight(baseURL, "/")

	return &ResolvedConfig{
		BaseURL: baseURL,
		Auth: AuthConfig{
			Type:         authType,
			Username:     username,
			Token:        token,
			ClientID:     fileClientID,
			ClientSecret: fileClientSecret,
			TokenURL:     fileTokenURL,
			Scopes:       fileScopes,
		},
		ProfileName:       name,
		AllowedOperations: fileAllowedOps,
		DeniedOperations:  fileDeniedOps,
		AuditLog:          fileAuditLog,
	}, nil
}
