package orchestration

import (
	"fmt"
	"time"
)

const MaxAttempts = 3

const DefaultStallThreshold = 30 * time.Second

const (
	PollInterval = 2 * time.Second
	PollTimeout  = 30 * time.Second
)

const (
	StatusDispatched = "dispatched"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusTimedOut   = "timed_out"
	StatusStale      = "stale"
	StatusPending    = "pending"
	StatusBlocked    = "blocked"
)

const (
	StateIdle        = "idle"
	StateWorking     = "working"
	StateRateLimited = "rate_limited"
)

// Registry status constants
const (
	RegistryStatusRunning   = "running"
	RegistryStatusStarting  = "starting"
	RegistryStatusPaused    = "paused"
	RegistryStatusDead      = "dead"
	RegistryStatusCompleted = "completed"
)

type Task struct {
	ID               string   `json:"id"`
	Status           string   `json:"status"`
	WorkerInstance   string   `json:"worker_instance"`
	WorkerAccount    string   `json:"worker_account"`
	PromptFile       string   `json:"prompt_file"`
	ResultFile       string   `json:"result_file"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
	DispatchedAt     *string  `json:"dispatched_at,omitempty"`
	CompletedAt      *string  `json:"completed_at,omitempty"`
	Error            *string  `json:"error,omitempty"`
	Attempts         int      `json:"attempts"`
	SendAttempts     int      `json:"send_attempts,omitempty"`
	DispatchedBy     string   `json:"dispatched_by"`
	InferredCategory string   `json:"inferred_category,omitempty"`
	DependsOn        []string `json:"depends_on,omitempty"`
	PipelineID       string   `json:"pipeline_id,omitempty"`
}

type WorkerStatus struct {
	State     string `json:"state"`
	LastTask  string `json:"last_task"`
	Timestamp string `json:"timestamp"`
}

type RegistryEntry struct {
	Account      string  `json:"account"`
	Role         string  `json:"role"`
	Program      string  `json:"program,omitempty"`
	Model        string  `json:"model,omitempty"`
	TmuxSession  string  `json:"tmux_session"`
	WorktreePath string  `json:"worktree_path"`
	Branch       string  `json:"branch"`
	Status       string  `json:"status"`
	CreatedAt    string  `json:"created_at"`
	LastOutputAt string  `json:"last_output_at"`
	DiedAt       *string `json:"died_at"`
}

type Registry struct {
	Instances map[string]RegistryEntry `json:"instances"`
	UpdatedAt string                   `json:"updated_at"`
}

func (t *Task) IsTerminal() bool {
	return t.Status == StatusCompleted || t.Status == StatusFailed || t.Status == StatusTimedOut
}

func NowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ParseISO parses a timestamp written by either the Go side or the worker
// protocol. time.Parse(time.RFC3339) is strict and rejects common near-valid
// forms (fractional seconds, "+00:00" offset) an LLM-driven worker may emit
// when it formats a timestamp itself; a parse failure silently strands tasks
// in stale or skips staleness checks. Try the strict form, then tolerant ones.
func ParseISO(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05.999999999Z0700", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized timestamp format: %q", s)
}
