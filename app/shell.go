package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/ansi"
	"github.com/muesli/reflow/truncate"
)

func (m *home) shellBannerHeight() int {
	height := 0
	if m.setupNeeded {
		height++
	}
	if m.conflictBanner != "" {
		height++
	}
	if m.wakeBanner != "" {
		height++
	}
	return height
}

func (m *home) shellFooterHeight() int {
	height := 1
	if m.conductorConfig != nil && m.statusBar != nil {
		height++
	}
	return height
}

func (m *home) renderShellBanners() []string {
	var banners []string

	if m.setupNeeded {
		banners = append(banners, lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true).
			Padding(0, 1).
			Render("Multi-account orchestration is not configured. Run `maestro setup` to enable it."))
	}
	if m.conflictBanner != "" {
		banners = append(banners, lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Bold(true).
			Padding(0, 1).
			Render(m.conflictBanner))
	}
	if m.wakeBanner != "" {
		banners = append(banners, lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true).
			Padding(0, 1).
			Render(m.wakeBanner))
	}

	return banners
}

func (m *home) renderMainPane(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	if m.list == nil || m.tabbedWindow == nil {
		return lipgloss.Place(width, height, lipgloss.Left, lipgloss.Top, "")
	}

	if m.list.NumInstances() == 0 && m.state == stateDefault {
		emptyMsg := lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Width(width).
			Align(lipgloss.Center).
			Render("No instances yet\n\nPress 'n' to create your first instance\nPress '?' for help\nPress 'q' to quit")
		return fitBox(width, height, lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, emptyMsg))
	}

	content := lipgloss.JoinHorizontal(lipgloss.Top, m.list.String(), m.tabbedWindow.String())
	return fitBox(width, height, lipgloss.Place(width, height, lipgloss.Left, lipgloss.Top, content))
}

func (m *home) renderFooter(layout layoutSpec) string {
	if layout.footer.Width <= 0 || layout.footer.Height <= 0 {
		return ""
	}

	var parts []string
	if m.menu != nil {
		parts = append(parts, m.menu.String())
	}
	if m.conductorConfig != nil && m.statusBar != nil {
		parts = append(parts, m.statusBar.Render())
	}

	return lipgloss.Place(
		layout.footer.Width,
		layout.footer.Height,
		lipgloss.Left,
		lipgloss.Top,
		strings.Join(parts, "\n"),
	)
}

func (m *home) renderShell(layout layoutSpec, mainContent string) string {
	if m.workflowNav != nil {
		m.workflowNav.SetItems(m.workflowNavItems())
		m.workflowNav.SetSize(layout.nav.Width, layout.nav.Height)
	}

	navView := fitBox(layout.nav.Width, layout.nav.Height, "")
	if m.workflowNav != nil {
		navView = fitBox(layout.nav.Width, layout.nav.Height, m.workflowNav.Render())
	}

	mainView := fitBox(layout.main.Width, layout.main.Height, mainContent)
	body := fitBox(layout.content.Width, layout.content.Height, lipgloss.JoinHorizontal(lipgloss.Top, navView, mainView))

	footer := fitBox(layout.footer.Width, layout.footer.Height, m.renderFooter(layout))
	errorView := ""
	if m.errBox != nil {
		errorView = fitBox(layout.error.Width, layout.error.Height, m.errBox.String())
	}

	return lipgloss.JoinVertical(lipgloss.Left, body, footer, errorView)
}

func fitBox(width, height int, content string) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		if ansi.PrintableRuneWidth(line) > width {
			lines[i] = truncate.String(line, uint(width))
		}
	}

	return lipgloss.Place(width, height, lipgloss.Left, lipgloss.Top, strings.Join(lines, "\n"))
}
