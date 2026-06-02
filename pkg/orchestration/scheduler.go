package orchestration

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"maestro/pkg/accounts"
)

const (
	// EstimatedMsgRatePerMin is the estimated number of messages a worker sends per minute.
	EstimatedMsgRatePerMin = 3.0

	// DefaultTaskDurationMin is the fallback task duration if no hint or historical data exists.
	DefaultTaskDurationMin = 15

	// DefaultMaxWaitDuration is the maximum time the scheduler will wait for a worker to reset.
	DefaultMaxWaitDuration = 2 * time.Hour

	// SchedulerSaturationThreshold is the usage percentage above which the scheduler
	// considers a worker saturated for immediate dispatch purposes.
	SchedulerSaturationThreshold = 80.0
)

// WorkerProjection represents a candidate worker's projected ability to handle a task.
type WorkerProjection struct {
	AccountName    string
	UsagePercent   float64
	MessagesLimit  int
	WindowResetAt  time.Time
	AvailableAt    time.Time
	WaitDuration   time.Duration
	ProjectedUsage float64
	Score          float64
	Immediate      bool
}

// ScheduleResult is the output of ComputeSchedule.
type ScheduleResult struct {
	Worker       WorkerProjection
	TaskDuration time.Duration
	NoRouteFound bool
}

// ScheduleOptions configures the scheduler behavior for a dispatch.
type ScheduleOptions struct {
	DurationHintM int
	MaxWait       time.Duration
}

// ComputeSchedule evaluates all workers and picks the best candidate for a task.
// It considers both immediately available workers and workers that will become available
// after their rate-limit window resets.
func ComputeSchedule(workers []AccountUsage, taskDuration time.Duration, now time.Time) ScheduleResult {
	neededMessages := math.Ceil(taskDuration.Minutes() * EstimatedMsgRatePerMin)

	var candidates []WorkerProjection

	for _, w := range workers {
		// Skip orchestrators
		if w.Role == string(accounts.RoleOrchestrator) {
			continue
		}

		// Skip workers with zero message limit — they cannot handle any task
		// and would cause a division by zero in projected usage calculations.
		if w.MessagesLimit <= 0 {
			continue
		}

		headroom := float64(w.MessagesLimit) * (1.0 - w.UsagePercent/100.0)
		proj := WorkerProjection{
			AccountName:   w.AccountName,
			UsagePercent:  w.UsagePercent,
			MessagesLimit: w.MessagesLimit,
		}

		// Parse reset time if available
		if w.WindowResetAt != "" {
			if t, err := time.Parse(time.RFC3339, w.WindowResetAt); err == nil {
				proj.WindowResetAt = t
			}
		}

		// Check if worker can handle the task immediately
		if headroom >= neededMessages && w.UsagePercent < SchedulerSaturationThreshold {
			proj.Immediate = true
			proj.AvailableAt = now
			proj.WaitDuration = 0
			// Score = projected usage after task (lower is better)
			proj.ProjectedUsage = w.UsagePercent + (neededMessages/float64(w.MessagesLimit))*100.0
			proj.Score = proj.ProjectedUsage
			candidates = append(candidates, proj)
		} else if !proj.WindowResetAt.IsZero() && proj.WindowResetAt.After(now) {
			// Post-reset option: worker will reset and be fresh
			wait := proj.WindowResetAt.Sub(now)
			proj.Immediate = false
			proj.AvailableAt = proj.WindowResetAt
			proj.WaitDuration = wait
			// After reset, usage goes to 0; projected = just the task's usage
			proj.ProjectedUsage = (neededMessages / float64(w.MessagesLimit)) * 100.0
			// Score = wait_minutes + projected_usage (lower is better)
			proj.Score = wait.Minutes() + proj.ProjectedUsage
			candidates = append(candidates, proj)
		}
	}

	if len(candidates) == 0 {
		return ScheduleResult{NoRouteFound: true, TaskDuration: taskDuration}
	}

	// Sort: immediate always beats waiting, then by score ascending
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Immediate != candidates[j].Immediate {
			return candidates[i].Immediate
		}
		return candidates[i].Score < candidates[j].Score
	})

	return ScheduleResult{
		Worker:       candidates[0],
		TaskDuration: taskDuration,
	}
}

