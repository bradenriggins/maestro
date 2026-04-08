package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"maestro/app"
	cmd2 "maestro/cmd"
	"maestro/config"
	"maestro/daemon"
	"maestro/log"
	"maestro/pkg/accounts"
	"maestro/pkg/orchestration"
	"maestro/session"
	"maestro/session/git"
	"maestro/session/tmux"
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
	scheduleFlag     bool
	durationFlag     int
	maxWaitFlag      int
	rootCmd          = &cobra.Command{
		Use:   "maestro",
		Short: "Maestro - Multi-account Claude Code and Codex orchestration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			defer log.Close()

			if daemonFlag {
				cfg := config.LoadConfig()
				err := daemon.RunDaemon(cfg)
				if err != nil {
					log.ErrorLog.Printf("failed to start daemon: %v", err)
				}
				return err
			}

			// Check if we're in a git repository
			currentDir, err := filepath.Abs(".")
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}

			if !git.IsGitRepo(currentDir) {
				return fmt.Errorf("error: maestro must be run from within a git repository")
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
		RunE: func(c *cobra.Command, args []string) error {
			defer log.Close()

			state := config.LoadState()
			storage, err := session.NewStorage(state)
			if err != nil {
				return fmt.Errorf("failed to initialize storage: %w", err)
			}

			return cmd2.RunReset(cmd2.ResetDeps{
				LoadInstanceCount: func() int {
					if instances, e := storage.LoadInstances(); e == nil {
						return len(instances)
					}
					return -1
				},
				GetRepoPaths:       storage.GetRepoPaths,
				DeleteAllInstances: storage.DeleteAllInstances,
				CleanupSessions: func() error {
					return tmux.CleanupSessions(cmd2.MakeExecutor())
				},
				CleanupWorktrees: git.CleanupWorktrees,
				StopDaemon:       daemon.StopDaemon,
			})
		},
	}

	debugCmd = &cobra.Command{
		Use:   "debug",
		Short: "Print debug information like config paths",
		RunE: func(cmd *cobra.Command, args []string) error {
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
		Short: "Print the version number of maestro",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("maestro version %s\n", version)
		},
	}

	setupCmd = &cobra.Command{
		Use:   "setup",
		Short: "Configure accounts for multi-session orchestration",
		RunE: func(cmd *cobra.Command, args []string) error {
			programBin, _ := cmd.Flags().GetString("program-bin")
			if programBin == "" {
				programBin, _ = cmd.Flags().GetString("claude-bin")
			}
			configFileFlag, _ := cmd.Flags().GetString("config-file")
			if err := accounts.RunSetup(programBin, configFileFlag); err != nil {
				return fmt.Errorf("setup failed: %w", err)
			}
			return nil
		},
	}

	addAccountCmd = &cobra.Command{
		Use:   "add-account",
		Short: "Add a new account to an existing configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			programBin, _ := cmd.Flags().GetString("program-bin")
			if programBin == "" {
				programBin, _ = cmd.Flags().GetString("claude-bin")
			}
			return accounts.RunAddAccount(programBin)
		},
	}

	dispatchCmd = &cobra.Command{
		Use:   "dispatch <instance-name> [task-prompt]",
		Short: "Dispatch a task to a worker instance",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			instanceName := args[0]
			var taskPrompt string
			if len(args) > 1 {
				taskPrompt = args[1]
			}
			afterDeps, _ := c.Flags().GetStringSlice("after")
			return cmd2.RunDispatchCmd(instanceName, taskPrompt, taskFlag, afterDeps, scheduleFlag, durationFlag, maxWaitFlag)
		},
	}

	pipelineCmd = &cobra.Command{
		Use:   "pipeline <file.yaml>",
		Short: "Execute a task pipeline from a YAML definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return cmd2.RunPipeline(args[0])
		},
	}

	statusCmd = &cobra.Command{
		Use:   "status [instance-name]",
		Short: "Show worker and task status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := loadConductorConfig(); err != nil {
				return err
			}
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
			if _, err := loadConductorConfig(); err != nil {
				return err
			}
			return orchestration.RunWorkers()
		},
	}

	tasksCmd = &cobra.Command{
		Use:   "tasks",
		Short: "List all tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := loadConductorConfig(); err != nil {
				return err
			}
			return orchestration.RunTasks(statusFilterFlag)
		},
	}

	outputCmd = &cobra.Command{
		Use:   "output <instance-name>",
		Short: "Capture recent terminal output from a worker",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := loadConductorConfig(); err != nil {
				return err
			}
			return orchestration.RunOutput(args[0], linesFlag)
		},
	}

	recallCmd = &cobra.Command{
		Use:   "recall <instance-name>",
		Short: "Capture full terminal history from a worker and save to file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := loadConductorConfig(); err != nil {
				return err
			}
			return orchestration.RunRecall(args[0])
		},
	}

	usageCmd = &cobra.Command{
		Use:   "usage",
		Short: "Show usage levels for all configured accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConductorConfig()
			if err != nil {
				return err
			}
			report, err := orchestration.CollectUsage(cfg)
			if err != nil {
				return err
			}
			orchestration.SaveUsageReport(report)
			fmt.Println("Account Usage (5-hour rolling window):")
			fmt.Print(orchestration.FormatUsageSummary(report))
			advice := orchestration.GetRoutingAdvice(report)
			fmt.Println(orchestration.FormatRoutingAdvice(advice))
			return nil
		},
	}

	doctorCmd = &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose and fix state issues",
		RunE: func(c *cobra.Command, args []string) error {
			return cmd2.RunDoctor()
		},
	}

	retryFailedCmd = &cobra.Command{
		Use:   "retry-failed",
		Short: "Retry all failed and timed-out tasks across idle workers",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := loadConductorConfig(); err != nil {
				return err
			}

			maxTasks, _ := cmd.Flags().GetInt("max")
			workerName, _ := cmd.Flags().GetString("worker")

			result, err := orchestration.RunBulkRetry(orchestration.BulkRetryOptions{
				MaxTasks:   maxTasks,
				WorkerName: workerName,
			})
			if err != nil {
				return err
			}

			if result.Total == 0 {
				fmt.Println("No failed or timed-out tasks to retry.")
				return nil
			}

			fmt.Printf("Retried %d/%d failed tasks", result.Retried, result.Total)
			if len(result.WorkersUsed) > 0 {
				fmt.Printf(" across %d workers", len(result.WorkersUsed))
			}
			fmt.Println(".")
			if result.Skipped > 0 {
				fmt.Printf("  %d skipped (max attempts reached).\n", result.Skipped)
			}
			if result.Failed > 0 {
				fmt.Printf("  %d failed to dispatch:\n", result.Failed)
				for _, e := range result.Errors {
					fmt.Printf("    - %s\n", e)
				}
			}
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
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := log.Initialize(daemonFlag); err != nil {
			return fmt.Errorf("failed to initialize logger: %w", err)
		}
		return nil
	}

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
	dispatchCmd.Flags().BoolVar(&scheduleFlag, "schedule", false, "Enable throughput-optimal scheduling (waits for rate-limit resets if beneficial)")
	dispatchCmd.Flags().IntVar(&durationFlag, "duration", 0, "Estimated task duration in minutes (used by scheduler)")
	dispatchCmd.Flags().IntVar(&maxWaitFlag, "max-wait", 120, "Maximum wait time in minutes for a worker reset (used by scheduler)")
	dispatchCmd.Flags().StringSlice("after", nil, "Task IDs that must complete before this task runs")
	tasksCmd.Flags().StringVar(&statusFilterFlag, "status", "", "Filter tasks by status (e.g. dispatched, in_progress, completed, failed)")
	outputCmd.Flags().IntVar(&linesFlag, "lines", 50, "Number of lines to capture (max 500)")

	setupCmd.Flags().String("program-bin", "", "path to the AI program binary (claude, codex)")
	setupCmd.Flags().String("claude-bin", "", "deprecated: use --program-bin")
	_ = setupCmd.Flags().MarkHidden("claude-bin")
	setupCmd.Flags().String("config-file", "", "path to JSON file pre-seeding account names and roles")

	addAccountCmd.Flags().String("program-bin", "", "path to the AI program binary (claude, codex)")
	addAccountCmd.Flags().String("claude-bin", "", "deprecated: use --program-bin")
	_ = addAccountCmd.Flags().MarkHidden("claude-bin")

	rootCmd.AddCommand(debugCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(resetCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(addAccountCmd)
	rootCmd.AddCommand(dispatchCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(workersCmd)
	rootCmd.AddCommand(tasksCmd)
	rootCmd.AddCommand(outputCmd)
	rootCmd.AddCommand(recallCmd)
	rootCmd.AddCommand(usageCmd)
	rootCmd.AddCommand(doctorCmd)
	retryFailedCmd.Flags().Int("max", 0, "maximum number of tasks to retry (0 = all)")
	retryFailedCmd.Flags().String("worker", "", "force all retries to a specific worker")
	rootCmd.AddCommand(retryFailedCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(pipelineCmd)
}

func loadConductorConfig() (*accounts.ConductorConfig, error) {
	cfg, err := accounts.LoadConductorConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load conductor config: %w", err)
	}
	if cfg == nil {
		return nil, fmt.Errorf("not configured; run 'maestro setup' first")
	}
	return cfg, nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
