package accounts

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_MinimumTwoAccounts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Accounts = nil
	err := cfg.Validate()
	assert.ErrorContains(t, err, "at least two accounts")

	// A single account also fails
	cfg.Accounts = []Account{
		{Name: "main", Role: RoleOrchestrator, ConfigDir: "/tmp/main"},
	}
	err = cfg.Validate()
	assert.ErrorContains(t, err, "at least two accounts")
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

func TestValidate_OrchestratorRequired(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Accounts = []Account{
		{Name: "worker-1", Role: RoleWorker, ConfigDir: "/tmp/a"},
		{Name: "worker-2", Role: RoleWorker, ConfigDir: "/tmp/b"},
	}
	err := cfg.Validate()
	assert.ErrorContains(t, err, "at least one account must have")
}

func TestValidate_DuplicateNames(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Accounts = []Account{
		{Name: "main", Role: RoleOrchestrator, ConfigDir: "/tmp/main"},
		{Name: "worker", Role: RoleWorker, ConfigDir: "/tmp/a"},
		{Name: "worker", Role: RoleWorker, ConfigDir: "/tmp/b"},
	}
	err := cfg.Validate()
	assert.ErrorContains(t, err, "duplicate account name")
}

func TestValidate_InvalidNameFormat(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Accounts = []Account{
		{Name: "main", Role: RoleOrchestrator, ConfigDir: "/tmp/main"},
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

// TestConductorConfigRoundTrip verifies that every field in ConductorConfig,
// including the Program and Model fields on Account, survives a JSON marshal→unmarshal
// cycle with no loss or mutation.
func TestConductorConfigRoundTrip(t *testing.T) {
	original := &ConductorConfig{
		ProtocolVersion:   "1",
		DefaultProgram:    "claude",
		BranchPrefix:      "conductor/",
		AutoYes:           true,
		PostWorktreeSetup: "make install",
		Notifications: NotificationConfig{
			Enabled:       true,
			TaskCompleted: true,
			TaskFailed:    true,
			WorkerStalled: false,
			AllDone:       true,
		},
		StallWarnSeconds:    45,
		StallTimeoutSeconds: 120,
		AutoCleanup:         false,
		Accounts: []Account{
			{
				Name:      "main",
				Role:      RoleOrchestrator,
				ConfigDir: "/home/user/.config/claude-orch",
				Email:     "orch@example.com",
				Program:   "claude",
				Model:     "sonnet-4.6",
				Verified:  true,
			},
			{
				Name:      "worker-1",
				Role:      RoleWorker,
				ConfigDir: "/home/user/.config/claude-w1",
				Email:     "w1@example.com",
				Program:   "codex",
				Model:     "gpt-5.3-codex",
				Verified:  false,
			},
			{
				Name:      "worker-2",
				Role:      RoleWorker,
				ConfigDir: "/home/user/.config/claude-w2",
				Email:     "w2@example.com",
				// Program and Model intentionally empty — should round-trip as empty string
				Verified: true,
			},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err, "marshal should not fail")

	var got ConductorConfig
	require.NoError(t, json.Unmarshal(data, &got), "unmarshal should not fail")

	// Top-level fields
	assert.Equal(t, original.ProtocolVersion, got.ProtocolVersion)
	assert.Equal(t, original.DefaultProgram, got.DefaultProgram)
	assert.Equal(t, original.BranchPrefix, got.BranchPrefix)
	assert.Equal(t, original.AutoYes, got.AutoYes)
	assert.Equal(t, original.PostWorktreeSetup, got.PostWorktreeSetup)
	assert.Equal(t, original.StallWarnSeconds, got.StallWarnSeconds)
	assert.Equal(t, original.StallTimeoutSeconds, got.StallTimeoutSeconds)
	assert.Equal(t, original.AutoCleanup, got.AutoCleanup)

	// Notification sub-struct
	assert.Equal(t, original.Notifications, got.Notifications)

	// Accounts length
	require.Len(t, got.Accounts, len(original.Accounts))

	for i, want := range original.Accounts {
		got := got.Accounts[i]
		assert.Equal(t, want.Name, got.Name, "account[%d].Name", i)
		assert.Equal(t, want.Role, got.Role, "account[%d].Role", i)
		assert.Equal(t, want.ConfigDir, got.ConfigDir, "account[%d].ConfigDir", i)
		assert.Equal(t, want.Email, got.Email, "account[%d].Email", i)
		assert.Equal(t, want.Program, got.Program, "account[%d].Program", i)
		assert.Equal(t, want.Model, got.Model, "account[%d].Model", i)
		assert.Equal(t, want.Verified, got.Verified, "account[%d].Verified", i)
	}
}

// TestConductorConfigRoundTrip_ProgramCodex specifically verifies that Program="codex"
// is preserved after a write→read cycle (it uses omitempty, so this is the key case).
func TestConductorConfigRoundTrip_ProgramCodex(t *testing.T) {
	original := &ConductorConfig{
		ProtocolVersion:     "1",
		DefaultProgram:      "claude",
		BranchPrefix:        "conductor/",
		Notifications:       NotificationConfig{Enabled: true},
		StallWarnSeconds:    30,
		StallTimeoutSeconds: 90,
		Accounts: []Account{
			{Name: "main", Role: RoleOrchestrator, ConfigDir: "/tmp/main"},
			{Name: "codex-worker", Role: RoleWorker, ConfigDir: "/tmp/cw", Program: "codex", Model: "gpt-5.3-codex"},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Confirm "codex" appears in the JSON bytes (omitempty should not suppress it)
	assert.Contains(t, string(data), `"codex"`, "Program=codex must appear in marshaled JSON")

	var got ConductorConfig
	require.NoError(t, json.Unmarshal(data, &got))

	codexWorker, err := got.GetAccount("codex-worker")
	require.NoError(t, err)
	assert.Equal(t, "codex", codexWorker.Program, "Program should round-trip as 'codex'")
	assert.Equal(t, "gpt-5.3-codex", codexWorker.Model, "Model should round-trip correctly")
}

// TestConductorConfigRoundTrip_EmptyProgramModel verifies that accounts with empty
// Program and Model strings survive a round-trip as empty strings (not as some other value).
func TestConductorConfigRoundTrip_EmptyProgramModel(t *testing.T) {
	original := &ConductorConfig{
		ProtocolVersion:     "1",
		DefaultProgram:      "claude",
		BranchPrefix:        "conductor/",
		Notifications:       NotificationConfig{Enabled: true},
		StallWarnSeconds:    30,
		StallTimeoutSeconds: 90,
		Accounts: []Account{
			{Name: "main", Role: RoleOrchestrator, ConfigDir: "/tmp/main"},
			{Name: "worker-1", Role: RoleWorker, ConfigDir: "/tmp/w1"},
			// Program and Model are zero-value ("") — should not appear in JSON (omitempty)
			// but must unmarshal back as "".
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	// omitempty: these keys should be absent from the JSON (empty string suppressed)
	assert.NotContains(t, string(data), `"program"`, "empty Program should be omitted by omitempty")
	assert.NotContains(t, string(data), `"model"`, "empty Model should be omitted by omitempty")

	var got ConductorConfig
	require.NoError(t, json.Unmarshal(data, &got))

	worker, err := got.GetAccount("worker-1")
	require.NoError(t, err)
	assert.Equal(t, "", worker.Program, "missing Program key should unmarshal as empty string")
	assert.Equal(t, "", worker.Model, "missing Model key should unmarshal as empty string")
}
