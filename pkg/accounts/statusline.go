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
	if err := os.MkdirAll(usageDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create usage dir: %w", err)
	}

	outputPath := filepath.Join(usageDir, accountName+".json")

	// The script reads JSON from stdin, extracts rate_limits, and writes to file only if non-empty
	script := fmt.Sprintf(`#!/bin/bash
# Maestro statusLine receiver for account: %s
# Receives JSON on stdin from Claude Code, extracts rate_limits, writes to usage file
RESULT=$(jq -c '.rate_limits // empty' 2>/dev/null)
if [ -n "$RESULT" ]; then
    printf '%%s' "$RESULT" > "%s.tmp" && mv "%s.tmp" "%s"
fi
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
// Only Claude supports statusLine; for other programs this is a no-op.
func configureStatusLine(accountConfigDir, scriptPath, programName string) error {
	// Only Claude supports statusLine
	if programName != "" && programName != "claude" {
		return nil
	}
	settingsPath := filepath.Join(accountConfigDir, "settings.json")

	// Read existing settings or start fresh.
	// Surface unexpected read errors (e.g., permission denied) rather than
	// silently discarding them and overwriting the file.
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if jsonErr := json.Unmarshal(data, &settings); jsonErr != nil {
			// File exists but is not valid JSON — reset to empty rather than
			// propagating a parse error, matching prior behaviour intentionally.
			settings = make(map[string]interface{})
		}
	} else if os.IsNotExist(err) {
		settings = make(map[string]interface{})
	} else {
		return fmt.Errorf("failed to read existing settings.json: %w", err)
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
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // best-effort cleanup of the temp file
		return fmt.Errorf("failed to finalize %s: %w", path, err)
	}
	return nil
}
