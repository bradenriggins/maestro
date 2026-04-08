package accounts

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"maestro/pkg/programs"
)

// emailNotAvailable is the sentinel value returned by verifyAuth when the
// program's auth status output does not include an email address.
// Callers must not treat this as a real email for duplicate-detection purposes.
const emailNotAvailable = "authenticated (email not available)"

type AuthStatus struct {
	LoggedIn         bool   `json:"loggedIn"`
	AuthMethod       string `json:"authMethod"`
	Email            string `json:"email"`
	SubscriptionType string `json:"subscriptionType"`
}

// authVerifyTimeout is the maximum time allowed for an auth-status command.
// A hanging auth command (e.g. waiting for network or keychain) would otherwise
// block setup indefinitely.
const authVerifyTimeout = 30 * time.Second

// verifyAuth runs the auth status command for the given program and returns
// the logged-in email address. For Claude it parses JSON output; for other
// programs it checks the exit code and attempts to extract an email from text.
// The command is bounded by authVerifyTimeout to prevent indefinite hangs.
func verifyAuth(programBin string, spec programs.ProgramSpec, configDir string) (email string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), authVerifyTimeout)
	defer cancel()

	args := strings.Fields(spec.AuthStatusCommand)
	statusCmd := exec.CommandContext(ctx, programBin, args...)
	statusCmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s", spec.ConfigDirEnvVar, configDir))

	if spec.AuthProvider == "anthropic" {
		// Claude: parse JSON output; capture stderr for diagnostics on failure.
		var stderr bytes.Buffer
		statusCmd.Stderr = &stderr
		output, statusErr := statusCmd.Output()
		if statusErr != nil {
			if ctx.Err() != nil {
				return "", fmt.Errorf("auth status command timed out after %s", authVerifyTimeout)
			}
			if stderr.Len() > 0 {
				return "", fmt.Errorf("auth status command failed: %w\n  stderr: %s", statusErr, strings.TrimSpace(stderr.String()))
			}
			return "", fmt.Errorf("auth status command failed: %w", statusErr)
		}
		var status AuthStatus
		if jsonErr := json.Unmarshal(output, &status); jsonErr != nil {
			return "", fmt.Errorf("failed to parse auth status JSON: %w", jsonErr)
		}
		if !status.LoggedIn {
			return "", fmt.Errorf("not logged in")
		}
		return status.Email, nil
	}

	// Non-anthropic providers (e.g. codex): check exit code, parse text for email.
	var stderr bytes.Buffer
	statusCmd.Stderr = &stderr
	output, statusErr := statusCmd.Output()
	if statusErr != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("auth status command timed out after %s", authVerifyTimeout)
		}
		if stderr.Len() > 0 {
			return "", fmt.Errorf("auth status command failed: %w\n  stderr: %s", statusErr, strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("auth status command failed: %w", statusErr)
	}
	// Try to extract an email from the text output.
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "@") {
			// Heuristic: take the first token containing '@'.
			for _, token := range strings.Fields(trimmed) {
				if strings.Contains(token, "@") {
					// Strip leading/trailing punctuation that is not part of an email
					// (e.g. trailing comma, period, colon, angle brackets).
					token = strings.TrimLeft(token, "<\"'(")
					token = strings.TrimRight(token, ">\"',.;:)")
					if token != "" && strings.Contains(token, "@") {
						return token, nil
					}
				}
			}
		}
	}
	// Auth succeeded (exit code 0) but no email found; return sentinel.
	return emailNotAvailable, nil
}

