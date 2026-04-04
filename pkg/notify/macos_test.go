package notify

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBundling(t *testing.T) {
	// Reset state
	lastNotifyTime = time.Time{}

	// First call should update lastNotifyTime
	Notify("Test", "First")
	assert.False(t, lastNotifyTime.IsZero())
	first := lastNotifyTime

	// Second call within 5s should be bundled (lastNotifyTime unchanged)
	time.Sleep(10 * time.Millisecond)
	Notify("Test", "Second")
	assert.Equal(t, first, lastNotifyTime)
}
