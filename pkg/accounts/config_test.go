package accounts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_MinimumOneAccount(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Accounts = nil
	err := cfg.Validate()
	assert.ErrorContains(t, err, "at least one account")
}

func TestValidate_MaxOneOrchestrator(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Accounts = []Account{
		{Name: "a", Role: RoleOrchestrator, ConfigDir: "/tmp/a"},
		{Name: "b", Role: RoleOrchestrator, ConfigDir: "/tmp/b"},
	}
	err := cfg.Validate()
	assert.ErrorContains(t, err, "at most one account")
}

func TestValidate_ZeroOrchestratorsAllowed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Accounts = []Account{
		{Name: "a", Role: RoleWorker, ConfigDir: "/tmp/a"},
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_DuplicateNames(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Accounts = []Account{
		{Name: "worker", Role: RoleWorker, ConfigDir: "/tmp/a"},
		{Name: "worker", Role: RoleWorker, ConfigDir: "/tmp/b"},
	}
	err := cfg.Validate()
	assert.ErrorContains(t, err, "duplicate account name")
}

func TestValidate_InvalidNameFormat(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Accounts = []Account{
		{Name: "My Account", Role: RoleWorker, ConfigDir: "/tmp/a"},
	}
	err := cfg.Validate()
	assert.ErrorContains(t, err, "must match")
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Accounts = []Account{
		{Name: "main", Role: RoleOrchestrator, ConfigDir: "/tmp/main"},
		{Name: "worker-1", Role: RoleWorker, ConfigDir: "/tmp/w1"},
		{Name: "worker-2", Role: RoleWorker, ConfigDir: "/tmp/w2"},
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestGetAccount(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Accounts = []Account{
		{Name: "main", Role: RoleOrchestrator, ConfigDir: "/tmp/main"},
		{Name: "worker-1", Role: RoleWorker, ConfigDir: "/tmp/w1"},
	}
	acct, err := cfg.GetAccount("worker-1")
	require.NoError(t, err)
	assert.Equal(t, "worker-1", acct.Name)
	assert.Equal(t, RoleWorker, acct.Role)

	_, err = cfg.GetAccount("nonexistent")
	assert.Error(t, err)
}

func TestGetOrchestratorAccount(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Accounts = []Account{
		{Name: "main", Role: RoleOrchestrator, ConfigDir: "/tmp/main"},
		{Name: "worker-1", Role: RoleWorker, ConfigDir: "/tmp/w1"},
	}
	orch := cfg.GetOrchestratorAccount()
	require.NotNil(t, orch)
	assert.Equal(t, "main", orch.Name)
}
