package accounts

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type AuthStatus struct {
	LoggedIn         bool   `json:"loggedIn"`
	AuthMethod       string `json:"authMethod"`
	Email            string `json:"email"`
	SubscriptionType string `json:"subscriptionType"`
}

func RunSetup() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("┌─ Claude Conductor Setup ─────────────────────────────┐")
	fmt.Println("│                                                       │")
	fmt.Println("│  IMPORTANT: Ensure your use of multiple accounts      │")
	fmt.Println("│  complies with Anthropic's Terms of Service.          │")
	fmt.Println("└───────────────────────────────────────────────────────┘")
	fmt.Println()

	fmt.Print("How many accounts do you want to configure? [2-10]: ")
	countStr, _ := reader.ReadString('\n')
	countStr = strings.TrimSpace(countStr)
	var count int
	if _, err := fmt.Sscanf(countStr, "%d", &count); err != nil || count < 1 || count > 10 {
		return fmt.Errorf("invalid account count: %q (must be 1-10)", countStr)
	}

	cfg := DefaultConfig()
	cfg.Accounts = make([]Account, 0, count)
	orchestratorSet := false
	configuredEmails := make(map[string]string)

	for i := 0; i < count; i++ {
		fmt.Printf("\n── Account %d of %d ──\n", i+1, count)

		defaultName := fmt.Sprintf("worker-%d", i)
		if i == 0 {
			defaultName = "main"
		}
		fmt.Printf("  Name [%s]: ", defaultName)
		nameStr, _ := reader.ReadString('\n')
		nameStr = strings.TrimSpace(nameStr)
		if nameStr == "" {
			nameStr = defaultName
		}

		if !validNameRegex.MatchString(nameStr) {
			return fmt.Errorf("invalid name %q — must match /^[a-z0-9][a-z0-9-]*$/", nameStr)
		}

		var role Role
		if !orchestratorSet {
			fmt.Print("  Role [orchestrator/worker]: ")
			roleStr, _ := reader.ReadString('\n')
			roleStr = strings.TrimSpace(roleStr)
			if roleStr == "" || roleStr == "orchestrator" {
				role = RoleOrchestrator
				orchestratorSet = true
			} else {
				role = RoleWorker
			}
		} else {
			role = RoleWorker
			fmt.Printf("  Role: worker (orchestrator already set)\n")
		}

		configDir, err := AccountConfigDir(nameStr)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(configDir, 0700); err != nil {
			return fmt.Errorf("failed to create account dir: %w", err)
		}

		fmt.Println()
		fmt.Println("  TIP: Use a private/incognito browser window to avoid")
		fmt.Println("  session carryover between accounts.")
		fmt.Println()
		fmt.Printf("  Press Enter to open login for %q...", nameStr)
		reader.ReadString('\n')

		loginCmd := exec.Command("claude", "auth", "login")
		loginCmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+configDir)
		loginCmd.Stdin = os.Stdin
		loginCmd.Stdout = os.Stdout
		loginCmd.Stderr = os.Stderr
		if err := loginCmd.Run(); err != nil {
			return fmt.Errorf("login failed for %q: %w", nameStr, err)
		}

		statusCmd := exec.Command("claude", "auth", "status", "--json")
		statusCmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+configDir)
		output, err := statusCmd.Output()
		if err != nil {
			return fmt.Errorf("failed to verify login for %q: %w", nameStr, err)
		}

		var status AuthStatus
		if err := json.Unmarshal(output, &status); err != nil {
			return fmt.Errorf("failed to parse auth status for %q: %w", nameStr, err)
		}

		if !status.LoggedIn {
			return fmt.Errorf("login verification failed for %q — not logged in", nameStr)
		}

		fmt.Printf("  ✓ Logged in as: %s\n", status.Email)
		fmt.Printf("    Subscription: %s\n", status.SubscriptionType)

		if prevAccount, exists := configuredEmails[status.Email]; exists {
			fmt.Printf("\n  WARNING: This email was already used for account %q.\n", prevAccount)
			fmt.Print("  Continue anyway? [y/N]: ")
			confirm, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(confirm)) != "y" {
				return fmt.Errorf("setup cancelled — duplicate email")
			}
		}
		configuredEmails[status.Email] = nameStr

		fmt.Print("  Is this correct? [Y/n]: ")
		confirm, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(confirm)) == "n" {
			return fmt.Errorf("setup cancelled by user")
		}

		cfg.Accounts = append(cfg.Accounts, Account{
			Name:      nameStr,
			Role:      role,
			ConfigDir: configDir,
			Email:     status.Email,
			Verified:  true,
		})

		// Configure statusLine for real-time usage tracking
		scriptPath, slErr := generateStatusLineScriptFile(nameStr)
		if slErr != nil {
			fmt.Printf("  Warning: could not set up usage tracking: %v\n", slErr)
		} else {
			if slErr := configureStatusLine(configDir, scriptPath); slErr != nil {
				fmt.Printf("  Warning: could not configure statusLine: %v\n", slErr)
			} else {
				fmt.Println("  \u2713 Usage tracking configured (statusLine)")
			}
		}
	}

	if err := SaveConductorConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Ensure correct permissions on base dir even if it pre-existed
	base, err := ConductorDir()
	if err != nil {
		return err
	}
	os.Chmod(base, 0700)

	// Create required directories
	for _, dir := range []string{"tasks", "status", "results", "logs", "captures", "history", "archive", "bin", "usage"} {
		if err := os.MkdirAll(filepath.Join(base, dir), 0700); err != nil {
			return fmt.Errorf("failed to create %s dir: %w", dir, err)
		}
	}

	// Create .gitignore
	gitignorePath := filepath.Join(base, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("*\n"), 0600); err != nil {
		return fmt.Errorf("failed to create .gitignore: %w", err)
	}

	// Generate shell script wrappers
	binDir := filepath.Join(base, "bin")
	wrappers := map[string]string{
		"dispatch.sh": "dispatch",
		"status.sh":   "status",
		"workers.sh":  "workers",
		"tasks.sh":    "tasks",
		"output.sh":   "output",
		"recall.sh":   "recall",
	}
	for filename, subcmd := range wrappers {
		content := fmt.Sprintf("#!/bin/bash\nexec claude-conductor %s \"$@\"\n", subcmd)
		path := filepath.Join(binDir, filename)
		if err := os.WriteFile(path, []byte(content), 0700); err != nil {
			return fmt.Errorf("failed to write %s: %w", filename, err)
		}
	}

	// Create empty registry.json
	registryPath := filepath.Join(base, "registry.json")
	if err := os.WriteFile(registryPath, []byte("{\"instances\":{},\"updated_at\":\"\"}\n"), 0600); err != nil {
		return fmt.Errorf("failed to create registry.json: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ Setup complete!")
	fmt.Printf("  Accounts: ")
	for i, acct := range cfg.Accounts {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("%s (%s)", acct.Name, acct.Role)
	}
	fmt.Println()
	fmt.Printf("  Config: %s\n", filepath.Join(base, ConfigFileName))
	fmt.Println("  Run `claude-conductor` in any git repo to start.")

	return nil
}
