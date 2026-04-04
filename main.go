package main

import (
	"claude-conductor/app"
	cmd2 "claude-conductor/cmd"
	"claude-conductor/config"
	"claude-conductor/daemon"
	"claude-conductor/log"
	"claude-conductor/pkg/accounts"
	"claude-conductor/pkg/orchestration"
	"claude-conductor/session"
	"claude-conductor/session/git"
	"claude-conductor/session/tmux"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	version          = "0.1.0"
	programFlag      string
	autoYesFlag      bool
	daemonFlag       bool
	freshFlag        bool
	noSafetyNetFlag  bool
	taskFlag         string
	statusFilterFlag string
	linesFlag        int
	rootCmd     = &cobra.Command{
		Use:   "claude-conductor",
		Short: "Claude Conductor - Multi-account Claude Code orchestration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			log.Initialize(daemonFlag)
			defer log.Close()

			if daemonFlag {
				cfg := config.LoadConfig()
				err := daemon.RunDaemon(cfg)
				log.ErrorLog.Printf("failed to start daemon %v", err)
				return err
			}

			// Check if we're in a git repository
			currentDir, err := filepath.Abs(".")
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}

			if !git.IsGitRepo(currentDir) {
				return fmt.Errorf("error: claude-conductor must be run from within a git repository")
			}

			cfg := config.LoadConfig()

			// Program flag overrides config
			program := cfg.GetProgram()
			if programFlag != "" {
				program = programFlag
			}
			// AutoYes flag overrides config
			autoYes := cfg.AutoYes
			if autoYesFlag {
				autoYes = true
			}
			if autoYes {
				defer func() {
					if err := daemon.LaunchDaemon(); err != nil {
						log.ErrorLog.Printf("failed to launch daemon: %v", err)
					}
				}()
			}
			// Kill any daemon that's running.
			if err := daemon.StopDaemon(); err != nil {
				log.ErrorLog.Printf("failed to stop daemon: %v", err)
			}

			return app.Run(ctx, program, autoYes, freshFlag, noSafetyNetFlag)
		},
	}

	resetCmd = &cobra.Command{
		Use:   "reset",
		Short: "Reset all stored instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Initialize(false)
			defer log.Close()

			state := config.LoadState()
			storage, err := session.NewStorage(state)
			if err != nil {
				return fmt.Errorf("failed to initialize storage: %w", err)
			}
			if err := storage.DeleteAllInstances(); err != nil {
				return fmt.Errorf("failed to reset storage: %w", err)
			}
			fmt.Println("Storage has been reset successfully")

			if err := tmux.CleanupSessions(cmd2.MakeExecutor()); err != nil {
				return fmt.Errorf("failed to cleanup tmux sessions: %w", err)
			}
			fmt.Println("Tmux sessions have been cleaned up")

			if err := git.CleanupWorktrees(); err != nil {
				return fmt.Errorf("failed to cleanup worktrees: %w", err)
			}
			fmt.Println("Worktrees have been cleaned up")

			// Kill any daemon that's running.
			if err := daemon.StopDaemon(); err != nil {
				return err
			}
			fmt.Println("daemon has been stopped")

			return nil
		},
	}

	debugCmd = &cobra.Command{
		Use:   "debug",
		Short: "Print debug information like config paths",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Initialize(false)
			defer log.Close()

			cfg := config.LoadConfig()

			configDir, err := config.GetConfigDir()
			if err != nil {
				return fmt.Errorf("failed to get config directory: %w", err)
			}
			configJson, _ := json.MarshalIndent(cfg, "", "  ")

			fmt.Printf("Config: %s\n%s\n", filepath.Join(configDir, config.ConfigFileName), configJson)

			return nil
		},
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number of claude-conductor",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("claude-conductor version %s\n", version)
		},
	}

	setupCmd = &cobra.Command{
		Use:   "setup",
		Short: "Configure Anthropic accounts for multi-session orchestration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := accounts.RunSetup(); err != nil {
				return fmt.Errorf("setup failed: %w", err)
			}
			return nil
		},
	}

	dispatchCmd = &cobra.Command{
		Use:   "dispatch <instance-name> [task-prompt]",
		Short: "Dispatch a task to a worker instance",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceName := args[0]
			var taskPrompt string
			if len(args) > 1 {
				taskPrompt = args[1]
			}

			result, err := orchestration.RunDispatch(instanceName, taskPrompt, taskFlag)
			if err != nil {
				if de, ok := err.(*orchestration.DispatchError); ok {
					fmt.Fprintf(os.Stderr, "Error: %s\n", de.Msg)
					os.Exit(de.Code)
				}
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(result.TaskID)
			return nil
		},
	}

	statusCmd = &cobra.Command{
		Use:   "status [instance-name]",
		Short: "Show worker and task status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var instanceFilter string
			if len(args) > 0 {
				instanceFilter = args[0]
			}
			return orchestration.RunStatus(instanceFilter)
		},
	}

	workersCmd = &cobra.Command{
		Use:   "workers",
		Short: "List all registered workers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return orchestration.RunWorkers()
		},
	}

	tasksCmd = &cobra.Command{
		Use:   "tasks",
		Short: "List all tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return orchestration.RunTasks(statusFilterFlag)
		},
	}

	outputCmd = &cobra.Command{
		Use:   "output <instance-name>",
		Short: "Capture recent terminal output from a worker",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return orchestration.RunOutput(args[0], linesFlag)
		},
	}

	recallCmd = &cobra.Command{
		Use:   "recall <instance-name>",
		Short: "Capture full terminal history from a worker and save to file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return orchestration.RunRecall(args[0])
		},
	}

	usageCmd = &cobra.Command{
		Use:   "usage",
		Short: "Show usage levels for all configured accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := accounts.LoadConductorConfig()
			if err != nil || cfg == nil {
				return fmt.Errorf("no conductor config found — run setup first")
			}
			report, err := orchestration.CollectUsage(cfg)
			if err != nil {
				return err
			}
			orchestration.SaveUsageReport(report)
			fmt.Println("Account Usage (5-hour rolling window):")
			fmt.Print(orchestration.FormatUsageSummary(report))
			return nil
		},
	}

	doctorCmd = &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose and fix state issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Running diagnostics...")

			// Check config
			cfg, err := accounts.LoadConductorConfig()
			if err != nil {
				fmt.Printf("  ✗ Config: %v\n", err)
			} else if cfg == nil {
				fmt.Printf("  ✗ Config: not found (run setup first)\n")
			} else {
				fmt.Printf("  ✓ Config: %d accounts configured\n", len(cfg.Accounts))
			}

			// Check account directories
			if cfg != nil {
				for _, acct := range cfg.Accounts {
					if _, err := os.Stat(acct.ConfigDir); err != nil {
						fmt.Printf("  ✗ Account %s: config dir missing (%s)\n", acct.Name, acct.ConfigDir)
					} else {
						fmt.Printf("  ✓ Account %s: config dir exists\n", acct.Name)
					}
				}
			}

			// Check registry
			reg, err := orchestration.LoadRegistry()
			if err != nil {
				fmt.Printf("  ✗ Registry: %v\n", err)
			} else {
				alive := 0
				dead := 0
				for _, entry := range reg.Instances {
					if orchestration.TmuxHasSession(entry.TmuxSession) {
						alive++
					} else if entry.Status == orchestration.RegistryStatusRunning {
						dead++
					}
				}
				fmt.Printf("  ✓ Registry: %d instances (%d alive, %d dead)\n", len(reg.Instances), alive, dead)
			}

			// Check task files
			store, err := orchestration.NewTaskStore()
			if err != nil {
				fmt.Printf("  ✗ Task store: %v\n", err)
			} else {
				tasks, _ := store.List("")
				stale := 0
				for _, t := range tasks {
					if t.Status == orchestration.StatusStale {
						stale++
					}
				}
				if stale > 0 {
					fmt.Printf("  ✗ Tasks: %d total, %d stale\n", len(tasks), stale)
				} else {
					fmt.Printf("  ✓ Tasks: %d total\n", len(tasks))
				}
			}

			// Check permissions
			base, _ := accounts.ConductorDir()
			if info, err := os.Stat(base); err == nil {
				if info.Mode().Perm() != 0700 {
					fmt.Printf("  ✗ Permissions: %s is %o (should be 0700)\n", base, info.Mode().Perm())
				} else {
					fmt.Printf("  ✓ Permissions: correct\n")
				}
			}

			fmt.Println("\nDone.")
			return nil
		},
	}

	cleanCmd = &cobra.Command{
		Use:   "clean",
		Short: "Clean up old resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			base, err := accounts.ConductorDir()
			if err != nil {
				return err
			}

			store, err := orchestration.NewTaskStore()
			if err != nil {
				return err
			}

			// Find old completed/failed tasks
			tasks, _ := store.List("")
			oldTasks := 0
			for _, t := range tasks {
				if t.IsTerminal() {
					oldTasks++
				}
			}

			// Find capture files
			capturesDir := filepath.Join(base, "captures")
			captures, _ := os.ReadDir(capturesDir)

			fmt.Printf("Found:\n")
			fmt.Printf("  Terminal-state tasks: %d\n", oldTasks)
			fmt.Printf("  Capture files: %d\n", len(captures))

			if oldTasks == 0 && len(captures) == 0 {
				fmt.Println("Nothing to clean.")
				return nil
			}

			fmt.Print("\nClean up? [y/N]: ")
			var answer string
			fmt.Scanln(&answer)
			if strings.ToLower(strings.TrimSpace(answer)) != "y" {
				fmt.Println("Cancelled.")
				return nil
			}

			// Archive old tasks
			archiveDir := filepath.Join(base, "archive")
			os.MkdirAll(archiveDir, 0700)
			archived := 0
			for _, t := range tasks {
				if t.IsTerminal() {
					// Move task JSON and prompt file to archive
					taskFile := filepath.Join(base, "tasks", t.ID+".json")
					promptFile := filepath.Join(base, "tasks", t.ID+".prompt")
					resultFile := filepath.Join(base, "results", t.ID+".md")

					for _, src := range []string{taskFile, promptFile, resultFile} {
						if _, err := os.Stat(src); err == nil {
							dst := filepath.Join(archiveDir, filepath.Base(src))
							os.Rename(src, dst)
						}
					}
					archived++
				}
			}

			// Remove capture files
			removed := 0
			for _, entry := range captures {
				os.Remove(filepath.Join(capturesDir, entry.Name()))
				removed++
			}

			fmt.Printf("Archived %d tasks, removed %d capture files.\n", archived, removed)
			return nil
		},
	}
)

