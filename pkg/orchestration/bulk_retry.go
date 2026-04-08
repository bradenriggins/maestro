package orchestration

import (
	"fmt"
	"sort"
)

// BulkRetryOptions controls the behavior of RunBulkRetry.
type BulkRetryOptions struct {
	MaxTasks   int    // 0 = unlimited
	WorkerName string // empty = round-robin across all idle workers
}

// BulkRetryResult holds the outcome of a bulk retry operation.
type BulkRetryResult struct {
	Retried     int
	Skipped     int // max attempts reached
	Failed      int // dispatch errors
	Total       int // total eligible tasks
	WorkersUsed []string
	Errors      []string
}

// RunBulkRetry retries all failed and timed-out tasks across idle workers.
func RunBulkRetry(opts BulkRetryOptions) (*BulkRetryResult, error) {
	result := &BulkRetryResult{}

	// 1. Create TaskStore
	store, err := NewTaskStore()
	if err != nil {
		return nil, fmt.Errorf("failed to create task store: %w", err)
	}

	// 2. Load failed and timed_out tasks, combine and sort by CreatedAt ascending
	failedTasks, err := store.List(StatusFailed)
	if err != nil {
		return nil, fmt.Errorf("failed to list failed tasks: %w", err)
	}
	timedOutTasks, err := store.List(StatusTimedOut)
	if err != nil {
		return nil, fmt.Errorf("failed to list timed-out tasks: %w", err)
	}

	eligible := append(failedTasks, timedOutTasks...)
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].CreatedAt < eligible[j].CreatedAt
	})

	// 3. If no eligible tasks, return early
	result.Total = len(eligible)
	if len(eligible) == 0 {
		return result, nil
	}

	// 4. Pre-filter: skip tasks that have reached MaxAttempts.
	// This must happen before the MaxTasks truncation so that MaxTasks limits
	// the number of tasks actually dispatched, not the number of candidates scanned.
	var dispatchList []*Task
	for _, task := range eligible {
		if task.Attempts >= MaxAttempts {
			result.Skipped++
		} else {
			dispatchList = append(dispatchList, task)
		}
	}

	// 5. Truncate dispatchList if MaxTasks is set
	if opts.MaxTasks > 0 && opts.MaxTasks < len(dispatchList) {
		dispatchList = dispatchList[:opts.MaxTasks]
	}

	// If nothing left to dispatch after filtering, return early
	if len(dispatchList) == 0 {
		return result, nil
	}

	// 6. Determine workers
	var workers []string

	if opts.WorkerName != "" {
		// Validate specific worker
		reg, err := LoadRegistry()
		if err != nil {
			return nil, fmt.Errorf("failed to load registry: %w", err)
		}
		if _, ok := reg.GetInstance(opts.WorkerName); !ok {
			return nil, fmt.Errorf("worker %q not found in registry", opts.WorkerName)
		}
		idle, err := IsWorkerIdle(opts.WorkerName)
		if err != nil {
			return nil, fmt.Errorf("failed to check worker %q status: %w", opts.WorkerName, err)
		}
		if !idle {
			return nil, fmt.Errorf("worker %q is not idle", opts.WorkerName)
		}
		workers = []string{opts.WorkerName}
	} else {
		// Round-robin across all idle workers
		reg, err := LoadRegistry()
		if err != nil {
			return nil, fmt.Errorf("failed to load registry: %w", err)
		}
		workerEntries := reg.ListWorkers()
		for name := range workerEntries {
			idle, err := IsWorkerIdle(name)
			if err != nil {
				continue
			}
			if idle {
				workers = append(workers, name)
			}
		}
		sort.Strings(workers)
	}

	if len(workers) == 0 {
		return nil, fmt.Errorf("no idle workers available")
	}

	// 7. Round-robin dispatch
	workerIdx := 0
	workersUsedSet := make(map[string]bool)
	for _, task := range dispatchList {
		dispatched := false
		for attempt := 0; attempt < len(workers); attempt++ {
			worker := workers[workerIdx]
			workerIdx = (workerIdx + 1) % len(workers)
			_, err := RunDispatch(worker, "", task.ID, nil)
			if err == nil {
				result.Retried++
				workersUsedSet[worker] = true
				dispatched = true
				break
			}
			// If worker was busy (Code 4), try next worker
			if de, ok := err.(*DispatchError); ok && de.Code == 4 {
				continue
			}
			// Any other error — give up on this task
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("task %s -> %s: %v", task.ID, worker, err))
			dispatched = true // counted as failed, don't retry
			break
		}
		if !dispatched {
			// All workers busy for this task
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("task %s: all workers busy", task.ID))
		}
	}

	// 8. Convert workersUsedSet to sorted slice
	for w := range workersUsedSet {
		result.WorkersUsed = append(result.WorkersUsed, w)
	}
	sort.Strings(result.WorkersUsed)

	return result, nil
}
