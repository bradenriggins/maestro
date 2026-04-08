package cmd

import (
	"context"
	"fmt"
	"maestro/pkg/orchestration"
	"time"
)

// RunDispatchCmd dispatches a task to a named worker instance. It handles
// dependency validation, scheduled dispatch, and re-dispatch of existing tasks.
func RunDispatchCmd(instanceName, taskPrompt, redispatchTaskID string, afterDeps []string, schedule bool, duration, maxWait int) error {
	if schedule && len(afterDeps) > 0 {
		return NewExitError(1, "--schedule and --after cannot be used together")
	}

	if len(afterDeps) > 0 {
		store, storeErr := orchestration.NewTaskStore()
		if storeErr != nil {
			return NewExitError(1, "%v", storeErr)
		}
		for _, depID := range afterDeps {
			if _, depErr := store.Get(depID); depErr != nil {
				return NewExitError(1, "dependency task %q not found", depID)
			}
		}
	}

	var result *orchestration.DispatchResult
	var err error

	if schedule {
		maxWaitDur := time.Duration(maxWait) * time.Minute
		opts := orchestration.ScheduleOptions{
			DurationHintM: duration,
			MaxWait:       maxWaitDur,
		}
		result, err = orchestration.RunScheduledDispatch(context.Background(), instanceName, taskPrompt, redispatchTaskID, opts)
	} else {
		result, err = orchestration.RunDispatch(instanceName, taskPrompt, redispatchTaskID, afterDeps)
	}

	if err != nil {
		return err
	}

	if result.Pending {
		fmt.Printf("Queued task %s on %s (pending: waiting on %d dependencies)\n",
			result.TaskID, result.InstanceName, len(afterDeps))
	} else {
		fmt.Printf("✓ Dispatched task %s to %s\n", result.TaskID, result.InstanceName)
		fmt.Printf("  Monitor progress: maestro status %s\n", result.InstanceName)
		fmt.Printf("  View output:      maestro output %s\n", result.InstanceName)
	}
	return nil
}
