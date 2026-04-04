package orchestration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTierLimit(t *testing.T) {
	assert.Equal(t, 225, GetTierLimit("max5"))
	assert.Equal(t, 900, GetTierLimit("max20"))
	assert.Equal(t, 225, GetTierLimit("unknown"))
}

func TestRenderUsageBar(t *testing.T) {
	bar := renderUsageBar(50.0, 10)
	assert.Equal(t, "\u2588\u2588\u2588\u2588\u2588\u2591\u2591\u2591\u2591\u2591", bar)

	bar = renderUsageBar(0.0, 10)
	assert.Equal(t, "\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591", bar)

	bar = renderUsageBar(100.0, 10)
	assert.Equal(t, "\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2588", bar)
}

func TestFormatUsageSummary_Empty(t *testing.T) {
	result := FormatUsageSummary(nil)
	assert.Contains(t, result, "No usage data")
}

func TestFormatUsageSummary_WithData(t *testing.T) {
	report := &UsageReport{
		Accounts: []AccountUsage{
			{AccountName: "main", UsagePercent: 29.0, MessagesUsed: 261, MessagesLimit: 900},
			{AccountName: "worker-1", UsagePercent: 78.0, MessagesUsed: 175, MessagesLimit: 225},
		},
	}
	result := FormatUsageSummary(report)
	assert.Contains(t, result, "main")
	assert.Contains(t, result, "29.0%")
	assert.Contains(t, result, "worker-1")
	assert.Contains(t, result, "78.0%")
}
