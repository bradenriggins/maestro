package accounts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// generateStatusLineScript creates the shell script content that receives statusLine JSON
// and writes usage data to the conductor usage directory.
func generateStatusLineScript(accountName string) (string, error) {
	base, err := ConductorDir()
	if err != nil {
		return "", err
	}

	usageDir := filepath.Join(base, "usage")
	os.MkdirAll(usageDir, 0700)

	outputPath := filepath.Join(usageDir, accountName+".json")

	// The script reads JSON from stdin, extracts rate_limits, and writes to file
	script := fmt.Sprintf(`#!/bin/bash
# Claude Conductor statusLine receiver for account: %s
# Receives JSON on stdin from Claude Code, extracts rate_limits, writes to usage file
jq -c '.rate_limits // empty' | head -1 > "%s.tmp" 2>/dev/null && mv "%s.tmp" "%s" 2>/dev/null || true
`, accountName, outputPath, outputPath, outputPath)

	return script, nil
}

// generateStatusLineScriptFile writes the receiver script to disk and returns its path.
func generateStatusLineScriptFile(accountName string) (string, error) {
	base, err := ConductorDir()
	if err != nil {
		return "", err
	}

	binDir := filepath.Join(base, "bin")
	if err := os.MkdirAll(binDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create bin dir: %w", err)
	}
	scriptPath := filepath.Join(binDir, fmt.Sprintf("usage-receiver-%s.sh", accountName))

	script, err := generateStatusLineScript(accountName)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		return "", err
	}

	return scriptPath, nil
}

// configureStatusLine writes the statusLine setting into an account's Claude Code settings.
// It reads the existing settings.json (or creates one), adds/updates the statusLine field.
func configureStatusLine(accountConfigDir, scriptPath string) error {
	settingsPath := filepath.Join(accountConfigDir, "settings.json")

	// Read existing settings or start fresh
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if jsonErr := json.Unmarshal(data, &settings); jsonErr != nil {
			settings = make(map[string]interface{})
		}
	} else {
		settings = make(map[string]interface{})
	}

	// Set the statusLine configuration
	settings["statusLine"] = map[string]interface{}{
		"type":    "command",
		"command": scriptPath,
	}

	return atomicWriteJSON(settingsPath, settings)
}

// atomicWriteJSON writes data as JSON to a file atomically (write tmp, then rename).
func atomicWriteJSON(path string, data interface{}) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	return os.Rename(tmp, path)
}
