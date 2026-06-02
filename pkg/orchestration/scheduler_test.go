package orchestration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeWorker(name, role string, usagePct float64, limit int, resetAt string) AccountUsage {
	return AccountUsage{
		AccountName:   name,
		Role:          role,
		UsagePercent:  usagePct,
		MessagesUsed:  int(usagePct / 100.0 * float64(limit)),
		MessagesLimit: limit,
		WindowResetAt: resetAt,
		LastUpdated:   NowISO(),
	}
}

// --- ComputeSchedule tests ---

func TestComputeSchedule_AllBelowThreshold_PickLowestUsage(t *testing.T) {
	now := time.Now()
	workers := []AccountUsage{
		makeWorker("worker-1", "worker", 30.0, 225, ""),
		makeWorker("worker-2", "worker", 20.0, 225, ""),
		makeWorker("worker-3", "worker", 50.0, 225, ""),
	}

	result := ComputeSchedule(workers, 15*time.Minute, now)

	assert.False(t, result.NoRouteFound)
	assert.True(t, result.Worker.Immediate)
	assert.Equal(t, "worker-2", result.Worker.AccountName, "should pick worker with lowest usage")
}

func TestComputeSchedule_AllSaturated_OneResetsSoon(t *testing.T) {
	now := time.Now()
	resetIn5Min := now.Add(5 * time.Minute).UTC().Format(time.RFC3339)
	workers := []AccountUsage{
		makeWorker("worker-1", "worker", 90.0, 225, ""),
		makeWorker("worker-2", "worker", 85.0, 225, resetIn5Min),
		makeWorker("worker-3", "worker", 95.0, 225, ""),
	}

	result := ComputeSchedule(workers, 15*time.Minute, now)

	assert.False(t, result.NoRouteFound)
	assert.False(t, result.Worker.Immediate)
	assert.Equal(t, "worker-2", result.Worker.AccountName, "should schedule for worker that resets soon")
	assert.InDelta(t, 5*time.Minute, result.Worker.WaitDuration, float64(2*time.Second))
}

func TestComputeSchedule_AllSaturated_NoResetTimes(t *testing.T) {
	now := time.Now()
	workers := []AccountUsage{
		makeWorker("worker-1", "worker", 90.0, 225, ""),
		makeWorker("worker-2", "worker", 85.0, 225, ""),
	}

	result := ComputeSchedule(workers, 15*time.Minute, now)

	assert.True(t, result.NoRouteFound, "should return NoRouteFound when all saturated with no reset info")
}

func TestComputeSchedule_MixImmediateAndWait_ImmediateWins(t *testing.T) {
	now := time.Now()
	resetIn5Min := now.Add(5 * time.Minute).UTC().Format(time.RFC3339)
	workers := []AccountUsage{
		makeWorker("worker-1", "worker", 50.0, 225, ""),          // immediate candidate
		makeWorker("worker-2", "worker", 90.0, 225, resetIn5Min), // wait candidate (reset soon)
	}

	result := ComputeSchedule(workers, 15*time.Minute, now)

	assert.False(t, result.NoRouteFound)
	assert.True(t, result.Worker.Immediate, "immediate candidate should always win over wait candidate")
	assert.Equal(t, "worker-1", result.Worker.AccountName)
}

func TestComputeSchedule_SkipsOrchestrators(t *testing.T) {
	now := time.Now()
	workers := []AccountUsage{
		makeWorker("orchestrator", "orchestrator", 10.0, 900, ""),
		makeWorker("worker-1", "worker", 30.0, 225, ""),
	}

	result := ComputeSchedule(workers, 15*time.Minute, now)

	assert.False(t, result.NoRouteFound)
	assert.Equal(t, "worker-1", result.Worker.AccountName, "should skip orchestrator")
}

func TestComputeSchedule_ExactlyAtThreshold_Saturated(t *testing.T) {
	now := time.Now()
	resetIn10Min := now.Add(10 * time.Minute).UTC().Format(time.RFC3339)
	workers := []AccountUsage{
		makeWorker("worker-1", "worker", SchedulerSaturationThreshold, 225, resetIn10Min),
	}

	result := ComputeSchedule(workers, 15*time.Minute, now)

	// At exactly the threshold, the worker is treated as saturated (not <threshold)
	assert.False(t, result.NoRouteFound)
	assert.False(t, result.Worker.Immediate, "worker at exactly the saturation threshold should be treated as saturated")
	assert.Equal(t, "worker-1", result.Worker.AccountName)
}

func TestComputeSchedule_NoWorkers(t *testing.T) {
	now := time.Now()
	result := ComputeSchedule(nil, 15*time.Minute, now)
	assert.True(t, result.NoRouteFound)
}

