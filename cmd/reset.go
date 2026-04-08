package cmd

import (
	"fmt"
	"maestro/pkg/accounts"
	"os"
	"path/filepath"
	"strings"
)

// ResetDeps bundles the dependencies that RunReset needs but cannot import
// directly due to import-cycle constraints (session, session/tmux, daemon all
// eventually import this package).
type ResetDeps struct {
	// LoadInstanceCount returns the number of stored instances, or -1 on error.
	LoadInstanceCount func() int
	// GetRepoPaths returns repo paths from stored instances.
	GetRepoPaths func() []string
	// DeleteAllInstances wipes the instance storage.
	DeleteAllInstances func() error
	// CleanupSessions tears down tmux sessions.
	CleanupSessions func() error
	// CleanupWorktrees removes git worktrees for a given repo path.
	CleanupWorktrees func(repoPath string) error
	// StopDaemon stops the running daemon process.
	StopDaemon func() error
}

// RunReset destroys all stored instances, tasks, tmux sessions, worktrees,
// and stops the daemon. It prompts the user for confirmation before proceeding.
func RunReset(deps ResetDeps) error {
	// Count what will be destroyed before prompting.
	instanceCount := "unknown"
	if n := deps.LoadInstanceCount(); n >= 0 {
		instanceCount = fmt.Sprintf("%d", n)
	}

	taskCount := "unknown"
	conductorBase, baseDirErr := accounts.ConductorDir()
	if baseDirErr == nil {
		matches, _ := filepath.Glob(filepath.Join(conductorBase, "tasks", "*.json"))
		taskCount = fmt.Sprintf("%d", len(matches))
	}

	registryStatus := "not found"
	if baseDirErr == nil {
		if _, statErr := os.Stat(filepath.Join(conductorBase, "registry.json")); statErr == nil {
			registryStatus = "exists"
		}
	}

	fmt.Printf("This will delete:\n")
	fmt.Printf("  %s instance(s)\n", instanceCount)
	fmt.Printf("  %s task file(s), results, and status files\n", taskCount)
	fmt.Printf("  Registry (%s), session state, and captures\n", registryStatus)
	fmt.Printf("\nContinue? [y/N]: ")

	var answer string
	fmt.Scanln(&answer)
	answer = strings.TrimSpace(answer)
	if answer != "y" && answer != "Y" {
		fmt.Println("Cancelled.")
		return nil
	}

	// Collect repo paths from stored instances before wiping storage,
	// so CleanupWorktrees works even when called from outside the repo.
	repoPaths := deps.GetRepoPaths()

	if err := deps.DeleteAllInstances(); err != nil {
		return fmt.Errorf("failed to reset storage: %w", err)
	}
	fmt.Println("Storage has been reset successfully")

	// Delete orchestration state: tasks, results, status files, and registry.
	// os.RemoveAll is a no-op on non-existent paths, so this is safe on first run.
	if baseDirErr == nil {
		for _, subdir := range []string{"tasks", "results", "status"} {
			_ = os.RemoveAll(filepath.Join(conductorBase, subdir))
		}
		_ = os.Remove(filepath.Join(conductorBase, "registry.json"))
	}
	fmt.Println("Task files and registry have been cleared")

	if err := deps.CleanupSessions(); err != nil {
		return fmt.Errorf("failed to cleanup tmux sessions: %w", err)
	}
	fmt.Println("Tmux sessions have been cleaned up")

	if len(repoPaths) == 0 {
		// Fall back to the current directory if no stored instances were found.
		currentDir, err := filepath.Abs(".")
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		repoPaths = []string{currentDir}
	}

	for _, repoPath := range repoPaths {
		if err := deps.CleanupWorktrees(repoPath); err != nil {
			// Log but continue cleaning up remaining repos.
			fmt.Fprintf(os.Stderr, "warning: failed to cleanup worktrees for %s: %v\n", repoPath, err)
		}
	}
	fmt.Println("Worktrees have been cleaned up")

	// Kill any daemon that's running.
	if err := deps.StopDaemon(); err != nil {
		return err
	}
	fmt.Println("daemon has been stopped")

	return nil
}
