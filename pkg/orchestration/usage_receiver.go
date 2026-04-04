package orchestration

import (
	"path/filepath"

	"claude-conductor/pkg/accounts"
)

// StatusLineUsage is the rate limit data extracted from Claude Code's statusLine JSON.
type StatusLineUsage struct {
	FiveHour *RateLimitWindow `json:"five_hour,omitempty"`
	SevenDay *RateLimitWindow `json:"seven_day,omitempty"`
}

// RateLimitWindow represents a single rate limit window with usage percentage and reset time.
type RateLimitWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

// collectStatusLineUsage reads the real-time usage data written by the statusLine receiver.
func collectStatusLineUsage(accountName string) *StatusLineUsage {
	base, err := accounts.ConductorDir()
	if err != nil {
		return nil
	}
	path := filepath.Join(base, "usage", accountName+".json")
	var usage StatusLineUsage
	if err := ReadJSONFile(path, &usage); err != nil {
		return nil
	}
	return &usage
}
