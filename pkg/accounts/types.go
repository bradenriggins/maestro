package accounts

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type Role string

const (
	RoleOrchestrator Role = "orchestrator"
	RoleWorker       Role = "worker"
)

type Account struct {
	Name      string `json:"name"`
	Role      Role   `json:"role"`
	ConfigDir string `json:"config_dir"`
	Email     string `json:"email"`
	Verified  bool   `json:"verified"`
}

type NotificationConfig struct {
	Enabled       bool `json:"enabled"`
	TaskCompleted bool `json:"task_completed"`
	TaskFailed    bool `json:"task_failed"`
	WorkerStalled bool `json:"worker_stalled"`
	AllDone       bool `json:"all_done"`
}

type ConductorConfig struct {
	ProtocolVersion     string             `json:"protocol_version"`
	Accounts            []Account          `json:"accounts"`
	DefaultProgram      string             `json:"default_program"`
	BranchPrefix        string             `json:"branch_prefix"`
	AutoYes             bool               `json:"auto_yes"`
	PostWorktreeSetup   string             `json:"post_worktree_setup"`
	Notifications       NotificationConfig `json:"notifications"`
	StallWarnSeconds    int                `json:"stall_warn_seconds"`
	StallTimeoutSeconds int                `json:"stall_timeout_seconds"`
	AutoCleanup         bool               `json:"auto_cleanup"`
}

var validNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func (c *ConductorConfig) Validate() error {
	if len(c.Accounts) == 0 {
		return fmt.Errorf("at least one account is required")
	}
	orchestratorCount := 0
	names := make(map[string]bool)
	for _, acct := range c.Accounts {
		if !validNameRegex.MatchString(acct.Name) {
			return fmt.Errorf("account name %q must match /^[a-z0-9][a-z0-9-]*$/", acct.Name)
		}
		if names[acct.Name] {
			return fmt.Errorf("duplicate account name: %q", acct.Name)
		}
		names[acct.Name] = true
		if acct.Role != RoleOrchestrator && acct.Role != RoleWorker {
			return fmt.Errorf("account %q has invalid role %q (must be %q or %q)", acct.Name, acct.Role, RoleOrchestrator, RoleWorker)
		}
		if acct.Role == RoleOrchestrator {
			orchestratorCount++
		}
	}
	if orchestratorCount > 1 {
		return fmt.Errorf("at most one account may have role %q, found %d", RoleOrchestrator, orchestratorCount)
	}
	return nil
}

func (c *ConductorConfig) GetAccount(name string) (*Account, error) {
	for i := range c.Accounts {
		if c.Accounts[i].Name == name {
			return &c.Accounts[i], nil
		}
	}
	return nil, fmt.Errorf("account %q not found", name)
}

func (c *ConductorConfig) GetOrchestratorAccount() *Account {
	for i := range c.Accounts {
		if c.Accounts[i].Role == RoleOrchestrator {
			return &c.Accounts[i]
		}
	}
	return nil
}

func (c *ConductorConfig) GetWorkerAccounts() []Account {
	var workers []Account
	for _, acct := range c.Accounts {
		if acct.Role == RoleWorker {
			workers = append(workers, acct)
		}
	}
	return workers
}

func ConductorDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".claude-conductor"), nil
}

func AccountConfigDir(accountName string) (string, error) {
	base, err := ConductorDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "accounts", accountName), nil
}
