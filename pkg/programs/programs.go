package programs

import (
	"path/filepath"
	"sort"
	"strings"
)

// ProgramSpec defines the static configuration for a supported AI coding program.
type ProgramSpec struct {
	Name               string
	Binary             string
	AuthCommand        string   // e.g., "auth login"
	AuthStatusCommand  string   // e.g., "auth status --json"
	ConfigDirEnvVar    string   // e.g., "CLAUDE_CONFIG_DIR" or "CODEX_HOME"
	InstructionsFile   string   // e.g., "CLAUDE.md" or "AGENTS.md"
	PromptStrings      []string // strings that indicate program is waiting for input
	TrustPromptStrings []string // strings for trust/permissions prompt
	DefaultModel       string   // e.g., "sonnet-4.6" or "gpt-5.3-codex"
	AvailableModels    []string // all models this program supports
	HasStatusLine      bool     // whether Claude-style statusLine is supported
	HasRateLimitRPC    bool     // reserved: true for Codex accounts; CLI rate-limit polling not yet available
	AuthProvider       string   // "anthropic" or "chatgpt"
}

// Specs is the static registry of all supported program specs.
var Specs = map[string]ProgramSpec{
	"claude": {
		Name:               "claude",
		Binary:             "claude",
		AuthCommand:        "auth login",
		AuthStatusCommand:  "auth status --json",
		ConfigDirEnvVar:    "CLAUDE_CONFIG_DIR",
		InstructionsFile:   "CLAUDE.md",
		PromptStrings:      []string{"No, and tell Claude what to do differently"},
		TrustPromptStrings: []string{"Do you trust the files in this folder?", "new MCP server"},
		DefaultModel:       "sonnet-4.6",
		AvailableModels:    []string{"opus-4.6", "sonnet-4.6", "haiku-4.5"},
		HasStatusLine:      true,
		HasRateLimitRPC:    false,
		AuthProvider:       "anthropic",
	},
	"codex": {
		Name:               "codex",
		Binary:             "codex",
		AuthCommand:        "login",
		AuthStatusCommand:  "login status",
		ConfigDirEnvVar:    "CODEX_HOME",
		InstructionsFile:   "AGENTS.md",
		PromptStrings:      []string{}, // TBD -- needs runtime discovery
		TrustPromptStrings: []string{}, // TBD
		DefaultModel:       "gpt-5.3-codex",
		AvailableModels:    []string{"gpt-5.4-thinking", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.3-codex", "gpt-5.1-codex-max", "gpt-5-codex-mini"},
		HasStatusLine:      false,
		HasRateLimitRPC:    true,
		AuthProvider:       "chatgpt",
	},
}

// Get returns the ProgramSpec for the given program name.
// If name is empty, returns the default (claude).
func Get(name string) (ProgramSpec, bool) {
	if name == "" {
		name = "claude"
	}
	spec, ok := Specs[name]
	return spec, ok
}

// GetByBinary returns the ProgramSpec whose Binary field matches the base
// name of the given binary path, or appears as a path-separated suffix.
// A simple HasSuffix check is insufficient because it would match
// "/usr/bin/notclaude" against "claude".
func GetByBinary(binaryPath string) (ProgramSpec, bool) {
	base := filepath.Base(binaryPath)
	for _, spec := range Specs {
		// Exact base name match (most common case).
		if base == spec.Binary {
			return spec, true
		}
		// Also accept if the binary name appears after a path separator
		// (e.g., "/opt/tools/claude" matches "claude").
		if strings.HasSuffix(binaryPath, string(filepath.Separator)+spec.Binary) {
			return spec, true
		}
	}
	return ProgramSpec{}, false
}

// All returns all registered program specs.
func All() []ProgramSpec {
	result := make([]ProgramSpec, 0, len(Specs))
	for _, spec := range Specs {
		result = append(result, spec)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// ValidProgram returns true if the given name is a registered program.
func ValidProgram(name string) bool {
	_, ok := Specs[name]
	return ok
}

// Names returns sorted list of all program names.
func Names() []string {
	names := make([]string, 0, len(Specs))
	for name := range Specs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DefaultSpec returns the default program spec (claude).
func DefaultSpec() ProgramSpec {
	spec, ok := Specs["claude"]
	if !ok {
		panic("programs: default spec \"claude\" is missing from registry")
	}
	return spec
}