func init() {
	rootCmd.Flags().StringVarP(&programFlag, "program", "p", "",
		"Program to run in new instances (e.g. 'aider --model ollama_chat/gemma3:1b')")
	rootCmd.Flags().BoolVarP(&autoYesFlag, "autoyes", "y", false,
		"[experimental] If enabled, all instances will automatically accept prompts")
	rootCmd.Flags().BoolVar(&daemonFlag, "daemon", false, "Run a program that loads all sessions"+
		" and runs autoyes mode on them.")
	rootCmd.Flags().BoolVar(&freshFlag, "fresh", false, "Start fresh, don't resume previous session")
	rootCmd.Flags().BoolVar(&noSafetyNetFlag, "no-safety-net", false, "Skip git stash and session start tag")

	// Hide the daemonFlag as it's only for internal use
	err := rootCmd.Flags().MarkHidden("daemon")
	if err != nil {
		panic(err)
	}

	dispatchCmd.Flags().StringVar(&taskFlag, "task", "", "Re-dispatch an existing task by ID")
	tasksCmd.Flags().StringVar(&statusFilterFlag, "status", "", "Filter tasks by status (e.g. dispatched, in_progress, completed, failed)")
	outputCmd.Flags().IntVar(&linesFlag, "lines", 50, "Number of lines to capture (max 500)")

	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(resetCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(dispatchCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(workersCmd)
	rootCmd.AddCommand(tasksCmd)
	rootCmd.AddCommand(outputCmd)
	rootCmd.AddCommand(recallCmd)
	rootCmd.AddCommand(usageCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(cleanCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
	}
}
