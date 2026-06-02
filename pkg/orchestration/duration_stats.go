package orchestration

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"maestro/log"
	"maestro/pkg/accounts"
)

const (
	// emaAlpha is the smoothing factor for the exponential moving average.
	emaAlpha = 0.2
)

// WorkerDurationRecord tracks task duration statistics for a single worker account.
type WorkerDurationRecord struct {
	AccountName    string  `json:"account_name"`
	SampleCount    int     `json:"sample_count"`
	TotalSeconds   float64 `json:"total_seconds"`
	AverageSeconds float64 `json:"average_seconds"`
	LastUpdated    string  `json:"last_updated"`
}

// DurationStats holds duration statistics for all workers.
type DurationStats struct {
	Workers   map[string]*WorkerDurationRecord `json:"workers"`
	UpdatedAt string                           `json:"updated_at"`
}

// durationStatsPath returns the path to the duration_stats.json file.
func durationStatsPath() (string, error) {
	base, err := accounts.ConductorDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "duration_stats.json"), nil
}

// LoadDurationStats reads the duration stats from ~/.maestro/duration_stats.json.
// Returns nil, nil if the file does not exist or is empty.
// Returns an empty DurationStats if the file contains corrupt JSON (logs a warning).
func LoadDurationStats() (*DurationStats, error) {
	path, err := durationStatsPath()
	if err != nil {
		return nil, err
	}
	var stats DurationStats
	if err := ReadJSONFile(path, &stats); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		// Corrupt JSON (e.g. truncated mid-write): log and return empty stats
		// rather than failing callers that just want to record a new sample.
		log.WarningLog.Printf("duration stats: %s is corrupt, returning empty: %v", path, err)
		return &DurationStats{Workers: make(map[string]*WorkerDurationRecord)}, nil
	}
	if stats.Workers == nil {
		stats.Workers = make(map[string]*WorkerDurationRecord)
	}
	return &stats, nil
}

// SaveDurationStats writes the duration stats atomically.
// Creates the parent directory if it does not already exist (first-run safety).
func SaveDurationStats(stats *DurationStats) error {
	path, err := durationStatsPath()
	if err != nil {
		return err
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0700); mkErr != nil {
		return fmt.Errorf("failed to create duration stats dir: %w", mkErr)
	}
	stats.UpdatedAt = NowISO()
	return AtomicWriteJSON(path, stats)
}

// UpdateDurationStats records the duration of a completed task, updating the EMA.
// Both DispatchedAt and CompletedAt must be set on the task; otherwise this is a no-op.
func UpdateDurationStats(task *Task) error {
	if task.DispatchedAt == nil || task.CompletedAt == nil {
		return nil
	}

	dispatched, err := ParseISO(*task.DispatchedAt)
	if err != nil {
		return nil // silently ignore malformed timestamps
	}
	completed, err := ParseISO(*task.CompletedAt)
	if err != nil {
		return nil
	}

	durationSecs := completed.Sub(dispatched).Seconds()
	if durationSecs <= 0 {
		return nil
	}

	stats, err := LoadDurationStats()
	if err != nil {
		return err
	}
	if stats == nil {
		stats = &DurationStats{
			Workers: make(map[string]*WorkerDurationRecord),
		}
	}

	record, exists := stats.Workers[task.WorkerAccount]
	if !exists {
		// First sample: average = duration
		record = &WorkerDurationRecord{
			AccountName:    task.WorkerAccount,
			SampleCount:    1,
			TotalSeconds:   durationSecs,
			AverageSeconds: durationSecs,
			LastUpdated:    NowISO(),
		}
	} else {
		// EMA: new_avg = alpha * new_sample + (1 - alpha) * old_avg
		record.SampleCount++
		record.TotalSeconds += durationSecs
		record.AverageSeconds = emaAlpha*durationSecs + (1-emaAlpha)*record.AverageSeconds
		record.LastUpdated = NowISO()
	}

	stats.Workers[task.WorkerAccount] = record
	return SaveDurationStats(stats)
}

// AverageFor returns the average task duration for the given account.
// Returns false if no data is available for that account.
func (d *DurationStats) AverageFor(accountName string) (time.Duration, bool) {
	if d == nil || d.Workers == nil {
		return 0, false
	}
	record, ok := d.Workers[accountName]
	if !ok || record.SampleCount == 0 {
		return 0, false
	}
	return time.Duration(record.AverageSeconds * float64(time.Second)), true
}
