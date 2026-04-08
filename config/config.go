package config

import (
	"encoding/json"
	"fmt"
	"maestro/log"
	"maestro/pkg/programs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	ConfigFileName = "config.json"
	defaultProgram = "claude"
)

// aliasRegex matches shell alias output like "claude: aliased to /path" or
// "claude -> /path" or "claude=/path" and captures the target path.
var aliasRegex = regexp.MustCompile(`(?:aliased to|->|=)\s*([^\s]+)`)

// GetConfigDir returns the path to the application's configuration directory
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config home directory: %w", err)
	}
	return filepath.Join(homeDir, ".maestro"), nil
}

// getLegacyConfigDir returns the path to the legacy configuration directory
// used before the rename from claude-conductor to maestro.
func getLegacyConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".claude-conductor"), nil
}

// Profile represents a named program configuration
type Profile struct {
	Name    string `json:"name"`
	Program string `json:"program"`
}

// Config represents the application configuration
type Config struct {
	// DefaultProgram is the default program to run in new instances
	DefaultProgram string `json:"default_program"`
	// AutoYes is a flag to automatically accept all prompts.
	AutoYes bool `json:"auto_yes"`
	// DaemonPollInterval is the interval (ms) at which the daemon polls sessions for autoyes mode.
	DaemonPollInterval int `json:"daemon_poll_interval"`
	// BranchPrefix is the prefix used for git branches created by the application.
	BranchPrefix string `json:"branch_prefix"`
	// Profiles is a list of named program profiles.
	Profiles []Profile `json:"profiles,omitempty"`
}

// GetProgram returns the program to run. If Profiles is non-empty and
// DefaultProgram matches a profile name, that profile's Program is returned.
// Otherwise DefaultProgram is returned as-is. Falls back to "claude" if the
// resolved program name is empty.
func (c *Config) GetProgram() string {
	for _, p := range c.Profiles {
		if p.Name == c.DefaultProgram {
			if p.Program != "" {
				return p.Program
			}
		}
	}
	if c.DefaultProgram == "" {
		return defaultProgram
	}
	return c.DefaultProgram
}

// GetProfiles returns a unified list of profiles. If Profiles is defined,
// those are returned with the default profile first. Otherwise, a single
// profile is synthesized from DefaultProgram.
func (c *Config) GetProfiles() []Profile {
	if len(c.Profiles) == 0 {
		return []Profile{{Name: c.DefaultProgram, Program: c.DefaultProgram}}
	}
	// Reorder so the default profile comes first.
	profiles := make([]Profile, 0, len(c.Profiles))
	for _, p := range c.Profiles {
		if p.Name == c.DefaultProgram {
			profiles = append(profiles, p)
			break
		}
	}
	for _, p := range c.Profiles {
		if p.Name != c.DefaultProgram {
			profiles = append(profiles, p)
		}
	}
	return profiles
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	program, err := GetClaudeCommand()
	if err != nil {
		log.ErrorLog.Printf("failed to get claude command: %v", err)
		program = defaultProgram
	}

	return &Config{
		DefaultProgram:     program,
		AutoYes:            false,
		DaemonPollInterval: 1000,
		BranchPrefix: func() string {
			user, err := user.Current()
			if err != nil || user == nil || user.Username == "" {
				log.ErrorLog.Printf("failed to get current user: %v", err)
				return "session/"
			}
			return fmt.Sprintf("%s/", strings.ToLower(user.Username))
		}(),
	}
}

// GetProgramCommand attempts to find the given program's binary in the user's
// shell. It checks shell alias resolution (via "which") first, then falls back
// to a direct PATH lookup. If both fail, it returns an error.
func GetProgramCommand(programName string) (string, error) {
	spec, ok := programs.Get(programName)
	if !ok {
		return "", fmt.Errorf("unknown program %q", programName)
	}
	binaryName := spec.Binary

	shell := os.Getenv("SHELL")
	if shell == "" || !filepath.IsAbs(shell) {
		shell = "/bin/sh" // Default to /bin/sh if SHELL is not set or not an absolute path
	}

	// Force the shell to load the user's profile and then run the command
	// For zsh, source .zshrc; for bash, source .bashrc
	var shellCmd string
	if strings.Contains(shell, "zsh") {
		shellCmd = fmt.Sprintf("source ~/.zshrc &>/dev/null || true; which %s", binaryName)
	} else if strings.Contains(shell, "bash") {
		shellCmd = fmt.Sprintf("source ~/.bashrc &>/dev/null || true; which %s", binaryName)
	} else {
		shellCmd = fmt.Sprintf("which %s", binaryName)
	}

	cmd := exec.Command(shell, "-c", shellCmd)
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		path := strings.TrimSpace(string(output))
		if path != "" {
			// Check if the output is an alias definition and extract the actual path
			// Handle formats like "claude: aliased to /path/to/claude" or other shell-specific formats
			matches := aliasRegex.FindStringSubmatch(path)
			if len(matches) > 1 {
				path = matches[1]
			}
			return path, nil
		}
	}

	// Otherwise, try to find in PATH directly
	binPath, err := exec.LookPath(binaryName)
	if err == nil {
		return binPath, nil
	}

	return "", fmt.Errorf("%s command not found in aliases or PATH", binaryName)
}

// GetClaudeCommand is deprecated -- use GetProgramCommand("claude").
func GetClaudeCommand() (string, error) {
	return GetProgramCommand("claude")
}

func LoadConfig() *Config {
	configDir, err := GetConfigDir()
	if err != nil {
		log.ErrorLog.Printf("failed to get config directory: %v", err)
		return DefaultConfig()
	}

	configPath := filepath.Join(configDir, ConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Check for legacy config at ~/.claude-conductor/
			if legacyDir, legacyErr := getLegacyConfigDir(); legacyErr == nil {
				legacyPath := filepath.Join(legacyDir, ConfigFileName)
				if _, statErr := os.Stat(legacyPath); statErr == nil {
					log.WarningLog.Printf("Found legacy config at %s — please migrate to %s", legacyDir, configDir)
				}
			}
			// Create and save default config if file doesn't exist
			defaultCfg := DefaultConfig()
			if saveErr := saveConfig(defaultCfg); saveErr != nil {
				log.WarningLog.Printf("failed to save default config: %v", saveErr)
			}
			return defaultCfg
		}

		log.WarningLog.Printf("failed to get config file: %v", err)
		return DefaultConfig()
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		log.ErrorLog.Printf("failed to parse config file: %v", err)
		return DefaultConfig()
	}

	return &config
}

// saveConfig saves the configuration to disk
func saveConfig(config *Config) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, ConfigFileName)
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	tmp := configPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp config file: %w", err)
	}
	return os.Rename(tmp, configPath)
}

// SaveConfig exports the saveConfig function for use by other packages
func SaveConfig(config *Config) error {
	return saveConfig(config)
}
