package accounts

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"maestro/pkg/programs"
)

// RunAddAccount adds a single new account to an existing Maestro configuration
// without re-running full setup.
func RunAddAccount(programBin string) (err error) {
	programBin = strings.TrimSpace(programBin)
	cfg, loadErr := LoadConductorConfig()
	if loadErr != nil {
		return fmt.Errorf("failed to load conductor config: %w", loadErr)
	}
	if cfg == nil {
		return fmt.Errorf("no config found; run 'maestro setup' first")
	}

	fmt.Printf("Adding new account to existing configuration (currently %d accounts configured)\n", len(cfg.Accounts))

	// Hard jq requirement — same as setup.
	if _, jqErr := exec.LookPath("jq"); jqErr != nil {
		return fmt.Errorf("jq is required for usage tracking and worker task updates.\nInstall it first:  brew install jq    (macOS)\n                   apt install jq     (Linux)")
	}

	// Browser tip.
	fmt.Println()
	fmt.Println("TIP: Have your account ready. Use a separate private/incognito")
	fmt.Println("browser window to avoid session carryover.")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	// --- Name prompt ---
	// Determine default name: "new-worker" if unused, else "worker-N".
	defaultName := "new-worker"
	for _, acct := range cfg.Accounts {
		if acct.Name == defaultName {
			defaultName = fmt.Sprintf("worker-%d", len(cfg.Accounts))
			break
		}
	}

	var nameStr string
	for {
		fmt.Printf("  Name [%s]: ", defaultName)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			input = defaultName
		}
		if !validNameRegex.MatchString(input) {
			fmt.Printf("  Invalid name %q — must match /^[a-z0-9][a-z0-9-]*$/\n", input)
			continue
		}
		duplicate := false
		for _, acct := range cfg.Accounts {
			if acct.Name == input {
				fmt.Printf("  Name %q is already in use — choose a different name.\n", input)
				duplicate = true
				break
			}
		}
		if !duplicate {
			nameStr = input
			break
		}
	}

	// --- Program selection ---
	fmt.Printf("  Program [%s] (default: claude): ", strings.Join(programs.Names(), "/"))
	progInput, _ := reader.ReadString('\n')
	progInput = strings.TrimSpace(strings.ToLower(progInput))
	if progInput == "" {
		progInput = "claude"
	}
	if !programs.ValidProgram(progInput) {
		return fmt.Errorf("invalid program %q — valid programs: %s", progInput, strings.Join(programs.Names(), ", "))
	}
	programStr := progInput
	spec, _ := programs.Get(programStr)

	// Resolve the binary to use for this account.
	acctBin := programBin
	if acctBin == "" {
		acctBin = spec.Binary
	}

	// --- Role prompt ---
	orchestratorExists := false
	for _, acct := range cfg.Accounts {
		if acct.Role == RoleOrchestrator {
			orchestratorExists = true
			break
		}
	}

	var role Role
	for {
		fmt.Print("  Role [orchestrator/worker] (default: worker): ")
		roleInput, _ := reader.ReadString('\n')
		roleInput = strings.TrimSpace(strings.ToLower(roleInput))
		if roleInput == "" || roleInput == "worker" {
			role = RoleWorker
			break
		} else if roleInput == "orchestrator" {
			role = RoleOrchestrator
			if orchestratorExists {
				fmt.Print("  Warning: an orchestrator is already configured. Having two orchestrators is unusual. Continue? [y/N]: ")
				confirm, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(confirm)) != "y" {
					fmt.Println("  Aborted — keeping role as worker.")
					role = RoleWorker
				}
			}
			break
		} else {
			fmt.Printf("  Invalid role %q — enter \"orchestrator\" or \"worker\".\n", roleInput)
		}
	}

	// Resolve account config directory early so the dry-run validate can include it.
	configDir, dirErr := AccountConfigDir(nameStr)
	if dirErr != nil {
		return dirErr
	}

	// Dry-run validate before committing to OAuth — catches e.g. duplicate orchestrator early.
	// Include ConfigDir in the test account so Validate() doesn't reject it for having
	// an empty config_dir.
	testCfg := *cfg
	testCfg.Accounts = append(testCfg.Accounts, Account{Name: nameStr, Role: role, ConfigDir: configDir})
	if err := testCfg.Validate(); err != nil {
		return fmt.Errorf("cannot add account: %w", err)
	}
	if mkErr := os.MkdirAll(configDir, 0700); mkErr != nil {
		return fmt.Errorf("failed to create account dir: %w", mkErr)
	}

	// Cleanup-on-error: remove the config dir if anything after this fails.
	defer func() {
		if err != nil {
			os.RemoveAll(configDir)
		}
	}()

	// Auth login — branched by program.
	authArgs := strings.Fields(spec.AuthCommand)
	fmt.Printf("  Launching %s %s for %q...\n", acctBin, spec.AuthCommand, nameStr)
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

	// Check for duplicate email against existing accounts.
	// Skip the check when the email is the emailNotAvailable sentinel returned
	// for programs that don't expose an email address in their auth status output.
	if email != emailNotAvailable {
		for _, acct := range cfg.Accounts {
			if acct.Email == email {
				fmt.Printf("\n  WARNING: This email is already used by account %q.\n", acct.Name)
				fmt.Print("  Continue anyway? [y/N]: ")
				dupConfirm, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(dupConfirm)) != "y" {
					return fmt.Errorf("add-account cancelled — duplicate email")
				}
				break
			}
		}
	}

	// --- Model selection ---
	modelChoices := append([]string{"auto"}, spec.AvailableModels...)
	var modelStr string
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

	// Append account and save.
	cfg.Accounts = append(cfg.Accounts, Account{
		Name:      nameStr,
		Role:      role,
		ConfigDir: configDir,
		Email:     email,
		Program:   programStr,
		Model:     modelStr,
		Verified:  true,
	})

	if saveErr := SaveConductorConfig(cfg); saveErr != nil {
		return fmt.Errorf("failed to save config: %w", saveErr)
	}

	base, baseErr := ConductorDir()
	if baseErr != nil {
		return baseErr
	}

	fmt.Printf("\n✓ Account %q (%s) added successfully.\n", nameStr, role)
	fmt.Printf("  Config: %s now has %d accounts.\n", filepath.Join(base, ConfigFileName), len(cfg.Accounts))
	fmt.Println("  Run `maestro workers` to see active instances.")

	return nil
}
