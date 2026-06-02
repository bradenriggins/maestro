package git

import (
	"fmt"
	"maestro/log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Setup creates a new worktree for the session
func (g *GitWorktree) Setup() error {
	if g.worktreePath == "" {
		return fmt.Errorf("worktree path is empty for session %q", g.sessionName)
	}

	// Ensure worktrees directory exists early (can be done in parallel with branch check)
	worktreesDir, err := getWorktreeDirectory()
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	if err := os.MkdirAll(worktreesDir, 0700); err != nil {
		return fmt.Errorf("failed to create worktree directory %s: %w", worktreesDir, err)
	}

	// If this worktree uses a pre-existing branch, always set up from that branch
	// (it may exist locally or only on the remote).
	if g.isExistingBranch {
		return g.setupFromExistingBranch()
	}

	// Check if branch exists using git CLI (much faster than go-git PlainOpen)
	_, err = g.runGitCommand(g.repoPath, "show-ref", "--verify", fmt.Sprintf("refs/heads/%s", g.branchName))
	if err == nil {
		return g.setupFromExistingBranch()
	}
	return g.setupNewWorktree()
}

// setupFromExistingBranch creates a worktree from an existing branch
func (g *GitWorktree) setupFromExistingBranch() error {
	// Directory already created in Setup(), skip duplicate creation

	// Clean up any existing worktree first
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath) // Ignore error if worktree doesn't exist

	// Check if the local branch exists
	_, localErr := g.runGitCommand(g.repoPath, "show-ref", "--verify", fmt.Sprintf("refs/heads/%s", g.branchName))
	if localErr != nil {
		// Local branch doesn't exist — check if remote tracking branch exists
		_, remoteErr := g.runGitCommand(g.repoPath, "show-ref", "--verify", fmt.Sprintf("refs/remotes/origin/%s", g.branchName))
		if remoteErr != nil {
			return fmt.Errorf("branch %s not found locally or on remote", g.branchName)
		}
		// Create a local tracking branch via worktree add -b
		if _, err := g.runGitCommand(g.repoPath, "worktree", "add", "-b", g.branchName, g.worktreePath, fmt.Sprintf("origin/%s", g.branchName)); err != nil {
			return fmt.Errorf("failed to create worktree from remote branch %s: %w", g.branchName, err)
		}
		return nil
	}

	// Create a new worktree from the existing local branch
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", g.worktreePath, g.branchName); err != nil {
		return fmt.Errorf("failed to create worktree from branch %s: %w", g.branchName, err)
	}

	return nil
}

// setupNewWorktree creates a new worktree from HEAD
func (g *GitWorktree) setupNewWorktree() error {
	// Clean up any existing worktree first
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath) // Ignore error if worktree doesn't exist

	// Safely clean up any existing branch reference.
	// Check if the branch exists before attempting deletion.
	listOutput, listErr := g.runGitCommand(g.repoPath, "branch", "--list", g.branchName)
	if listErr == nil && strings.TrimSpace(listOutput) != "" {
		// Branch exists — check if it is fully merged into HEAD before deleting.
		mergedOutput, mergedErr := g.runGitCommand(g.repoPath, "branch", "--merged", "HEAD", "--list", g.branchName)
		if mergedErr == nil && strings.TrimSpace(mergedOutput) != "" {
			// Branch is fully merged — safe to delete.
			_, _ = g.runGitCommand(g.repoPath, "branch", "-d", g.branchName)
		} else {
			// Branch has unmerged commits — generate a unique name to avoid data loss.
			g.branchName = fmt.Sprintf("%s-%x", g.branchName, time.Now().UnixNano())
		}
	}

	output, err := g.runGitCommand(g.repoPath, "rev-parse", "HEAD")
	if err != nil {
		if strings.Contains(err.Error(), "fatal: ambiguous argument 'HEAD'") ||
			strings.Contains(err.Error(), "fatal: not a valid object name") ||
			strings.Contains(err.Error(), "fatal: HEAD: not a valid object name") {
			return fmt.Errorf("this appears to be a brand new repository: please create an initial commit before creating an instance")
		}
		return fmt.Errorf("failed to get HEAD commit hash: %w", err)
	}
	headCommit := strings.TrimSpace(string(output))
	g.baseCommitSHA = headCommit

	// Create a new worktree from the current HEAD commit so each session starts
	// from a clean, stable snapshot of the repository state.
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", "-b", g.branchName, g.worktreePath, headCommit); err != nil {
		return fmt.Errorf("failed to create worktree from commit %s: %w", headCommit, err)
	}

	return nil
}

