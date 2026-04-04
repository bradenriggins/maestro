package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// keyMsg is a helper that builds a tea.KeyMsg for the given key type.
func keyMsg(t tea.KeyType, runes ...rune) tea.KeyMsg {
	return tea.KeyMsg{Type: t, Runes: runes}
}

func TestNewQuickDispatchOverlay_defaults(t *testing.T) {
	workers := []string{"worker-1", "worker-2", "worker-3"}
	q := NewQuickDispatchOverlay(workers)

	require.False(t, q.IsSubmitted())
	require.False(t, q.IsCanceled())
	require.Equal(t, "worker-1", q.GetWorker())
	require.Equal(t, "", q.GetTask())
}

func TestNewQuickDispatchOverlay_noWorkers(t *testing.T) {
	q := NewQuickDispatchOverlay(nil)
	require.Equal(t, "", q.GetWorker())
}

func TestSetWidth(t *testing.T) {
	q := NewQuickDispatchOverlay(nil)

	q.SetWidth(100)
	require.Equal(t, 60, q.width)

	// Minimum width is 40
	q.SetWidth(10)
	require.Equal(t, 40, q.width)

	q.SetWidth(0)
	require.Equal(t, 40, q.width)
}

func TestHandleKeyPress_workerNavigation(t *testing.T) {
	workers := []string{"alpha", "beta", "gamma"}
	q := NewQuickDispatchOverlay(workers)

	// Left at start is a no-op
	close := q.HandleKeyPress(keyMsg(tea.KeyLeft))
	require.False(t, close)
	require.Equal(t, "alpha", q.GetWorker())

	// Right advances
	close = q.HandleKeyPress(keyMsg(tea.KeyRight))
	require.False(t, close)
	require.Equal(t, "beta", q.GetWorker())

	close = q.HandleKeyPress(keyMsg(tea.KeyRight))
	require.False(t, close)
	require.Equal(t, "gamma", q.GetWorker())

	// Right at end is a no-op
	close = q.HandleKeyPress(keyMsg(tea.KeyRight))
	require.False(t, close)
	require.Equal(t, "gamma", q.GetWorker())

	// Left goes back
	close = q.HandleKeyPress(keyMsg(tea.KeyLeft))
	require.False(t, close)
	require.Equal(t, "beta", q.GetWorker())
}

func TestHandleKeyPress_taskInput(t *testing.T) {
	q := NewQuickDispatchOverlay([]string{"w1"})

	// Type characters
	q.HandleKeyPress(keyMsg(tea.KeyRunes, 'h'))
	q.HandleKeyPress(keyMsg(tea.KeyRunes, 'i'))
	require.Equal(t, "hi", q.GetTask())

	// Space
	q.HandleKeyPress(keyMsg(tea.KeySpace))
	require.Equal(t, "hi", q.GetTask()) // trimmed: trailing space not in result

	// Type after space
	q.HandleKeyPress(keyMsg(tea.KeyRunes, 't'))
	q.HandleKeyPress(keyMsg(tea.KeyRunes, 'h'))
	q.HandleKeyPress(keyMsg(tea.KeyRunes, 'e'))
	q.HandleKeyPress(keyMsg(tea.KeyRunes, 'r'))
	q.HandleKeyPress(keyMsg(tea.KeyRunes, 'e'))
	require.Equal(t, "hi there", q.GetTask())

	// Backspace removes last char
	q.HandleKeyPress(keyMsg(tea.KeyBackspace))
	require.Equal(t, "hi ther", q.GetTask())
}

func TestHandleKeyPress_backspaceOnEmpty(t *testing.T) {
	q := NewQuickDispatchOverlay([]string{"w1"})
	// Backspace on empty input must not panic
	close := q.HandleKeyPress(keyMsg(tea.KeyBackspace))
	require.False(t, close)
	require.Equal(t, "", q.GetTask())
}

func TestHandleKeyPress_submitWithTask(t *testing.T) {
	q := NewQuickDispatchOverlay([]string{"worker-1"})

	q.HandleKeyPress(keyMsg(tea.KeyRunes, 'd'))
	q.HandleKeyPress(keyMsg(tea.KeyRunes, 'o'))
	q.HandleKeyPress(keyMsg(tea.KeyRunes, ' '))
	q.HandleKeyPress(keyMsg(tea.KeyRunes, 'x'))

	close := q.HandleKeyPress(keyMsg(tea.KeyEnter))
	require.True(t, close)
	require.True(t, q.IsSubmitted())
	require.False(t, q.IsCanceled())
	require.Equal(t, "worker-1", q.GetWorker())
	require.Equal(t, "do x", q.GetTask())
}

func TestHandleKeyPress_submitEmptyTask(t *testing.T) {
	q := NewQuickDispatchOverlay([]string{"worker-1"})
	// Enter with no task should not close the overlay
	close := q.HandleKeyPress(keyMsg(tea.KeyEnter))
	require.False(t, close)
	require.False(t, q.IsSubmitted())
}

func TestHandleKeyPress_submitNoWorkers(t *testing.T) {
	q := NewQuickDispatchOverlay(nil)
	q.HandleKeyPress(keyMsg(tea.KeyRunes, 't'))
	// Enter with no workers should not close the overlay
	close := q.HandleKeyPress(keyMsg(tea.KeyEnter))
	require.False(t, close)
	require.False(t, q.IsSubmitted())
}

func TestHandleKeyPress_cancel(t *testing.T) {
	q := NewQuickDispatchOverlay([]string{"worker-1"})
	close := q.HandleKeyPress(keyMsg(tea.KeyEsc))
	require.True(t, close)
	require.True(t, q.IsCanceled())
	require.False(t, q.IsSubmitted())
}

func TestRender_containsExpectedSections(t *testing.T) {
	workers := []string{"worker-a", "worker-b"}
	q := NewQuickDispatchOverlay(workers)
	q.SetWidth(120)

	rendered := q.Render()

	require.True(t, strings.Contains(rendered, "Quick Dispatch"), "title missing")
	require.True(t, strings.Contains(rendered, "worker-a"), "selected worker missing")
	require.True(t, strings.Contains(rendered, "worker-b"), "unselected worker missing")
	require.True(t, strings.Contains(rendered, "[Enter]"), "footer hint missing")
	require.True(t, strings.Contains(rendered, "[Esc]"), "footer hint missing")
}

func TestRender_noWorkers(t *testing.T) {
	q := NewQuickDispatchOverlay(nil)
	q.SetWidth(120)
	rendered := q.Render()
	require.True(t, strings.Contains(rendered, "no workers available"))
}

func TestRender_taskInput(t *testing.T) {
	q := NewQuickDispatchOverlay([]string{"w1"})
	q.SetWidth(120)

	q.HandleKeyPress(keyMsg(tea.KeyRunes, 'r'))
	q.HandleKeyPress(keyMsg(tea.KeyRunes, 'u'))
	q.HandleKeyPress(keyMsg(tea.KeyRunes, 'n'))

	rendered := q.Render()
	require.True(t, strings.Contains(rendered, "run"), "typed task missing from render")
}

func TestString(t *testing.T) {
	q := NewQuickDispatchOverlay([]string{"worker-1"})
	q.HandleKeyPress(keyMsg(tea.KeyRunes, 'x'))
	s := q.String()
	require.Contains(t, s, "worker-1")
	require.Contains(t, s, "x")
}
