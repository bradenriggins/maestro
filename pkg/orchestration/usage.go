package orchestration

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"maestro/log"
	"maestro/pkg/accounts"
)

// AccountUsage represents usage data for a single account.
type AccountUsage struct {
	AccountName   string  `json:"account_name"`
	Role          string  `json:"role"`
	Program       string  `json:"program,omitempty"`
	Model         string  `json:"model,omitempty"`
	UsagePercent  float64 `json:"usage_percent"` // 0.0 to 100.0
	MessagesUsed  int     `json:"messages_used"`
	MessagesLimit int     `json:"messages_limit"`
	WindowResetAt string  `json:"window_reset_at,omitempty"` // ISO timestamp
	LastUpdated   string  `json:"last_updated"`
}

// UsageReport contains usage for all accounts.
type UsageReport struct {
	Accounts  []AccountUsage `json:"accounts"`
	UpdatedAt string         `json:"updated_at"`
}

// CollectUsage gathers usage data for all configured accounts.
// It reads Claude Code's local stats from each account's config dir.
func CollectUsage(cfg *accounts.ConductorConfig) (*UsageReport, error) {
	report := &UsageReport{
		UpdatedAt: NowISO(),
	}

	for _, acct := range cfg.Accounts {
		usage := AccountUsage{
			AccountName: acct.Name,
			Role:        string(acct.Role),
			Program:     acct.Program,
			Model:       acct.Model,
			LastUpdated: NowISO(),
		}

		// Determine limit based on tier (we don't have tier info in config,
		// so use role as heuristic: orchestrator = max20, worker = max5)
		limit := 225
		if acct.Role == accounts.RoleOrchestrator {
			limit = 900
		}
		usage.MessagesLimit = limit

		if acct.Program == "codex" {
			// Codex: use rate limit polling
			codexUsage := collectCodexUsage(acct.Name, acct.ConfigDir)
			if codexUsage != nil {
				usage.UsagePercent = codexUsage.UsedPercent
				usage.MessagesUsed = int(codexUsage.UsedPercent * float64(limit) / 100.0)
				if codexUsage.ResetsAt > 0 {
					resetTime := time.Unix(codexUsage.ResetsAt, 0).UTC().Format(time.RFC3339)
					usage.WindowResetAt = resetTime
				}
			}
		} else {
			// Claude: try real-time statusLine data
			slUsage := collectStatusLineUsage(acct.Name)
			if slUsage != nil && slUsage.FiveHour != nil {
				usage.UsagePercent = slUsage.FiveHour.UsedPercentage
				usage.MessagesUsed = int(slUsage.FiveHour.UsedPercentage * float64(limit) / 100.0)
				if slUsage.FiveHour.ResetsAt > 0 {
					resetTime := time.Unix(slUsage.FiveHour.ResetsAt, 0).UTC().Format(time.RFC3339)
					usage.WindowResetAt = resetTime
				}
			} else {
				// No real-time data available -- show 0% rather than fabricating an estimate.
				usage.UsagePercent = 0
				usage.MessagesUsed = 0
			}
		}

		report.Accounts = append(report.Accounts, usage)
	}

	return report, nil
}

// SaveUsageReport writes the usage report to ~/.maestro/usage.json.
// It creates the conductor directory if it does not already exist.
func SaveUsageReport(report *UsageReport) error {
	base, err := accounts.ConductorDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return fmt.Errorf("failed to create conductor dir: %w", err)
	}
	path := filepath.Join(base, "usage.json")
	return AtomicWriteJSON(path, report)
}

// LoadUsageReport reads the usage report from ~/.maestro/usage.json.
// Returns nil, nil if the file is missing or empty.
// Returns nil, nil if the file contains corrupt JSON (logs a warning).
func LoadUsageReport() (*UsageReport, error) {
	base, err := accounts.ConductorDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(base, "usage.json")
	var report UsageReport
	if err := ReadJSONFile(path, &report); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		log.WarningLog.Printf("usage report: %s is corrupt, returning nil: %v", path, err)
		return nil, nil
	}
	return &report, nil
}

// FormatUsageSummary returns a human-readable usage summary for all accounts.
func FormatUsageSummary(report *UsageReport) string {
	if report == nil || len(report.Accounts) == 0 {
		return "No usage data available"
	}
	var result string
	for _, acct := range report.Accounts {
		bar := renderUsageBar(acct.UsagePercent, 10)
		resetInfo := ""
		if acct.WindowResetAt != "" && len(acct.WindowResetAt) >= 16 {
			resetInfo = fmt.Sprintf("  (resets: %s)", acct.WindowResetAt[:16])
		}
		result += fmt.Sprintf("  %-12s [%s] %5.1f%%  (%d/%d msgs)%s\n",
			acct.AccountName, bar, acct.UsagePercent, acct.MessagesUsed, acct.MessagesLimit, resetInfo)
	}
	return result
}

func renderUsageBar(percent float64, width int) string {
	filled := int(percent / 100.0 * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "\u2588"
		} else {
			bar += "\u2591"
		}
	}
	return bar
}

// RoutingStrategy describes how the orchestrator should route the next task.
type RoutingStrategy int

const (
	// StrategyDispatchToWorker: workers have capacity, dispatch normally.
	StrategyDispatchToWorker RoutingStrategy = iota
	// StrategyOrchestratorSubagent: all workers are saturated but orchestrator
	// has significant headroom — spawn a subagent on the orchestrator account.
	StrategyOrchestratorSubagent
	// StrategyWait: all accounts are near capacity, hold off.
	StrategyWait
)

// Scheduler-specific routing strategy constants (used by ComputeSchedule / RunScheduledDispatch).
const (
	StrategyDirect     RoutingStrategy = 10
	StrategyRoundRobin RoutingStrategy = 11
	StrategyLeastUsage RoutingStrategy = 12
	StrategyScheduled  RoutingStrategy = 13
)

