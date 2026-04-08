package orchestration

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// AtomicWriteJSON marshals data as indented JSON and writes it atomically to path
// via a temporary file and rename. This prevents readers from seeing partial writes.
func AtomicWriteJSON(path string, data interface{}) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("failed to generate temp file suffix: %w", err)
	}
	tmp := fmt.Sprintf("%s.%d-%x.tmp", path, os.Getpid(), b)
	if err := os.WriteFile(tmp, bytes, 0600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // best-effort cleanup to avoid temp file leak
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}

// AtomicWritePrompt writes raw bytes to path atomically (write-to-temp, rename).
// Used for prompt files, which are plain text rather than JSON.
func AtomicWritePrompt(path string, data []byte) error {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("failed to generate temp file suffix: %w", err)
	}
	tmp := fmt.Sprintf("%s.%d-%x.tmp", path, os.Getpid(), b)
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp prompt file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // best-effort cleanup to avoid temp file leak
		return fmt.Errorf("failed to rename temp prompt file: %w", err)
	}
	return nil
}

// GenerateTaskID returns a unique task ID of the form "task-<unix_timestamp>-<hex>".
func GenerateTaskID() (string, error) {
	ts := time.Now().Unix()
	b := make([]byte, 4) // 4 bytes = 8 hex chars for better uniqueness
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return fmt.Sprintf("task-%d-%s", ts, hex.EncodeToString(b)), nil
}

// ReadJSONFile reads path and unmarshals its contents into target. Returns the
// underlying os error (e.g., os.ErrNotExist) when the file is missing or empty.
func ReadJSONFile(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		// Treat an empty file the same as a missing file so callers that check
		// os.IsNotExist get consistent behavior on first-run / truncated writes.
		return os.ErrNotExist
	}
	return json.Unmarshal(data, target)
}
