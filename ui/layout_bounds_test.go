package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/muesli/ansi"
	"github.com/stretchr/testify/require"
)

func assertRenderedLinesFitWidth(t *testing.T, rendered string, width int) {
	t.Helper()

	lines := strings.Split(strings.ReplaceAll(rendered, "\r", ""), "\n")
	for _, line := range lines {
		require.LessOrEqual(t, ansi.PrintableRuneWidth(line), width, "line exceeded width: %q", line)
	}
}

func TestTabbedWindowSetSizeUsesExactBounds(t *testing.T) {
	preview := NewPreviewPane()
	diff := NewDiffPane()
	terminal := NewTerminalPane()
	window := NewTabbedWindow(preview, diff, terminal)

	window.SetSize(120, 40)

	require.Equal(t, 120, window.width)

	expectedContentWidth := 120 - windowStyle.GetHorizontalFrameSize()
	require.Equal(t, expectedContentWidth, preview.width)
	require.Equal(t, expectedContentWidth, diff.width)
	require.Equal(t, expectedContentWidth, terminal.width)

	expectedContentHeight := 40 - (activeTabStyle.GetVerticalFrameSize() + 1) - windowStyle.GetVerticalFrameSize() - 2
	require.Equal(t, expectedContentHeight, preview.height)
	require.Equal(t, expectedContentHeight, diff.height)
	require.Equal(t, expectedContentHeight, terminal.height)
}

func TestListSetSizeUsesExactBounds(t *testing.T) {
	list := NewList(&spinner.Model{}, false)

	list.SetSize(120, 40)

	require.Equal(t, 120, list.width)
	require.Equal(t, 120, list.renderer.width)
}

func TestStatusBarRenderFitsWidth(t *testing.T) {
	bar := NewStatusBar()
	bar.SetWidth(28)
	bar.SetCounts(TaskCounts{
		Total:      99,
		Completed:  1,
		InProgress: 12,
		Failed:     8,
		Pending:    7,
		Blocked:    6,
		Stalled:    5,
	})

	rendered := bar.Render()
	assertRenderedLinesFitWidth(t, rendered, 28)
}

func TestMenuRenderFitsWidth(t *testing.T) {
	menu := NewMenu()
	menu.SetSize(32, 1)

	rendered := menu.String()
	assertRenderedLinesFitWidth(t, rendered, 32)
}

func TestErrBoxRenderFitsWidth(t *testing.T) {
	box := NewErrBox()
	box.SetSize(2, 1)
	box.SetError(errors.New("error: preview pane overflowed"))

	rendered := box.String()
	assertRenderedLinesFitWidth(t, rendered, 2)
}