// RoutingAdvice holds the recommended task routing strategy and context.
type RoutingAdvice struct {
	Strategy          RoutingStrategy
	RecommendedWorker string  // non-empty when Strategy == StrategyDispatchToWorker
	OrchestratorUsage float64 // 0.0-100.0
	MinWorkerUsage    float64
	MaxWorkerUsage    float64
	Reason            string
	TargetWorker      string        // used by scheduler StrategyDirect
	ScheduledWorker   string        // used by scheduler StrategyScheduled
	ScheduledAt       time.Time     // when the scheduled dispatch will fire
	ScheduledWait     time.Duration // how long to wait before dispatching
}

// WorkerSaturationThreshold is the usage % above which a worker is considered saturated.
const WorkerSaturationThreshold = 75.0

// OrchestratorHeadroomThreshold is the usage % below which the orchestrator is
// considered to have meaningful headroom for self-spawned subagents.
const OrchestratorHeadroomThreshold = 40.0

// GetRoutingAdvice returns the recommended task routing strategy based on current
// account usage. It requires a populated UsageReport (from CollectUsage).
func GetRoutingAdvice(report *UsageReport) RoutingAdvice {
	if report == nil || len(report.Accounts) == 0 {
		return RoutingAdvice{
			Strategy: StrategyDispatchToWorker,
			Reason:   "no usage data available; dispatching to worker",
		}
	}

	var orchestratorUsage float64
	orchestratorFound := false
	var workerUsages []float64
	var bestWorker string
	bestWorkerUsage := 101.0 // above max so first real worker wins

	for _, acct := range report.Accounts {
		if acct.Role == "orchestrator" {
			orchestratorUsage = acct.UsagePercent
			orchestratorFound = true
		} else {
			workerUsages = append(workerUsages, acct.UsagePercent)
			if acct.UsagePercent < bestWorkerUsage {
				bestWorkerUsage = acct.UsagePercent
				bestWorker = acct.AccountName
			}
		}
	}

	if len(workerUsages) == 0 {
		return RoutingAdvice{
			Strategy:          StrategyOrchestratorSubagent,
			OrchestratorUsage: orchestratorUsage,
			Reason:            "no workers configured; use orchestrator subagents",
		}
	}

	// Compute min/max worker usage
	minUsage, maxUsage := workerUsages[0], workerUsages[0]
	allSaturated := true
	for _, u := range workerUsages {
		if u < minUsage {
			minUsage = u
		}
		if u > maxUsage {
			maxUsage = u
		}
		if u < WorkerSaturationThreshold {
			allSaturated = false
		}
	}

	advice := RoutingAdvice{
		OrchestratorUsage: orchestratorUsage,
		MinWorkerUsage:    minUsage,
		MaxWorkerUsage:    maxUsage,
	}

	if !allSaturated {
		advice.Strategy = StrategyDispatchToWorker
		advice.RecommendedWorker = bestWorker
		advice.MinWorkerUsage = bestWorkerUsage
		advice.Reason = fmt.Sprintf("worker %q has %.0f%% usage (below %.0f%% threshold)",
			bestWorker, bestWorkerUsage, WorkerSaturationThreshold)
		return advice
	}

	// All workers are saturated — check if orchestrator has meaningful headroom
	if orchestratorFound && orchestratorUsage < OrchestratorHeadroomThreshold {
		advice.Strategy = StrategyOrchestratorSubagent
		advice.Reason = fmt.Sprintf(
			"all workers saturated (min %.0f%%), orchestrator has %.0f%% headroom — use own subagents",
			minUsage, 100.0-orchestratorUsage)
		return advice
	}

	// Everything is saturated
	advice.Strategy = StrategyWait
	advice.Reason = fmt.Sprintf(
		"all workers saturated (min %.0f%%) and orchestrator at %.0f%% — wait for window reset",
		minUsage, orchestratorUsage)
	return advice
}

// FormatRoutingAdvice returns a human-readable one-liner for the routing advice.
func FormatRoutingAdvice(advice RoutingAdvice) string {
	switch advice.Strategy {
	case StrategyOrchestratorSubagent:
		return fmt.Sprintf("⚡ Workers saturated (%.0f%%+ used) — spawn own subagent (%.0f%% headroom)",
			advice.MinWorkerUsage, 100.0-advice.OrchestratorUsage)
	case StrategyWait:
		return fmt.Sprintf("⏳ All accounts near capacity — wait for 5h window reset")
	case StrategyScheduled:
		return fmt.Sprintf("Scheduling for %q in %s (reset at %s)",
			advice.ScheduledWorker, advice.ScheduledWait.Round(time.Second), advice.ScheduledAt.Format("15:04 UTC"))
	case StrategyDirect:
		return fmt.Sprintf("Direct dispatch to %q", advice.TargetWorker)
	case StrategyRoundRobin:
		if advice.RecommendedWorker != "" {
			return fmt.Sprintf("Round-robin dispatch to %q", advice.RecommendedWorker)
		}
		return "Round-robin dispatch to next available worker"
	case StrategyLeastUsage:
		if advice.RecommendedWorker != "" {
			return fmt.Sprintf("Least-usage dispatch to %q (%.0f%% used)", advice.RecommendedWorker, advice.MinWorkerUsage)
		}
		return "Least-usage dispatch to lowest-utilization worker"
	default:
		if advice.RecommendedWorker != "" {
			return fmt.Sprintf("→ Dispatch to %q (%.0f%% used)", advice.RecommendedWorker, advice.MinWorkerUsage)
		}
		return "→ Dispatch to worker"
	}
}