// Cleanup removes the worktree and associated branch
func (g *GitWorktree) Cleanup() error {
	var errs []error

	// Check if worktree path exists before attempting removal
	if _, err := os.Stat(g.worktreePath); err == nil {
		// Remove the worktree using git command
		if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath); err != nil {
			errs = append(errs, err)
		}
	} else if !os.IsNotExist(err) {
		// Only append error if it's not a "not exists" error
		errs = append(errs, fmt.Errorf("failed to check worktree path: %w", err))
	}

	// Delete the branch using git CLI, but skip if this is a pre-existing branch
	if !g.isExistingBranch {
		if _, err := g.runGitCommand(g.repoPath, "branch", "-D", g.branchName); err != nil {
			// Only log if it's not a "branch not found" error
			if !strings.Contains(err.Error(), "not found") {
				errs = append(errs, fmt.Errorf("failed to remove branch %s: %w", g.branchName, err))
			}
		}
	}

	// Prune the worktree to clean up any remaining references
	if err := g.Prune(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return g.combineErrors(errs)
	}

	return nil
}

// Remove removes the worktree but keeps the branch
func (g *GitWorktree) Remove() error {
	// Remove the worktree using git command
	if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	return nil
}

// Prune removes all working tree administrative files and directories
func (g *GitWorktree) Prune() error {
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}
	return nil
}

// CleanupWorktrees removes all worktrees and their associated branches.
// repoPath must be a path within the git repository to operate on.
func CleanupWorktrees(repoPath string) error {
	worktreesDir, err := getWorktreeDirectory()
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing to clean up if the directory doesn't exist yet.
			return nil
		}
		return fmt.Errorf("failed to read worktree directory: %w", err)
	}

	// Get a list of all branches associated with worktrees
	cmd := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	// Parse the output to extract branch names AND the set of worktree paths
	// that actually belong to THIS repo. The worktrees dir (~/.maestro/worktrees)
	// is GLOBAL across every repo maestro has touched, so we must never act on an
	// entry owned by a different repo — git's remove would fail and the
	// os.RemoveAll fallback would then destroy that other repo's worktree
	// (uncommitted work included) and corrupt its .git/worktrees registration.
	worktreeBranches := make(map[string]string)
	ownedBaseNames := make(map[string]bool)
	currentWorktree := ""
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			currentWorktree = strings.TrimPrefix(line, "worktree ")
			ownedBaseNames[filepath.Base(currentWorktree)] = true
		} else if strings.HasPrefix(line, "branch ") {
			branchPath := strings.TrimPrefix(line, "branch ")
			// Extract branch name from refs/heads/branch-name
			branchName := strings.TrimPrefix(branchPath, "refs/heads/")
			if currentWorktree != "" {
				worktreeBranches[currentWorktree] = branchName
			}
		}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Skip worktrees that don't belong to this repo. Dir names are
			// globally unique (nanotime-suffixed), so a base-name match is a
			// safe ownership check — without it we would force-delete another
			// repo's worktree.
			if !ownedBaseNames[entry.Name()] {
				continue
			}
			worktreePath := filepath.Join(worktreesDir, entry.Name())

			// Remove the worktree via git so that its internal registration is
			// cleaned up — do NOT use os.RemoveAll directly, as that leaves a
			// stale entry in .git/worktrees/ requiring a manual `git worktree prune`.
			removeCmd := exec.Command("git", "-C", repoPath, "worktree", "remove", "-f", worktreePath)
			if removeOut, removeErr := removeCmd.CombinedOutput(); removeErr != nil {
				log.ErrorLog.Printf("failed to remove worktree %s: %v (%s)", worktreePath, removeErr, strings.TrimSpace(string(removeOut)))
				// Fallback: remove the directory directly so the on-disk state is
				// cleaned up even if git disagrees.
				if err := os.RemoveAll(worktreePath); err != nil {
					log.ErrorLog.Printf("failed to remove worktree directory %s: %v", worktreePath, err)
				}
			}

			// Delete the branch associated with this worktree if found
			for path, branch := range worktreeBranches {
				if filepath.Base(path) == entry.Name() {
					// Delete the branch
					deleteCmd := exec.Command("git", "-C", repoPath, "branch", "-D", branch)
					if deleteOut, deleteErr := deleteCmd.CombinedOutput(); deleteErr != nil {
						// Log the error but continue with other worktrees
						log.ErrorLog.Printf("failed to delete branch %s: %v (%s)", branch, deleteErr, strings.TrimSpace(string(deleteOut)))
					}
					break
				}
			}
		}
	}

	// You have to prune the cleaned up worktrees.
	cmd = exec.Command("git", "-C", repoPath, "worktree", "prune")
	_, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}

	return nil
}
