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

type viewportLayout struct {
	viewport       rect
	setupBanner    rect
	conflictBanner rect
	wakeBanner     rect
	content        rect
	menu           rect
	status         rect
	err            rect
}

func (l viewportLayout) totalHeight() int {
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

func computeLayout(width, height int, flags layoutFlags) viewportLayout {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}

	layout := viewportLayout{
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

type layoutRegion struct {
	Width  int
	Height int
}

type layoutSpec struct {
	viewport layoutRegion
	content  layoutRegion
	nav      layoutRegion
	main     layoutRegion
	footer   layoutRegion
	error    layoutRegion
}

func newLayoutSpec(viewportWidth, viewportHeight, bannerHeight, footerHeight int) layoutSpec {
	if viewportWidth < 0 {
		viewportWidth = 0
	}
	if viewportHeight < 0 {
		viewportHeight = 0
	}
	if bannerHeight < 0 {
		bannerHeight = 0
	}
	if footerHeight < 0 {
		footerHeight = 0
	}

	usableHeight := maxInt(viewportHeight-bannerHeight, 0)
	errorHeight := minInt(1, usableHeight)
	usableHeight -= errorHeight
	if footerHeight > usableHeight {
		footerHeight = usableHeight
	}

	contentHeight := maxInt(usableHeight-footerHeight, 0)
	navWidth := workflowNavWidth(viewportWidth)
	mainWidth := maxInt(viewportWidth-navWidth, 0)

	return layoutSpec{
		viewport: layoutRegion{Width: viewportWidth, Height: viewportHeight},
		content:  layoutRegion{Width: viewportWidth, Height: contentHeight},
		nav:      layoutRegion{Width: navWidth, Height: contentHeight},
		main:     layoutRegion{Width: mainWidth, Height: contentHeight},
		footer:   layoutRegion{Width: viewportWidth, Height: footerHeight},
		error:    layoutRegion{Width: viewportWidth, Height: errorHeight},
	}
}

func workflowNavWidth(totalWidth int) int {
	if totalWidth <= 0 {
		return 0
	}

	width := totalWidth / 5
	if width < 18 {
		width = 18
	}
	if width > 24 {
		width = 24
	}

	if remaining := totalWidth - width; remaining < 36 {
		width = maxInt(totalWidth-36, 0)
	}
	if width < 12 && totalWidth > 12 {
		width = 12
	}
	if width > totalWidth {
		width = totalWidth
	}

	return width
}

func splitMainPaneWidth(totalWidth int) (int, int) {
	if totalWidth <= 0 {
		return 0, 0
	}

	listWidth := totalWidth / 3
	if listWidth < 24 {
		listWidth = 24
	}
	if remaining := totalWidth - listWidth; remaining < 24 {
		listWidth = maxInt(totalWidth-24, 0)
	}
	if listWidth > totalWidth {
		listWidth = totalWidth
	}

	return listWidth, maxInt(totalWidth-listWidth, 0)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
