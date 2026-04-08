package session

import (
	"encoding/json"
	"fmt"
	"maestro/config"
	"maestro/log"
	"time"
)

// titleOnly is a minimal struct used to extract the title from a raw JSON entry
// without fully deserializing the InstanceData (avoids failures from broken entries).
type titleOnly struct {
	Title string `json:"title"`
}

// InstanceData represents the serializable data of an Instance
type InstanceData struct {
	Title     string    `json:"title"`
	Path      string    `json:"path"`
	Branch    string    `json:"branch"`
	Status    Status    `json:"status"`
	Height    int       `json:"height"`
	Width     int       `json:"width"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	AutoYes   bool      `json:"auto_yes"`

	Program   string            `json:"program"`
	Model     string            `json:"model,omitempty"`
	Worktree  GitWorktreeData   `json:"worktree"`
	DiffStats DiffStatsData     `json:"diff_stats"`
	Account   string            `json:"account"`
	Role      string            `json:"role"`
	Env       map[string]string `json:"env,omitempty"`
}

// GitWorktreeData represents the serializable data of a GitWorktree
type GitWorktreeData struct {
	RepoPath         string `json:"repo_path"`
	WorktreePath     string `json:"worktree_path"`
	SessionName      string `json:"session_name"`
	BranchName       string `json:"branch_name"`
	BaseCommitSHA    string `json:"base_commit_sha"`
	IsExistingBranch bool   `json:"is_existing_branch"`
}

// DiffStatsData represents the serializable data of a DiffStats
type DiffStatsData struct {
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Content string `json:"content"`
}

// Storage handles saving and loading instances using the state interface
type Storage struct {
	state config.InstanceStorage
}

// NewStorage creates a new storage instance
func NewStorage(state config.InstanceStorage) (*Storage, error) {
	return &Storage{
		state: state,
	}, nil
}

// SaveInstances saves the list of instances to disk
func (s *Storage) SaveInstances(instances []*Instance) error {
	// Convert instances to InstanceData
	data := make([]InstanceData, 0)
	for _, instance := range instances {
		if instance.Started() {
			data = append(data, instance.ToInstanceData())
		}
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	return s.state.SaveInstances(jsonData)
}

// LoadInstances loads the list of instances from disk
func (s *Storage) LoadInstances() ([]*Instance, error) {
	jsonData := s.state.GetInstances()

	if len(jsonData) == 0 {
		return []*Instance{}, nil
	}

	var instancesData []InstanceData
	if err := json.Unmarshal(jsonData, &instancesData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instances: %w", err)
	}

	instances := make([]*Instance, 0, len(instancesData))
	for _, data := range instancesData {
		instance, err := FromInstanceData(data)
		if err != nil {
			// Log and skip the failed instance rather than aborting the entire load.
			// A single corrupted or orphaned instance (e.g., whose git repo no longer
			// exists) must not prevent the other instances from being recovered.
			log.ErrorLog.Printf("skipping instance %q: failed to restore: %v", data.Title, err)
			continue
		}
		instances = append(instances, instance)
	}

	return instances, nil
}

// DeleteInstance removes an instance from storage.
// It operates directly on the raw JSON array so that a single corrupted entry
// does not prevent deletion of other (healthy) instances.
func (s *Storage) DeleteInstance(title string) error {
	raw := s.state.GetInstances()
	if len(raw) == 0 {
		return fmt.Errorf("instance not found: %s", title)
	}

	// Unmarshal into a slice of raw messages to avoid full deserialization.
	var rawEntries []json.RawMessage
	if err := json.Unmarshal(raw, &rawEntries); err != nil {
		return fmt.Errorf("failed to parse instances: %w", err)
	}

	found := false
	kept := rawEntries[:0]
	for _, entry := range rawEntries {
		var t titleOnly
		if err := json.Unmarshal(entry, &t); err != nil || t.Title == title {
			if err == nil {
				// Matched by title — drop it.
				found = true
				continue
			}
			// Could not parse — keep the entry to avoid accidental data loss.
			kept = append(kept, entry)
			continue
		}
		kept = append(kept, entry)
	}

	if !found {
		return fmt.Errorf("instance not found: %s", title)
	}

	newRaw, err := json.Marshal(kept)
	if err != nil {
		return fmt.Errorf("failed to marshal updated instances: %w", err)
	}

	return s.state.SaveInstances(newRaw)
}

// UpdateInstance updates an existing instance in storage.
// It operates directly on the raw JSON array so that other instances are never
// re-hydrated (and therefore never re-started) as a side-effect of the update.
func (s *Storage) UpdateInstance(instance *Instance) error {
	raw := s.state.GetInstances()
	if len(raw) == 0 {
		return fmt.Errorf("instance not found: %s", instance.Title)
	}

	// Unmarshal into a slice of raw messages to avoid full deserialization of
	// other entries (which would start their tmux sessions / git worktrees).
	var rawEntries []json.RawMessage
	if err := json.Unmarshal(raw, &rawEntries); err != nil {
		return fmt.Errorf("failed to parse instances: %w", err)
	}

	// Serialize the updated instance once.
	updatedData := instance.ToInstanceData()
	updatedJSON, err := json.Marshal(updatedData)
	if err != nil {
		return fmt.Errorf("failed to marshal updated instance: %w", err)
	}

	found := false
	for i, entry := range rawEntries {
		var t titleOnly
		if err := json.Unmarshal(entry, &t); err != nil {
			// Could not parse entry — leave it untouched to avoid data loss.
			continue
		}
		if t.Title == instance.Title {
			rawEntries[i] = json.RawMessage(updatedJSON)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", instance.Title)
	}

	newRaw, err := json.Marshal(rawEntries)
	if err != nil {
		return fmt.Errorf("failed to marshal updated instances: %w", err)
	}

	return s.state.SaveInstances(newRaw)
}

// DeleteAllInstances removes all stored instances
func (s *Storage) DeleteAllInstances() error {
	return s.state.DeleteAllInstances()
}

// repoPathOnly is a minimal struct used to extract the repo path from a raw instance entry.
type repoPathOnly struct {
	Worktree struct {
		RepoPath string `json:"repo_path"`
	} `json:"worktree"`
}

// GetRepoPaths returns the unique set of repository paths recorded across all
// stored instances.  It reads raw JSON so that corrupted entries are skipped
// gracefully rather than causing the whole call to fail.
func (s *Storage) GetRepoPaths() []string {
	raw := s.state.GetInstances()
	if len(raw) == 0 {
		return nil
	}

	var rawEntries []json.RawMessage
	if err := json.Unmarshal(raw, &rawEntries); err != nil {
		return nil
	}

	seen := make(map[string]struct{})
	var paths []string
	for _, entry := range rawEntries {
		var r repoPathOnly
		if err := json.Unmarshal(entry, &r); err != nil {
			continue
		}
		if r.Worktree.RepoPath == "" {
			continue
		}
		if _, ok := seen[r.Worktree.RepoPath]; !ok {
			seen[r.Worktree.RepoPath] = struct{}{}
			paths = append(paths, r.Worktree.RepoPath)
		}
	}
	return paths
}
