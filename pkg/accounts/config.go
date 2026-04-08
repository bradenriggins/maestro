package accounts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const ConfigFileName = "config.json"

func DefaultConfig() *ConductorConfig {
	return &ConductorConfig{
		ProtocolVersion:   "1",
		Accounts:          nil,
		DefaultProgram:    "claude",
		BranchPrefix:      "maestro/",
		AutoYes:           false,
		PostWorktreeSetup: "",
		Notifications: NotificationConfig{
			Enabled:       true,
			TaskCompleted: true,
			TaskFailed:    true,
			WorkerStalled: true,
			AllDone:       true,
		},
		StallWarnSeconds:    30,
		StallTimeoutSeconds: 90,
		AutoCleanup:         true,
	}
}

func LoadConductorConfig() (*ConductorConfig, error) {
	base, err := ConductorDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(base, ConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Check for legacy config at ~/.claude-conductor/
			if legacyDir, legacyErr := legacyConductorDir(); legacyErr == nil {
				legacyPath := filepath.Join(legacyDir, ConfigFileName)
				if _, statErr := os.Stat(legacyPath); statErr == nil {
					return nil, fmt.Errorf(
						"no config found at %s, but legacy config exists at %s\n"+
							"  Migrate by running: mv %s %s",
						base, legacyDir, legacyDir, base)
				}
			}
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var cfg ConductorConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return &cfg, nil
}

func SaveConductorConfig(cfg *ConductorConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	base, err := ConductorDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return fmt.Errorf("failed to create conductor dir: %w", err)
	}
	path := filepath.Join(base, ConfigFileName)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	return os.Rename(tmp, path)
}
