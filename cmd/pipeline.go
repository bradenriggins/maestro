package cmd

import (
	"fmt"
	"os"

	"maestro/pkg/orchestration"
)

// RunPipeline loads and executes a task pipeline from a YAML definition file,
// printing the created task IDs and any errors encountered. Zero-dependency
// tasks are immediately delivered to their workers' tmux sessions.
func RunPipeline(filePath string) error {
	p, pErr := orchestration.LoadPipeline(filePath)
	if pErr != nil {
		return fmt.Errorf("failed to load pipeline: %w", pErr)
	}

	store, storeErr := orchestration.NewTaskStore()
	if storeErr != nil {
		return fmt.Errorf("failed to create task store: %w", storeErr)
	}

	pipeResult, runErr := orchestration.RunPipeline(p, store)
	if runErr != nil {
		return fmt.Errorf("pipeline failed: %w", runErr)
	}

	// Deliver zero-dep tasks to their workers' tmux sessions.
	if len(pipeResult.Dispatched) > 0 {
		reg, regErr := orchestration.LoadRegistry()
		if regErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load registry for tmux delivery: %v\n", regErr)
		} else {
			orchestration.SendDAGDispatchedTasks(pipeResult.Dispatched, store, reg)
		}
	}

	fmt.Printf("Pipeline %q: created %d tasks\n", pipeResult.PipelineName, len(pipeResult.TaskIDs))
	for logicalID, realID := range pipeResult.TaskIDs {
		fmt.Printf("  %s -> %s\n", logicalID, realID)
	}
	if len(pipeResult.Errors) > 0 {
		fmt.Println("Errors:")
		for _, e := range pipeResult.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}
	return nil
}
