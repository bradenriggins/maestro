package overlay

import (
	"strings"
	"testing"

	"maestro/config"

	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/require"
)

func TestComputeModalBoxUsesViewportInputs(t *testing.T) {
	box := ComputeModalBox(100, 40, 80, 24)

	require.Equal(t, 60, box.OuterWidth)
	require.Equal(t, 24, box.OuterHeight)
	require.Equal(t, 54, box.InnerWidth)
	require.Equal(t, 20, box.InnerHeight)
}

func TestTextInputOverlaySetViewportUsesRemainingHeight(t *testing.T) {
	ti := NewTextInputOverlayWithBranchPicker("Enter prompt", "", []config.Profile{
		{Name: "codex", Program: "claude"},
	})

	ti.SetViewport(100, 30)

	require.Greater(t, ti.textarea.Height(), 0)
	require.Less(t, ti.textarea.Height(), 30)
	require.Equal(t, ti.textarea.Width(), ti.branchPicker.width)
	require.Equal(t, ti.textarea.Width(), ti.profilePicker.width)
}

func TestBranchPickerRenderFitsWidth(t *testing.T) {
	bp := NewBranchPicker()
	bp.SetWidth(18)
	bp.SetResults([]string{
		"feature/this-branch-name-is-way-too-long-for-the-picker",
		"bugfix/another-overflowing-branch-name",
	}, 0)

	for _, line := range strings.Split(strings.TrimSuffix(bp.Render(), "\n"), "\n") {
		require.LessOrEqual(t, runewidth.StringWidth(line), 18, "branch picker line overflowed: %q", line)
	}
}

func TestProfilePickerRenderFitsWidth(t *testing.T) {
	pp := NewProfilePicker([]config.Profile{
		{Name: "codex/gpt-5-super-long-profile-name"},
		{Name: "claude/sonnet-extra-wide-profile"},
	})
	pp.SetWidth(20)
	pp.Focus()

	for _, line := range strings.Split(strings.TrimSuffix(pp.Render(), "\n"), "\n") {
		require.LessOrEqual(t, runewidth.StringWidth(line), 20, "profile picker line overflowed: %q", line)
	}
}

func TestPlaceOverlayInViewportReturnsViewportSizedRender(t *testing.T) {
	bg := strings.Repeat(strings.Repeat(".", 20)+"\n", 10)
	fg := "overlay"

	rendered := PlaceOverlayInViewport(20, 10, fg, bg, false)
	lines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")

	require.Len(t, lines, 10)
	for _, line := range lines {
		require.LessOrEqual(t, runewidth.StringWidth(line), 20)
	}
	require.Contains(t, rendered, fg)
}
