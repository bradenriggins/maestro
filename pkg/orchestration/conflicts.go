package orchestration

import (
	"os/exec"
	"strings"
)

// FileConflict represents two workers editing the same file.
type FileConflict struct {
	FilePath string
	Workers  []string
}

// DetectConflicts checks all active worktrees for overlapping file modifications.
func DetectConflicts(worktrees map[string]string) []FileConflict {
	// worktrees is a map of instance_name -> worktree_path

	// Get modified files per worktree
	fileToWorkers := make(map[string][]string)

	for name, path := range worktrees {
		files := getModifiedFiles(path)
		for _, f := range files {
			fileToWorkers[f] = append(fileToWorkers[f], name)
		}
	}

	// Find conflicts (files modified by 2+ workers)
	var conflicts []FileConflict
	for file, workers := range fileToWorkers {
		if len(workers) > 1 {
			conflicts = append(conflicts, FileConflict{
				FilePath: file,
				Workers:  workers,
			})
		}
	}

	return conflicts
}

func getModifiedFiles(worktreePath string) []string {
	cmd := exec.Command("git", "-C", worktreePath, "diff", "--name-only")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	// Also check staged files
	stagedCmd := exec.Command("git", "-C", worktreePath, "diff", "--name-only", "--cached")
	stagedOut, err := stagedCmd.Output()
	if err == nil {
		out = append(out, stagedOut...)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	// Deduplicate
	seen := make(map[string]bool)
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !seen[line] {
			seen[line] = true
			result = append(result, line)
		}
	}
	return result
}
