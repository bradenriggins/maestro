package app

import (
	"strings"

	"github.com/muesli/ansi"
	"github.com/muesli/reflow/truncate"
)

type rect struct {
	X int
	Y int
	W int
	H int
}

type layoutFlags struct {
	setupBanner    bool
	conflictBanner bool
	wakeBanner     bool
	statusBar      bool
	errBox         bool
}

type layoutSpec struct {
	viewport       rect
	setupBanner    rect
	conflictBanner rect
	wakeBanner     rect
	content        rect
	menu           rect
	status         rect
	err            rect
}

func (l layoutSpec) totalHeight() int {
	maxBottom := 0
	for _, r := range []rect{l.setupBanner, l.conflictBanner, l.wakeBanner, l.content, l.menu, l.status, l.err} {
		if r.H == 0 {
			continue
		}
		bottom := r.Y + r.H
		if bottom > maxBottom {
			maxBottom = bottom
		}
	}
	return maxBottom
}

func computeLayout(width, height int, flags layoutFlags) layoutSpec {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}

	layout := layoutSpec{
		viewport: rect{W: width, H: height},
	}

	y := 0
	claimRow := func(enabled bool) rect {
		if !enabled || y >= height {
			return rect{}
		}
		row := rect{X: 0, Y: y, W: width, H: 1}
		y++
		return row
	}

	layout.setupBanner = claimRow(flags.setupBanner)
	layout.conflictBanner = claimRow(flags.conflictBanner)
	layout.wakeBanner = claimRow(flags.wakeBanner)

	remaining := height - y
	errHeight := 0
	if flags.errBox && remaining > 0 {
		errHeight = 1
		remaining--
	}
	statusHeight := 0
	if flags.statusBar && remaining > 0 {
		statusHeight = 1
		remaining--
	}
	menuHeight := 0
	if remaining > 0 {
		menuHeight = 1
		remaining--
	}

	contentHeight := maxInt(remaining, 0)
	layout.content = rect{X: 0, Y: y, W: width, H: contentHeight}
	y += contentHeight

	if menuHeight > 0 {
		layout.menu = rect{X: 0, Y: y, W: width, H: menuHeight}
		y += menuHeight
	}
	if statusHeight > 0 {
		layout.status = rect{X: 0, Y: y, W: width, H: statusHeight}
		y += statusHeight
	}
	if errHeight > 0 {
		layout.err = rect{X: 0, Y: y, W: width, H: errHeight}
	}

	return layout
}

func splitContentWidth(totalWidth int) (int, int) {
	if totalWidth <= 0 {
		return 0, 0
	}
	listWidth := totalWidth * 3 / 10
	if listWidth < 1 {
		listWidth = 1
	}
	if listWidth >= totalWidth {
		listWidth = totalWidth - 1
	}
	if totalWidth == 1 {
		listWidth = 1
	}
	tabsWidth := totalWidth - listWidth
	if tabsWidth < 0 {
		tabsWidth = 0
	}
	return listWidth, tabsWidth
}

func fitBlockToRect(block string, r rect) string {
	return fitBlock(block, r.W, r.H)
}

func fitBlock(block string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	rawLines := strings.Split(strings.ReplaceAll(block, "\r", ""), "\n")
	lines := make([]string, 0, height)
	for i := 0; i < height; i++ {
		line := ""
		if i < len(rawLines) {
			line = truncate.String(rawLines[i], uint(width))
		}
		renderWidth := ansi.PrintableRuneWidth(line)
		if renderWidth < width {
			line += strings.Repeat(" ", width-renderWidth)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
