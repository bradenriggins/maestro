package cmd

import (
	"fmt"
	"maestro/pkg/accounts"
	"maestro/pkg/orchestration"
	"maestro/pkg/programs"
	"os"
	"os/exec"
)

// RunDoctor performs a diagnostic check of the maestro installation,
// verifying configuration, accounts, registry, tasks, permissions,
// and required binaries. It returns an exit-code-bearing error for
// warning/error outcomes so process termination stays at the top level.
func RunDoctor() error {
	var warnings, errs int

	fmt.Println("Running diagnostics...")

	// Check config
	cfg, err := accounts.LoadConductorConfig()
	if err != nil {
		fmt.Printf("  ✗ Config: %v\n", err)
		errs++
	} else if cfg == nil {
		fmt.Printf("  ✗ Config: not found (run setup first)\n")
		errs++
	} else {
		fmt.Printf("  ✓ Config: %d accounts configured\n", len(cfg.Accounts))
	}

	// Check account directories
	if cfg != nil {
		for _, acct := range cfg.Accounts {
			if _, err := os.Stat(acct.ConfigDir); err != nil {
				fmt.Printf("  ✗ Account %s: config dir missing (%s)\n", acct.Name, acct.ConfigDir)
				errs++
			} else {
				fmt.Printf("  ✓ Account %s: config dir exists\n", acct.Name)
			}
		}
	}

	// Check registry
	reg, err := orchestration.LoadRegistry()
	if err != nil {
		fmt.Printf("  ✗ Registry: %v\n", err)
		errs++
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
		if dead > 0 {
			warnings++
		}
	}

	// Check task files
	store, err := orchestration.NewTaskStore()
	if err != nil {
		fmt.Printf("  ✗ Task store: %v\n", err)
		errs++
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
			warnings++
		} else {
			fmt.Printf("  ✓ Tasks: %d total\n", len(tasks))
		}
	}

	// Check permissions
	base, _ := accounts.ConductorDir()
	if info, err := os.Stat(base); err == nil {
		if info.Mode().Perm() != 0700 {
			fmt.Printf("  ✗ Permissions: %s is %o (should be 0700)\n", base, info.Mode().Perm())
			warnings++
		} else {
			fmt.Printf("  ✓ Permissions: correct\n")
		}
	}

	// Check required binaries — dynamically based on configured accounts.
	requiredBins := []string{"tmux", "jq"}
	if cfg != nil {
		seen := map[string]bool{}
		for _, acct := range cfg.Accounts {
			spec, specOk := programs.Get(acct.Program)
			if !specOk {
				fmt.Printf("  ✗ Account %s: unknown program %q\n", acct.Name, acct.Program)
				errs++
				continue
			}
			if !seen[spec.Binary] {
				requiredBins = append(requiredBins, spec.Binary)
				seen[spec.Binary] = true
			}
		}
	} else {
		requiredBins = append(requiredBins, "claude") // default
	}
	fmt.Println("\nRequired binaries:")
	for _, bin := range requiredBins {
		if _, err := exec.LookPath(bin); err != nil {
			fmt.Printf("  ✗ %s: not found in PATH\n", bin)
			errs++
		} else {
			fmt.Printf("  ✓ %s: found\n", bin)
		}
	}

	if errs > 0 {
		fmt.Printf("\n%d error(s), %d warning(s) found.\n", errs, warnings)
		return &ExitError{Code: 2}
	}
	if warnings > 0 {
		fmt.Printf("\n%d warning(s) found.\n", warnings)
		return &ExitError{Code: 1}
	}
	fmt.Println("\nAll checks passed.")
	return nil
}