func RunSetup(programBin string, configFile string) (err error) {
	programBin = strings.TrimSpace(programBin)
	configFile = strings.TrimSpace(configFile)
	var createdDirs []string
	cleanup := func() {
		for _, d := range createdDirs {
			os.RemoveAll(d)
		}
	}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()

	reader := bufio.NewReader(os.Stdin)

	if _, jqErr := exec.LookPath("jq"); jqErr != nil {
		return fmt.Errorf("jq is required for usage tracking and worker task updates.\nInstall it first:  brew install jq    (macOS)\n                   apt install jq     (Linux)")
	}

	// If a config file was provided, load and validate it. The claude_bin field
	// in the file overrides the default but is itself overridden by the flag.
	var setupCfg *SetupConfig
	if configFile != "" {
		sc, loadErr := loadSetupConfigFile(configFile)
		if loadErr != nil {
			return loadErr
		}
		// File's claude_bin wins only when the caller didn't specify a binary.
		if sc.ClaudeBin != "" && programBin == "" {
			programBin = sc.ClaudeBin
		}
		setupCfg = sc
	}

	if ConductorConfigExists() {
		fmt.Println("Warning: maestro is already configured.")
		fmt.Println("Re-running setup will overwrite your account configuration.")
		fmt.Println("Existing registry and running instances will not be affected.")
		fmt.Print("Continue? [y/N]: ")
		confirm, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(confirm)) != "y" {
			return nil
		}
		fmt.Println()
	}

	fmt.Println("┌─ Maestro Setup ───────────────────────────────────────┐")
	fmt.Println("│                                                       │")
	fmt.Println("│  IMPORTANT: Ensure your use of multiple accounts      │")
	fmt.Println("│  complies with the providers' Terms of Service.       │")
	fmt.Println("└───────────────────────────────────────────────────────┘")
	fmt.Println()

	fmt.Println("TIP: Have all your accounts ready. Use a separate private/incognito")
	fmt.Println("browser window for each account login to avoid session carryover.")
	fmt.Println()

	var count int
	var accountSpecs []AccountSpec

	if setupCfg != nil {
		// Config-file path: skip interactive account count + name/role prompts.
		accountSpecs = setupCfg.Accounts
		count = len(accountSpecs)
	} else {
		// Interactive path: prompt for account count.
		fmt.Print("How many accounts do you want to configure? [2-10]: ")
		countStr, _ := reader.ReadString('\n')
		countStr = strings.TrimSpace(countStr)
		if _, scanErr := fmt.Sscanf(countStr, "%d", &count); scanErr != nil || count < 2 || count > 10 {
			return fmt.Errorf("must be a number between 2 and 10 (for example, enter 3 for three accounts)")
		}

		fmt.Printf("You are about to configure %d accounts. Press Enter to continue, or Ctrl+C to abort.", count)
		reader.ReadString('\n')
		fmt.Println()
	}

	cfg := DefaultConfig()
	cfg.Accounts = make([]Account, 0, count)
	orchestratorSet := false
	configuredEmails := make(map[string]string)

	for i := 0; i < count; i++ {
		fmt.Printf("\n── Account %d of %d ──\n", i+1, count)

		var nameStr string
		var role Role

		if setupCfg != nil {
			// Config-file path: use pre-seeded name and role.
			spec := accountSpecs[i]
			nameStr = spec.Name
			switch strings.ToLower(spec.Role) {
			case "orchestrator":
				role = RoleOrchestrator
				orchestratorSet = true
			case "worker":
				role = RoleWorker
			default:
				return fmt.Errorf("config file error: account %q has invalid role %q (must be \"orchestrator\" or \"worker\")", nameStr, spec.Role)
			}
			fmt.Printf("  Account %d: %s (%s)\n", i+1, nameStr, role)
		} else {
			// Interactive path: prompt for name.
			defaultName := fmt.Sprintf("worker-%d", i)
			if i == 0 {
				defaultName = "main"
			}
			fmt.Printf("  Name [%s]: ", defaultName)
			nameStr, _ = reader.ReadString('\n')
			nameStr = strings.TrimSpace(nameStr)
			if nameStr == "" {
				nameStr = defaultName
			}
		}

		if !validNameRegex.MatchString(nameStr) {
			return fmt.Errorf("invalid name %q — must match /^[a-z0-9][a-z0-9-]*$/", nameStr)
		}

		for _, existing := range cfg.Accounts {
			if existing.Name == nameStr {
				return fmt.Errorf("account name %q already used — choose a different name", nameStr)
			}
		}
		// Also check that the config directory would be unique
		candidateDir, dirErr := AccountConfigDir(nameStr)
		if dirErr != nil {
			return dirErr
		}
		for _, existing := range cfg.Accounts {
			if existing.ConfigDir == candidateDir {
				return fmt.Errorf("account config directory %q already in use — choose a different name", candidateDir)
			}
		}

		// Program selection.
		var programStr string
		if setupCfg != nil {
			programStr = accountSpecs[i].Program
			if programStr == "" {
				programStr = "claude"
			}
			if !programs.ValidProgram(programStr) {
				return fmt.Errorf("config file error: account %q has invalid program %q (valid: %s)", nameStr, programStr, strings.Join(programs.Names(), ", "))
			}
			fmt.Printf("  Program: %s\n", programStr)
		} else {
			fmt.Printf("  Program [%s] (default: claude): ", strings.Join(programs.Names(), "/"))
			progInput, _ := reader.ReadString('\n')
			progInput = strings.TrimSpace(strings.ToLower(progInput))
			if progInput == "" {
				progInput = "claude"
			}
			if !programs.ValidProgram(progInput) {
				return fmt.Errorf("invalid program %q — valid programs: %s", progInput, strings.Join(programs.Names(), ", "))
			}
			programStr = progInput
		}
		spec, _ := programs.Get(programStr)

		// Resolve the binary to use for this account.
		acctBin := programBin
		if acctBin == "" {
			acctBin = spec.Binary
		}

		if setupCfg == nil {
			if !orchestratorSet {
			rolePrompt:
				var roleDefault string
				if i == 0 {
					roleDefault = "default: orchestrator"
				} else {
					roleDefault = "default: worker"
				}
				fmt.Printf("  Role [orchestrator/worker] (%s): ", roleDefault)
				roleStr, _ := reader.ReadString('\n')
				roleStr = strings.TrimSpace(roleStr)
				if i == 0 {
					// First account: empty input defaults to orchestrator.
					if roleStr == "" || roleStr == "orchestrator" {
						role = RoleOrchestrator
						orchestratorSet = true
					} else {
						role = RoleWorker
						// If this is the last account and still no orchestrator, force a re-prompt.
						if i == count-1 {
							fmt.Println("  Warning: at least one account must be designated as orchestrator.")
							fmt.Println("  You must change this account to orchestrator, or go back and reconfigure.")
							goto rolePrompt
						}
					}
				} else {
					// Subsequent accounts before orchestrator is set: empty input defaults to worker.
					if roleStr == "orchestrator" {
						role = RoleOrchestrator
						orchestratorSet = true
					} else {
						role = RoleWorker
						// If this is the last account and still no orchestrator, force a re-prompt.
						if i == count-1 {
							fmt.Println("  Warning: at least one account must be designated as orchestrator.")
							fmt.Println("  You must change this account to orchestrator, or go back and reconfigure.")
							goto rolePrompt
						}
					}
				}
			} else {
				role = RoleWorker
				fmt.Printf("  Role: worker (orchestrator already set)\n")
			}
		}

		configDir := candidateDir
		if mkErr := os.MkdirAll(configDir, 0700); mkErr != nil {
			return fmt.Errorf("failed to create account dir: %w", mkErr)
		}
		createdDirs = append(createdDirs, configDir)

		// Auth login — branched by program.
		authArgs := strings.Fields(spec.AuthCommand)
		fmt.Printf("\n  Launching %s %s for %q...\n", acctBin, spec.AuthCommand, nameStr)

		loginCmd := exec.Command(acctBin, authArgs...)
		loginCmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s", spec.ConfigDirEnvVar, configDir))
		loginCmd.Stdin = os.Stdin
		loginCmd.Stdout = os.Stdout
		loginCmd.Stderr = os.Stderr
		if loginErr := loginCmd.Run(); loginErr != nil {
			return fmt.Errorf("login failed for %q: %w\n  Is %s installed? Check with: %s --version", nameStr, loginErr, spec.Binary, spec.Binary)
		}

		// Verify auth — branched by program.
		fmt.Println("  Verifying login...")
		email, verifyErr := verifyAuth(acctBin, spec, configDir)
		if verifyErr != nil {
			return fmt.Errorf("login verification failed for %q: %w\n  Try running manually: %s=%s %s %s", nameStr, verifyErr, spec.ConfigDirEnvVar, configDir, spec.Binary, spec.AuthCommand)
		}

		fmt.Printf("  ✓ Logged in as: %s\n", email)

		// Only warn about duplicate emails when we have a real email address.
		// The emailNotAvailable sentinel may appear for multiple Codex accounts
		// and must not trigger a false duplicate warning.
		if email != emailNotAvailable {
			if prevAccount, exists := configuredEmails[email]; exists {
				fmt.Printf("\n  WARNING: This email was already used for account %q.\n", prevAccount)
				fmt.Print("  Continue anyway? [y/N]: ")
				dupConfirm, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(dupConfirm)) != "y" {
					return fmt.Errorf("setup cancelled — duplicate email")
				}
			}
			configuredEmails[email] = nameStr
		}

		fmt.Print("  Is this correct? [Y/n]: ")
		confirm, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(confirm)) == "n" {
			return fmt.Errorf("setup cancelled by user")
		}

		// Model selection.
		var modelStr string
		modelChoices := append([]string{"auto"}, spec.AvailableModels...)
		if setupCfg != nil {
			modelStr = accountSpecs[i].Model
			if modelStr == "" || modelStr == "auto" {
				modelStr = spec.DefaultModel
			} else {
				valid := false
				for _, m := range spec.AvailableModels {
					if m == modelStr {
						valid = true
						break
					}
				}
				if !valid {
					return fmt.Errorf("invalid model %q for program %q in config file — valid models: %s",
						modelStr, spec.Name, strings.Join(spec.AvailableModels, ", "))
				}
			}
			fmt.Printf("  Model: %s\n", modelStr)
		} else {
			for {
				fmt.Printf("  Model [%s] (default: auto): ", strings.Join(modelChoices, "/"))
				modelInput, _ := reader.ReadString('\n')
				modelInput = strings.TrimSpace(strings.ToLower(modelInput))
				if modelInput == "" || modelInput == "auto" {
					modelStr = spec.DefaultModel
					break
				}
				valid := false
				for _, m := range spec.AvailableModels {
					if m == modelInput {
						valid = true
						break
					}
				}
				if valid {
					modelStr = modelInput
					break
				}
				fmt.Printf("  Invalid model %q — choose from: %s\n", modelInput, strings.Join(modelChoices, ", "))
			}
		}

		cfg.Accounts = append(cfg.Accounts, Account{
			Name:      nameStr,
			Role:      role,
			ConfigDir: configDir,
			Email:     email,
			Program:   programStr,
			Model:     modelStr,
			Verified:  true,
		})

		// Configure statusLine for real-time usage tracking.
		if spec.HasStatusLine {
			scriptPath, slErr := generateStatusLineScriptFile(nameStr)
			if slErr != nil {
				fmt.Printf("  Warning: could not set up usage tracking: %v\n", slErr)
			} else {
				if slErr := configureStatusLine(configDir, scriptPath, programStr); slErr != nil {
					fmt.Printf("  Warning: could not configure statusLine: %v\n", slErr)
				} else {
					fmt.Println("  ✓ Usage tracking configured (statusLine)")
				}
			}
		} else {
			fmt.Println("  Usage tracking: built-in rate limit monitoring (no statusLine needed)")
		}
	}

	if saveErr := SaveConductorConfig(cfg); saveErr != nil {
		return fmt.Errorf("failed to save config: %w", saveErr)
	}

	if ensureErr := EnsureConductorDirs(); ensureErr != nil {
		return fmt.Errorf("failed to create conductor directories: %w", ensureErr)
	}

	base, baseErr := ConductorDir()
	if baseErr != nil {
		return baseErr
	}

	// Keep bin/ directory creation for the usage-receiver scripts.
	binDir := filepath.Join(base, "bin")
	if mkErr := os.MkdirAll(binDir, 0700); mkErr != nil {
		return fmt.Errorf("failed to create bin dir: %w", mkErr)
	}

	registryPath := filepath.Join(base, "registry.json")
	if _, statErr := os.Stat(registryPath); os.IsNotExist(statErr) {
		if writeErr := os.WriteFile(registryPath, []byte("{\"instances\":{},\"updated_at\":\"\"}\n"), 0600); writeErr != nil {
			return fmt.Errorf("failed to create registry.json: %w", writeErr)
		}
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
	fmt.Println("  Run `maestro` in any git repo to start.")

	return nil
}

