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

	if len(strings.TrimSpace(string(out))) > 0 {
		result.HadDirtyFiles = true
		// Stash changes
		ts := time.Now().Unix()
		stashMsg := fmt.Sprintf("conductor-auto-stash-%d", ts)
		stashCmd := exec.Command("git", "-C", repoDir, "stash", "push", "-m", stashMsg)
		if stashOut, err := stashCmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("git stash failed: %s: %w", string(stashOut), err)
		}
		result.StashRef = "stash@{0}"
	}

	// Create start tag
	ts := time.Now().Unix()
	tagName := fmt.Sprintf("conductor/session-start/%d", ts)
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
	}

	if stashRef != "" {
		summary.WriteString(fmt.Sprintf("Auto-stash from session start available: %s\n", stashRef))
		summary.WriteString("Run `git stash pop` to restore your uncommitted changes.\n")
	}

	return summary.String()
}
