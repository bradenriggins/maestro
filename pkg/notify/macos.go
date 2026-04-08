package notify

import (
	"fmt"
	"maestro/log"
	"os/exec"
	"strings"
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
	// Sanitize: replace single quotes to prevent AppleScript injection.
	// AppleScript single-quoted strings have no escape mechanism, so we
	// substitute the Unicode right single quotation mark (U+2019) instead.
	sanitize := func(s string) string {
		return strings.ReplaceAll(s, "'", "\u2019")
	}
	script := fmt.Sprintf(`display notification '%s' with title '%s'`, sanitize(message), sanitize(title))
	if err := exec.Command("osascript", "-e", script).Run(); err != nil {
		log.ErrorLog.Printf("notification failed: %v", err)
	}
}

// NotifyTaskCompleted sends a notification for a completed task.
func NotifyTaskCompleted(worker, taskSummary string) {
	Notify("Maestro", fmt.Sprintf("%s completed: %s", worker, taskSummary))
}

// NotifyUrgent sends a macOS notification immediately, bypassing the bundle
// window.  Use this for high-priority events (e.g. task failures) where the
// user must be informed without delay.
func NotifyUrgent(title, message string) {
	mu.Lock()
	lastNotifyTime = time.Now()
	mu.Unlock()

	go sendNotification(title, message)
}

// NotifyTaskFailed sends a notification for a failed task.
// It bypasses the bundle window so the user is informed immediately.
func NotifyTaskFailed(worker, errorMsg string) {
	NotifyUrgent("Maestro", fmt.Sprintf("%s FAILED: %s", worker, errorMsg))
}

// NotifyWorkerStalled sends a notification when a worker is stalled.
func NotifyWorkerStalled(worker string, duration time.Duration) {
	Notify("Maestro", fmt.Sprintf("%s stalled (no activity for %s)", worker, duration.Round(time.Second)))
}

// NotifyAllDone sends a notification when all tasks are done.
func NotifyAllDone(completedCount int) {
	Notify("Maestro", fmt.Sprintf("All %d tasks completed. Ready for review.", completedCount))
}
