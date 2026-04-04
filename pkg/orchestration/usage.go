package orchestration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"claude-conductor/pkg/accounts"
)

// AccountUsage represents usage data for a single account.
type AccountUsage struct {
	AccountName   string  `json:"account_name"`
	Role          string  `json:"role"`
	UsagePercent  float64 `json:"usage_percent"`  // 0.0 to 100.0
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

// EstimatedLimits per tier (messages per 5-hour rolling window)
var EstimatedLimits = map[string]int{
	"max5":  225,
	"max20": 900,
	"max":   225, // default if tier unknown
}

// GetTierLimit returns the estimated message limit for a subscription tier.
func GetTierLimit(tier string) int {
	if limit, ok := EstimatedLimits[tier]; ok {
		return limit
	}
	return 225 // conservative default
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
			LastUpdated: NowISO(),
		}

		// Try to read stats-cache.json from the account's config dir
		statsPath := filepath.Join(acct.ConfigDir, "stats-cache.json")
		messageCount := countRecentMessages(statsPath)

		// Determine limit based on tier (we don't have tier info in config,
		// so use role as heuristic: orchestrator = max20, worker = max5)
		limit := 225
		if acct.Role == accounts.RoleOrchestrator {
			limit = 900
		}

		usage.MessagesUsed = messageCount
		usage.MessagesLimit = limit
		if limit > 0 {
			usage.UsagePercent = float64(messageCount) / float64(limit) * 100.0
			if usage.UsagePercent > 100.0 {
				usage.UsagePercent = 100.0
			}
		}

		report.Accounts = append(report.Accounts, usage)
	}

	return report, nil
}

// countRecentMessages reads stats-cache.json and counts messages in the last 5 hours.
func countRecentMessages(statsPath string) int {
	data, err := os.ReadFile(statsPath)
	if err != nil {
		return 0
	}

	// stats-cache.json structure varies, but typically contains session data
	// with timestamps. Try to parse as a generic JSON structure.
	var stats interface{}
	if err := json.Unmarshal(data, &stats); err != nil {
		return 0
	}

	// Try to extract message counts from the stats
	// The file may be an array of session records or a map with daily stats
	cutoff := time.Now().Add(-5 * time.Hour)
	return countMessagesAfter(stats, cutoff)
}

// countMessagesAfter recursively looks for message count data after a cutoff time.
func countMessagesAfter(data interface{}, cutoff time.Time) int {
	switch v := data.(type) {
	case map[string]interface{}:
		total := 0
		// Look for "messages" or "message_count" fields
		if msgs, ok := v["messages"]; ok {
			if count, ok := msgs.(float64); ok {
				total += int(count)
			}
		}
		// Look for timestamped entries
		for key, val := range v {
			// Try parsing key as a date (YYYY-MM-DD format)
			if t, err := time.Parse("2006-01-02", key); err == nil {
				if t.After(cutoff) {
					total += countMessagesInEntry(val)
				}
			} else {
				// Recurse into nested structures
				total += countMessagesAfter(val, cutoff)
			}
		}
		return total
	case []interface{}:
		total := 0
		for _, item := range v {
			total += countMessagesAfter(item, cutoff)
		}
		return total
	}
	return 0
}

// countMessagesInEntry extracts message count from a daily stats entry.
func countMessagesInEntry(entry interface{}) int {
	switch v := entry.(type) {
	case float64:
		return int(v)
	case map[string]interface{}:
		total := 0
		for _, val := range v {
			if count, ok := val.(float64); ok {
				total += int(count)
			}
		}
		return total
	}
	return 0
}

// SaveUsageReport writes the usage report to ~/.claude-conductor/usage.json.
func SaveUsageReport(report *UsageReport) error {
	base, err := accounts.ConductorDir()
	if err != nil {
		return err
	}
	path := filepath.Join(base, "usage.json")
	return AtomicWriteJSON(path, report)
}

// LoadUsageReport reads the usage report from ~/.claude-conductor/usage.json.
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
		return nil, err
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
		result += fmt.Sprintf("  %-12s [%s] %5.1f%%  (%d/%d msgs)\n",
			acct.AccountName, bar, acct.UsagePercent, acct.MessagesUsed, acct.MessagesLimit)
	}
	return result
}

func renderUsageBar(percent float64, width int) string {
	filled := int(percent / 100.0 * float64(width))
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