func TestComputeSchedule_ResetInPast_Excluded(t *testing.T) {
	now := time.Now()
	resetInPast := now.Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	workers := []AccountUsage{
		makeWorker("worker-1", "worker", 90.0, 225, resetInPast),
	}

	result := ComputeSchedule(workers, 15*time.Minute, now)

	// Reset is in the past, so neither immediate (saturated) nor valid wait option
	assert.True(t, result.NoRouteFound)
}

func TestComputeSchedule_ZeroMessagesLimit_Skipped(t *testing.T) {
	now := time.Now()
	workers := []AccountUsage{
		makeWorker("worker-zero", "worker", 0.0, 0, ""),
		makeWorker("worker-ok", "worker", 20.0, 225, ""),
	}

	result := ComputeSchedule(workers, 15*time.Minute, now)

	assert.False(t, result.NoRouteFound)
	assert.Equal(t, "worker-ok", result.Worker.AccountName, "should skip worker with zero MessagesLimit")
}

func TestComputeSchedule_AllZeroLimit_NoRoute(t *testing.T) {
	now := time.Now()
	workers := []AccountUsage{
		makeWorker("worker-1", "worker", 0.0, 0, ""),
		makeWorker("worker-2", "worker", 0.0, 0, ""),
	}

	result := ComputeSchedule(workers, 15*time.Minute, now)

	assert.True(t, result.NoRouteFound, "should find no route when all workers have zero limit")
}

// --- ResolveTaskDuration tests ---

func TestResolveTaskDuration_FlagProvided(t *testing.T) {
	d := ResolveTaskDuration(30, "worker-1", nil)
	assert.Equal(t, 30*time.Minute, d)
}

func TestResolveTaskDuration_StatsAvailable(t *testing.T) {
	stats := &DurationStats{
		Workers: map[string]*WorkerDurationRecord{
			"worker-1": {
				AccountName:    "worker-1",
				SampleCount:    5,
				AverageSeconds: 600.0, // 10 minutes
			},
		},
	}

	d := ResolveTaskDuration(0, "worker-1", stats)
	assert.Equal(t, 10*time.Minute, d)
}

func TestResolveTaskDuration_NeitherFlagNorStats(t *testing.T) {
	d := ResolveTaskDuration(0, "worker-1", nil)
	assert.Equal(t, time.Duration(DefaultTaskDurationMin)*time.Minute, d)
}

func TestResolveTaskDuration_FlagOverridesStats(t *testing.T) {
	stats := &DurationStats{
		Workers: map[string]*WorkerDurationRecord{
			"worker-1": {
				AccountName:    "worker-1",
				SampleCount:    5,
				AverageSeconds: 600.0,
			},
		},
	}

	d := ResolveTaskDuration(25, "worker-1", stats)
	assert.Equal(t, 25*time.Minute, d, "flag should take priority over stats")
}

// --- UpdateDurationStats tests ---

func TestUpdateDurationStats_FirstSample(t *testing.T) {
	tmpDir := t.TempDir()
	// Override the home dir for test isolation
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create the conductor dir
	conductorDir := filepath.Join(tmpDir, ".maestro")
	require.NoError(t, os.MkdirAll(conductorDir, 0700))

	dispatched := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	completed := time.Now().UTC().Format(time.RFC3339)
	task := &Task{
		ID:            "task-123",
		WorkerAccount: "worker-1",
		DispatchedAt:  &dispatched,
		CompletedAt:   &completed,
	}

	err := UpdateDurationStats(task)
	require.NoError(t, err)

	stats, err := LoadDurationStats()
	require.NoError(t, err)
	require.NotNil(t, stats)

	record, ok := stats.Workers["worker-1"]
	require.True(t, ok)
	assert.Equal(t, 1, record.SampleCount)
	// Average should be approximately 10 minutes (600 seconds)
	assert.InDelta(t, 600.0, record.AverageSeconds, 2.0)
}

func TestUpdateDurationStats_SecondSample_EMA(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	conductorDir := filepath.Join(tmpDir, ".maestro")
	require.NoError(t, os.MkdirAll(conductorDir, 0700))

	// Seed with an initial record (average = 600s = 10 minutes)
	initialStats := &DurationStats{
		Workers: map[string]*WorkerDurationRecord{
			"worker-1": {
				AccountName:    "worker-1",
				SampleCount:    1,
				TotalSeconds:   600.0,
				AverageSeconds: 600.0,
				LastUpdated:    NowISO(),
			},
		},
		UpdatedAt: NowISO(),
	}
	require.NoError(t, SaveDurationStats(initialStats))

	// New task took 5 minutes (300 seconds)
	dispatched := time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	completed := time.Now().UTC().Format(time.RFC3339)
	task := &Task{
		ID:            "task-456",
		WorkerAccount: "worker-1",
		DispatchedAt:  &dispatched,
		CompletedAt:   &completed,
	}

	err := UpdateDurationStats(task)
	require.NoError(t, err)

	stats, err := LoadDurationStats()
	require.NoError(t, err)
	require.NotNil(t, stats)

	record := stats.Workers["worker-1"]
	assert.Equal(t, 2, record.SampleCount)
	// EMA: 0.2 * 300 + 0.8 * 600 = 60 + 480 = 540
	assert.InDelta(t, 540.0, record.AverageSeconds, 2.0)
}

