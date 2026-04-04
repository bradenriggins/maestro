package orchestration

import (
	"fmt"
	"path/filepath"

	"claude-conductor/pkg/accounts"
)

// LoadRegistry reads the registry from the default conductor directory.
func LoadRegistry() (*Registry, error) {
	dir, err := accounts.ConductorDir()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve conductor dir: %w", err)
	}
	return LoadRegistryFromPath(filepath.Join(dir, "registry.json"))
}

// LoadRegistryFromPath reads the registry from a specific path.
func LoadRegistryFromPath(path string) (*Registry, error) {
	var reg Registry
	if err := ReadJSONFile(path, &reg); err != nil {
		return nil, fmt.Errorf("failed to read registry: %w", err)
	}
	if reg.Instances == nil {
		reg.Instances = make(map[string]RegistryEntry)
	}
	return &reg, nil
}

// GetInstance returns the registry entry for the given instance name.
func (r *Registry) GetInstance(name string) (*RegistryEntry, bool) {
	entry, ok := r.Instances[name]
	if !ok {
		return nil, false
	}
	return &entry, true
}

// ListWorkers returns all entries whose Role is "worker".
func (r *Registry) ListWorkers() map[string]RegistryEntry {
	workers := make(map[string]RegistryEntry)
	for name, entry := range r.Instances {
		if entry.Role == "worker" {
			workers[name] = entry
		}
	}
	return workers
}

// ListAll returns all entries in the registry.
func (r *Registry) ListAll() map[string]RegistryEntry {
	all := make(map[string]RegistryEntry, len(r.Instances))
	for k, v := range r.Instances {
		all[k] = v
	}
	return all
}