// loadSetupConfigFile reads and validates a SetupConfig JSON file.
// It performs all fail-fast validation: count 2-10, valid names, no
// duplicates, and at least one orchestrator.
func loadSetupConfigFile(path string) (*SetupConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config file error: cannot read %q: %w", path, err)
	}
	var sc SetupConfig
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("config file error: invalid JSON in %q: %w", path, err)
	}
	count := len(sc.Accounts)
	if count < 2 || count > 10 {
		return nil, fmt.Errorf("config file error: accounts count must be between 2 and 10, got %d", count)
	}
	names := make(map[string]bool)
	orchestratorCount := 0
	for _, spec := range sc.Accounts {
		if !validNameRegex.MatchString(spec.Name) {
			return nil, fmt.Errorf("config file error: invalid name %q — must match /^[a-z0-9][a-z0-9-]*$/", spec.Name)
		}
		if names[spec.Name] {
			return nil, fmt.Errorf("config file error: duplicate account name %q", spec.Name)
		}
		names[spec.Name] = true
		switch strings.ToLower(spec.Role) {
		case "orchestrator":
			orchestratorCount++
		case "worker":
			// valid
		default:
			return nil, fmt.Errorf("config file error: account %q has invalid role %q (must be \"orchestrator\" or \"worker\")", spec.Name, spec.Role)
		}
	}
	if orchestratorCount == 0 {
		return nil, fmt.Errorf("config file error: at least one account must have role \"orchestrator\"")
	}
	if orchestratorCount > 1 {
		return nil, fmt.Errorf("config file error: at most one account may have role \"orchestrator\" (found %d)", orchestratorCount)
	}
	return &sc, nil
}
