package app

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
