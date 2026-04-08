package accounts

// SetupConfig is the schema for the --config-file JSON that pre-seeds account
// names and roles, allowing non-interactive setup.
type SetupConfig struct {
	// ClaudeBin overrides the program binary path for all accounts in this config
	// file. Despite the JSON key name "claude_bin" it applies to any supported
	// program (claude, codex, …). The --program-bin flag on the CLI takes priority.
	ClaudeBin string        `json:"claude_bin,omitempty"`
	Accounts  []AccountSpec `json:"accounts"`
}

// AccountSpec describes a single account entry inside a SetupConfig file.
type AccountSpec struct {
	Name    string `json:"name"`
	Role    string `json:"role"`
	Program string `json:"program,omitempty"`
	Model   string `json:"model,omitempty"`
}