func TestUpdateDurationStats_MissingTimestamps_NoOp(t *testing.T) {
	// No DispatchedAt
	task1 := &Task{
		ID:            "task-789",
		WorkerAccount: "worker-1",
		CompletedAt:   ptrStr(NowISO()),
	}
	err := UpdateDurationStats(task1)
	assert.NoError(t, err)

	// No CompletedAt
	task2 := &Task{
		ID:            "task-790",
		WorkerAccount: "worker-1",
		DispatchedAt:  ptrStr(NowISO()),
	}
	err = UpdateDurationStats(task2)
	assert.NoError(t, err)

	// Both nil
	task3 := &Task{
		ID:            "task-791",
		WorkerAccount: "worker-1",
	}
	err = UpdateDurationStats(task3)
	assert.NoError(t, err)
}

// --- RoutingAdvice tests ---

func TestFormatRoutingAdvice_Scheduled(t *testing.T) {
	advice := RoutingAdvice{
		Strategy:        StrategyScheduled,
		ScheduledWorker: "worker-1",
		ScheduledAt:     time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC),
		ScheduledWait:   5*time.Minute + 30*time.Second,
	}
	result := FormatRoutingAdvice(advice)
	assert.Contains(t, result, "worker-1")
	assert.Contains(t, result, "5m30s")
	assert.Contains(t, result, "14:30 UTC")
}

func TestFormatRoutingAdvice_Direct(t *testing.T) {
	advice := RoutingAdvice{
		Strategy:     StrategyDirect,
		TargetWorker: "worker-2",
	}
	result := FormatRoutingAdvice(advice)
	assert.Contains(t, result, "worker-2")
	assert.Contains(t, result, "Direct")
}

// --- DurationStats.AverageFor tests ---

func TestAverageFor_NilStats(t *testing.T) {
	var stats *DurationStats
	_, ok := stats.AverageFor("worker-1")
	assert.False(t, ok)
}

func TestAverageFor_MissingWorker(t *testing.T) {
	stats := &DurationStats{
		Workers: map[string]*WorkerDurationRecord{},
	}
	_, ok := stats.AverageFor("worker-1")
	assert.False(t, ok)
}

func TestAverageFor_HasData(t *testing.T) {
	stats := &DurationStats{
		Workers: map[string]*WorkerDurationRecord{
			"worker-1": {
				AccountName:    "worker-1",
				SampleCount:    3,
				AverageSeconds: 900.0,
			},
		},
	}
	avg, ok := stats.AverageFor("worker-1")
	assert.True(t, ok)
	assert.Equal(t, 15*time.Minute, avg)
}

func ptrStr(s string) *string {
	return &s
}

// TestResolveScheduledInstance verifies the routing fix: the scheduler chooses
// an account, and dispatch must be sent to the worker instance backing that
// account (not the originally-named instance). Regression for the bug where
// --schedule discarded the routing decision entirely.
func TestResolveScheduledInstance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	reg := Registry{
		UpdatedAt: NowISO(),
		Instances: map[string]RegistryEntry{
			"orch-1":   {Account: "alice", Role: "orchestrator", Status: RegistryStatusRunning},
			"worker-1": {Account: "bob", Role: "worker", Status: RegistryStatusRunning},
			"worker-2": {Account: "carol", Role: "worker", Status: RegistryStatusRunning},
			"worker-3": {Account: "dave", Role: "worker", Status: RegistryStatusDead},
		},
	}
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".maestro"), 0700))
	require.NoError(t, AtomicWriteJSON(filepath.Join(home, ".maestro", "registry.json"), reg))

	// A chosen account resolves to the worker instance backing it.
	assert.Equal(t, "worker-2", resolveScheduledInstance("carol", "fallback"))
	assert.Equal(t, "worker-1", resolveScheduledInstance("bob", "fallback"))

	// An orchestrator account is not a worker target → fall back.
	assert.Equal(t, "fallback", resolveScheduledInstance("alice", "fallback"))

	// A dead worker's account is skipped → fall back.
	assert.Equal(t, "fallback", resolveScheduledInstance("dave", "fallback"))

	// An unknown account → fall back to the originally-requested instance.
	assert.Equal(t, "fallback", resolveScheduledInstance("nobody", "fallback"))
}
