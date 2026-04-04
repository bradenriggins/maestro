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
			fmt.Printf("Dispatched task %s to %s\n", result.TaskID, result.InstanceName)
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
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
	}
}
