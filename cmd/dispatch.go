package cmd

import (
	"context"
	"fmt"
	"maestro/pkg/orchestration"
	"os"
	"time"
)

// RunDispatchCmd dispatches a task to a named worker instance. It handles
// dependency validation, scheduled dispatch, and re-dispatch of existing tasks.
func RunDispatchCmd(instanceName, taskPrompt, redispatchTaskID string, afterDeps []string, schedule bool, duration, maxWait int) error {
	// Validate that --schedule and --after are not both set
	if schedule && len(afterDeps) > 0 {
		fmt.Fprintf(os.Stderr, "Error: --schedule and --after cannot be used together\n")
		os.Exit(1)
	}

	// Validate that all referenced deps exist before dispatching
	if len(afterDeps) > 0 {
		store, storeErr := orchestration.NewTaskStore()
		if storeErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", storeErr)
			os.Exit(1)
		}
		for _, depID := range afterDeps {
			if _, depErr := store.Get(depID); depErr != nil {
				fmt.Fprintf(os.Stderr, "Error: dependency task %q not found\n", depID)
				os.Exit(1)
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
		if de, ok := err.(*orchestration.DispatchError); ok {
			fmt.Fprintf(os.Stderr, "Error: %s\n", de.Msg)
			os.Exit(de.Code)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
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
