package orchestration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"claude-conductor/pkg/accounts"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// RunOutput captures the last N lines from a worker's tmux pane and prints them.
// Default lines is 50, max is 500.
func RunOutput(instanceName string, lines int) error {
	if lines <= 0 {
		lines = 50
	}
	if lines > 500 {
		lines = 500
	}

	entry, err := resolveAndValidateInstance(instanceName)
	if err != nil {
		return err
	}

	content, err := tmuxCapturePaneLines(entry.TmuxSession, lines)
	if err != nil {
		return fmt.Errorf("failed to capture pane output: %w", err)
	}

	cleaned := stripANSI(content)
	fmt.Print(cleaned)
	return nil
}

// RunRecall captures the full scrollback history from a worker's tmux pane,
// saves it to a timestamped file, and prints the path and stats.
func RunRecall(instanceName string) error {
	entry, err := resolveAndValidateInstance(instanceName)
	if err != nil {
		return err
	}

	content, err := tmuxCaptureFullHistory(entry.TmuxSession)
	if err != nil {
		return fmt.Errorf("failed to capture full history: %w", err)
	}

	cleaned := stripANSI(content)

	// Save to captures directory
	base, err := accounts.ConductorDir()
	if err != nil {
		return fmt.Errorf("failed to resolve conductor dir: %w", err)
	}
	capturesDir := filepath.Join(base, "captures")
	if err := os.MkdirAll(capturesDir, 0700); err != nil {
		return fmt.Errorf("failed to create captures dir: %w", err)
	}

	filename := fmt.Sprintf("%s-%d.txt", instanceName, time.Now().Unix())
	capturePath := filepath.Join(capturesDir, filename)
	if err := os.WriteFile(capturePath, []byte(cleaned), 0600); err != nil {
		return fmt.Errorf("failed to write capture file: %w", err)
	}

	lineCount := strings.Count(cleaned, "\n")
	byteCount := len(cleaned)
	fmt.Printf("Saved to: %s\n", capturePath)
	fmt.Printf("Lines: %d, Size: %d bytes\n", lineCount, byteCount)
	return nil
}

// resolveAndValidateInstance loads the registry, checks the instance exists,
// and validates the conductor session prefix.
func resolveAndValidateInstance(instanceName string) (*RegistryEntry, error) {
	reg, err := LoadRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to load registry: %w", err)
	}

	entry, ok := reg.GetInstance(instanceName)
	if !ok {
		return nil, fmt.Errorf("instance %q not found in registry", instanceName)
	}

	if !strings.HasPrefix(entry.TmuxSession, conductorSessionPrefix) {
		return nil, fmt.Errorf("tmux session %q does not have required prefix %q", entry.TmuxSession, conductorSessionPrefix)
	}

	return entry, nil
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}
