package orchestration

import (
	"os"
	"path/filepath"

	"maestro/pkg/accounts"
)

// statusLineUsage is the rate limit data extracted from Claude Code's statusLine JSON.
type statusLineUsage struct {
	FiveHour *rateLimitWindow `json:"five_hour,omitempty"`
}

// rateLimitWindow represents a single rate limit window with usage percentage and reset time.
type rateLimitWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

// collectStatusLineUsage reads the real-time usage data written by the statusLine receiver.
func collectStatusLineUsage(accountName string) *statusLineUsage {
	base, err := accounts.ConductorDir()
	if err != nil {
		return nil
	}
	path := filepath.Join(base, "usage", accountName+".json")

	// Guard: treat a zero-byte file as "no data" to avoid json.Unmarshal errors.
	if info, statErr := os.Stat(path); statErr == nil && info.Size() == 0 {
		return nil
	}

	var usage statusLineUsage
	if err := ReadJSONFile(path, &usage); err != nil {
		return nil
	}
	return &usage
}
