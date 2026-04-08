package orchestration

import (
	"fmt"
	"os"
	"path/filepath"

	"maestro/log"
	"maestro/pkg/accounts"
)

// SessionState represents the persisted session state for auto-resume.
type SessionState struct {
	RepoPath    string            `json:"repo_path"`
	StartedAt   string            `json:"started_at"`
	StashRef    string            `json:"stash_ref,omitempty"`
	StartTag    string            `json:"start_tag,omitempty"`
	Instances   []SessionInstance `json:"instances"`
	LastSavedAt string            `json:"last_saved_at"`
}

// SessionInstance is a lightweight record of an instance for resume.
type SessionInstance struct {
	Title   string            `json:"title"`
	Account string            `json:"account"`
	Branch  string            `json:"branch"`
	Env     map[string]string `json:"env,omitempty"`
}

// SaveSession writes session state to ~/.maestro/session.json.
// Creates the maestro directory if it does not already exist (first-run safety).
func SaveSession(state *SessionState) error {
	state.LastSavedAt = NowISO()
	base, err := accounts.ConductorDir()
	if err != nil {
		return err
	}
	if mkErr := os.MkdirAll(base, 0700); mkErr != nil {
		return fmt.Errorf("failed to create maestro dir: %w", mkErr)
	}
	path := filepath.Join(base, "session.json")
	return AtomicWriteJSON(path, state)
}

// LoadSession reads session state. Returns nil, nil if no session file exists or is empty.
// Returns nil, nil if the file is corrupt (logs a warning).
func LoadSession() (*SessionState, error) {
	base, err := accounts.ConductorDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(base, "session.json")
	var state SessionState
	if err := ReadJSONFile(path, &state); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		log.WarningLog.Printf("session: %s is corrupt, ignoring: %v", path, err)
		return nil, nil
	}
	return &state, nil
}

// DeleteSession removes the session file.
func DeleteSession() error {
	base, err := accounts.ConductorDir()
	if err != nil {
		return err
	}
	path := filepath.Join(base, "session.json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