// RunScheduledDispatch loads usage data, computes the optimal schedule, and dispatches
// the task -- potentially waiting for a worker's rate-limit window to reset first.
func RunScheduledDispatch(ctx context.Context, instanceName, taskPrompt, redispatchTaskID string, opts ScheduleOptions) (*DispatchResult, error) {
	cfg, err := accounts.LoadConductorConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load conductor config: %w", err)
	}
	if cfg == nil {
		return nil, fmt.Errorf("no config found; run 'maestro setup' first")
	}

	report, err := CollectUsage(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to collect usage: %w", err)
	}

	taskDuration := ResolveTaskDuration(opts.DurationHintM, instanceName, nil)

	maxWait := opts.MaxWait
	if maxWait == 0 {
		maxWait = DefaultMaxWaitDuration
	}

	schedule := ComputeSchedule(report.Accounts, taskDuration, time.Now())

	if schedule.NoRouteFound {
		return nil, &DispatchError{
			Code: 6,
			Msg:  "no worker available within max wait window; all workers saturated with no known reset times",
		}
	}

	// Exclude candidates whose wait exceeds MaxWait
	if schedule.Worker.WaitDuration > maxWait {
		return nil, &DispatchError{
			Code: 6,
			Msg: fmt.Sprintf("best available worker %q resets in %s, which exceeds max wait of %s",
				schedule.Worker.AccountName,
				schedule.Worker.WaitDuration.Round(time.Second),
				maxWait.Round(time.Second)),
		}
	}

	if schedule.Worker.Immediate {
		target := resolveScheduledInstance(schedule.Worker.AccountName, instanceName)
		fmt.Printf("Dispatching immediately to %q [account %q] (usage: %.1f%%)\n",
			target, schedule.Worker.AccountName, schedule.Worker.UsagePercent)
		return RunDispatch(target, taskPrompt, redispatchTaskID, nil)
	}

	// Wait for the worker's rate-limit window to reset
	wait := schedule.Worker.WaitDuration
	fmt.Printf("Scheduling for %q in %s (reset at %s)\n",
		schedule.Worker.AccountName,
		wait.Round(time.Second),
		schedule.Worker.AvailableAt.Format("15:04 UTC"))

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-timer.C:
		// Re-collect usage and recompute the schedule — the pre-wait estimate
		// can be stale (clock skew, or another process used the account during
		// the wait), so dispatching blind here risks hitting a still-saturated
		// worker after a long wait.
		fmt.Printf("Wait complete. Re-checking usage...\n")
		if report2, err := CollectUsage(cfg); err == nil {
			if rescheduled := ComputeSchedule(report2.Accounts, taskDuration, time.Now()); !rescheduled.NoRouteFound {
				schedule = rescheduled
			}
		}
		target := resolveScheduledInstance(schedule.Worker.AccountName, instanceName)
		fmt.Printf("Dispatching to %q [account %q] (usage: %.1f%%)\n",
			target, schedule.Worker.AccountName, schedule.Worker.UsagePercent)
		return RunDispatch(target, taskPrompt, redispatchTaskID, nil)
	case <-ctx.Done():
		return nil, fmt.Errorf("scheduling canceled: %w", ctx.Err())
	}
}

// resolveScheduledInstance maps a scheduler-chosen account back to a worker
// instance name. The scheduler routes by account, but RunDispatch targets an
// instance; without this resolution the routing decision is silently discarded
// and the task always lands on the originally-named instance. Falls back to the
// requested instance if no live worker backs the chosen account.
func resolveScheduledInstance(accountName, fallbackInstance string) string {
	reg, err := LoadRegistry()
	if err != nil {
		return fallbackInstance
	}
	var matches []string
	for name, entry := range reg.ListWorkers() {
		if entry.Account == accountName && entry.Status != RegistryStatusDead {
			matches = append(matches, name)
		}
	}
	if len(matches) == 0 {
		return fallbackInstance
	}
	sort.Strings(matches) // deterministic when several instances back one account
	return matches[0]
}

// ResolveTaskDuration determines the task duration from a flag, historical stats, or the default.
func ResolveTaskDuration(durationFlag int, workerAccount string, stats *DurationStats) time.Duration {
	if durationFlag > 0 {
		return time.Duration(durationFlag) * time.Minute
	}
	if stats != nil {
		if avg, ok := stats.AverageFor(workerAccount); ok {
			return avg
		}
	}
	return time.Duration(DefaultTaskDurationMin) * time.Minute
}
