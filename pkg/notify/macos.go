package notify

import (
	"fmt"
	"os/exec"
	"sync"
	"time"
)

var (
	lastNotifyTime time.Time
	mu             sync.Mutex
	bundleWindow   = 5 * time.Second
)

// Notify sends a macOS notification via osascript.
// Notifications within 5 seconds of each other are bundled.
func Notify(title, message string) {
	mu.Lock()
	defer mu.Unlock()

	// Bundle: skip if last notification was within 5 seconds
	if time.Since(lastNotifyTime) < bundleWindow {
		return
	}
	lastNotifyTime = time.Now()

	go sendNotification(title, message)
}

func sendNotification(title, message string) {
	script := fmt.Sprintf(`display notification %q with title %q`, message, title)
	exec.Command("osascript", "-e", script).Run()
}

// NotifyTaskCompleted sends a notification for a completed task.
func NotifyTaskCompleted(worker, taskSummary string) {
	Notify("Claude Conductor", fmt.Sprintf("%s completed: %s", worker, taskSummary))
}

// NotifyTaskFailed sends a notification for a failed task.
func NotifyTaskFailed(worker, errorMsg string) {
	Notify("Claude Conductor", fmt.Sprintf("%s FAILED: %s", worker, errorMsg))
}

// NotifyWorkerStalled sends a notification when a worker is stalled.
func NotifyWorkerStalled(worker string, duration time.Duration) {
	Notify("Claude Conductor", fmt.Sprintf("%s stalled (no activity for %s)", worker, duration.Round(time.Second)))
}

// NotifyAllDone sends a notification when all tasks are done.
func NotifyAllDone(completedCount int) {
	Notify("Claude Conductor", fmt.Sprintf("All %d tasks completed. Ready for review.", completedCount))
}
