package git

import (
	"fmt"
	"strings"
)

// DiffStats holds statistics about the changes in a diff
type DiffStats struct {
	// Content is the full diff content
	Content string
	// Added is the number of added lines
	Added int
	// Removed is the number of removed lines
	Removed int
	// Error holds any error that occurred during diff computation
	// This allows propagating setup errors (like missing base commit) without breaking the flow
	Error error
}

func (d *DiffStats) IsEmpty() bool {
	return d.Added == 0 && d.Removed == 0 && d.Content == ""
}

// Diff returns the git diff between the worktree and the base branch along with statistics.
// Untracked files are listed as synthetic new-file diff headers without modifying the git index.
func (g *GitWorktree) Diff() *DiffStats {
	stats := &DiffStats{}

	// Diff tracked file changes against the base commit.
	// Fall back to HEAD when no base commit SHA is recorded (e.g. existing-branch worktrees).
	base := g.GetBaseCommitSHA()
	if base == "" {
		base = "HEAD"
	}
	trackedContent, err := g.runGitCommand(g.worktreePath, "--no-pager", "diff", base)
	if err != nil {
		stats.Error = err
		return stats
	}

	// List untracked files without touching the index.
	untrackedOutput, err := g.runGitCommand(g.worktreePath, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		stats.Error = err
		return stats
	}

	// Build synthetic diff headers for each untracked file so callers see them
	// as new files without us having staged anything via git add -N.
	var untrackedDiff strings.Builder
	for _, file := range strings.Split(strings.TrimSpace(untrackedOutput), "\n") {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		untrackedDiff.WriteString(fmt.Sprintf("diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n", file, file, file))
	}

	content := trackedContent + untrackedDiff.String()

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			stats.Added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			stats.Removed++
		}
	}
	stats.Content = content

	return stats
}
