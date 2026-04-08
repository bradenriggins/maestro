package orchestration

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GitSafetyResult holds the results of the git safety net setup.
type GitSafetyResult struct {
	StashRef      string
	StartTag      string
	HadDirtyFiles bool
}

// SetupGitSafetyNet stashes uncommitted changes and creates a session start tag.
// Returns the stash ref and tag name, or empty strings if not applicable.
func SetupGitSafetyNet(repoDir string) (*GitSafetyResult, error) {
	result := &GitSafetyResult{}

	// Check for uncommitted changes
	cmd := exec.Command("git", "-C", repoDir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	now := time.Now()
	if len(strings.TrimSpace(string(out))) > 0 {
		result.HadDirtyFiles = true
		// Stash changes
		stashMsg := fmt.Sprintf("maestro-auto-stash-%d", now.Unix())
		stashCmd := exec.Command("git", "-C", repoDir, "stash", "push", "-m", stashMsg)
		stashOut, err := stashCmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("git stash failed: %s: %w", string(stashOut), err)
		}
		// Only record a stash ref if git actually created a stash entry.
		// "git stash" prints "Saved working directory and index state ..." on success.
		// If nothing was stashable it prints "No local changes to save" without error,
		// so we guard against setting a stash ref in that case.
		stashOutStr := string(stashOut)
		if strings.Contains(stashOutStr, "Saved") {
			// Parse the stash ref from the output, e.g. "stash@{0}" or fall back to stash@{0}
			// git stash output: "Saved working directory and index state On <branch>: <msg>"
			// The ref is always stash@{0} right after a successful stash push, but we
			// confirm it was truly saved before recording it.
			result.StashRef = "stash@{0}"
		}
	}

	// Create start tag
	tagName := fmt.Sprintf("maestro/session-start/%d", now.Unix())
	tagCmd := exec.Command("git", "-C", repoDir, "tag", tagName)
	if tagOut, err := tagCmd.CombinedOutput(); err != nil {
		// Non-fatal — tag creation can fail if HEAD hasn't changed
		_ = tagOut
	} else {
		result.StartTag = tagName
	}

	return result, nil
}

// TeardownGitSafetyNet shows changes since start and offers to pop stash.
// Returns a summary string for display.
func TeardownGitSafetyNet(repoDir, startTag, stashRef string) string {
	var summary strings.Builder

	if startTag != "" {
		// Show changes since start
		cmd := exec.Command("git", "-C", repoDir, "diff", "--stat", startTag+"..HEAD")
		out, err := cmd.Output()
		if err == nil && len(strings.TrimSpace(string(out))) > 0 {
			summary.WriteString("Changes since session start:\n")
			summary.WriteString(string(out))
			summary.WriteString("\n")
		}
		// Clean up the session start tag so they don't accumulate.
		delCmd := exec.Command("git", "-C", repoDir, "tag", "-d", startTag)
		_ = delCmd.Run()
	}

	if stashRef != "" {
		summary.WriteString(fmt.Sprintf("Auto-stash from session start available: %s\n", stashRef))
		summary.WriteString("Run `git stash pop` to restore your uncommitted changes.\n")
	}

	return summary.String()
}
