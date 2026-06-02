package ui

import (
	"errors"
	"fmt"
	"maestro/log"
	"maestro/pkg/accounts"
	"maestro/session"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const readyIcon = "● "
const pausedIcon = "⏸ "

type List struct {
	items         []*session.Instance
	selectedIdx   int
	height, width int
	renderer      *InstanceRenderer
	autoyes       bool

	// map of repo name to number of instances using it. Used to display the repo name only if there are
	// multiple repos in play.
	repos map[string]int
}

func NewList(spinner *spinner.Model, autoYes bool) *List {
	return &List{
		items:    []*session.Instance{},
		renderer: &InstanceRenderer{spinner: spinner},
		repos:    make(map[string]int),
		autoyes:  autoYes,
	}
}

// SetSize sets the height and width of the list.
func (l *List) SetSize(width, height int) {
	l.width = width
	l.height = height
	l.renderer.setWidth(width)
}

// SetSessionPreviewSize sets the height and width for the tmux sessions. This makes the stdout line have the correct
// width and height.
func (l *List) SetSessionPreviewSize(width, height int) (err error) {
	for i, item := range l.items {
		if !item.Started() || item.Paused() {
			continue
		}

		if innerErr := item.SetPreviewSize(width, height); innerErr != nil {
			err = errors.Join(
				err, fmt.Errorf("could not set preview size for instance %d: %v", i, innerErr))
		}
	}
	return
}

func (l *List) NumInstances() int {
	return len(l.items)
}

// InstanceRenderer handles rendering of session.Instance objects
type InstanceRenderer struct {
	spinner *spinner.Model
	width   int
}

func (r *InstanceRenderer) setWidth(width int) {
	r.width = width
}

// ɹ and ɻ are other options.
const branchIcon = "Ꮧ"

func (r *InstanceRenderer) Render(i *session.Instance, idx int, selected bool, hasMultipleRepos bool) string {
	prefix := fmt.Sprintf(" %d. ", idx)
	if idx >= 10 {
		prefix = prefix[:len(prefix)-1]
	}
	titleS := selectedTitleStyle
	descS := selectedDescStyle
	if !selected {
		titleS = titleStyle
		descS = listDescStyle
	}

	// add spinner next to title if it's running
	var join string
	switch i.Status {
	case session.Running, session.Loading:
		join = fmt.Sprintf("%s ", r.spinner.View())
	case session.Ready:
		join = readyStyle.Render(readyIcon)
	case session.Paused:
		join = pausedStyle.Render(pausedIcon)
	default:
	}

	// Build role prefix (orchestrator gets a gold star, others get nothing).
	// Track the plain-text visual width separately from the styled string.
	var rolePrefix string
	const rolePrefixPlain = "★ "
	rolePrefixWidth := 0
	if i.Role == string(accounts.RoleOrchestrator) {
		rolePrefix = orchStyle.Render("★") + " "
		rolePrefixWidth = runewidth.StringWidth(rolePrefixPlain)
	}

	// Build account badge (dim/muted, shown after title if Account is set).
	var badge string
	badgeWidth := 0
	if i.Account != "" {
		badgePlain := fmt.Sprintf(" [%s]", i.Account)
		badgeWidth = runewidth.StringWidth(badgePlain)
		badge = accountBadgeStyle.Render(badgePlain)
	}

	// Build model badge (dim, shown after account badge if Model is set).
	var modelBadge string
	modelBadgeWidth := 0
	if i.Model != "" {
		modelBadgePlain := " " + i.Model
		modelBadgeWidth = runewidth.StringWidth(modelBadgePlain)
		modelBadgeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
		modelBadge = modelBadgeStyle.Render(modelBadgePlain)
	}

	// Cut the title if it's too long
	titleText := i.Title
	contentWidth := clampDimension(r.width - titleS.GetHorizontalFrameSize())
	statusWidth := lipgloss.Width(join)
	widthAvail := contentWidth - statusWidth - runewidth.StringWidth(prefix) - rolePrefixWidth - badgeWidth - modelBadgeWidth
	if widthAvail > 0 && runewidth.StringWidth(titleText) > widthAvail {
		titleText = runewidth.Truncate(titleText, widthAvail-3, "...")
	}
	titleInner := fmt.Sprintf("%s %s%s", prefix, rolePrefix, titleText)
	title := titleS.Render(lipgloss.Place(
		contentWidth, 1,
		lipgloss.Left, lipgloss.Center,
		lipgloss.JoinHorizontal(
			lipgloss.Left,
			lipgloss.Place(clampDimension(contentWidth-statusWidth), 1, lipgloss.Left, lipgloss.Center, titleInner+badge+modelBadge),
			join,
		)))

	stat := i.GetDiffStats()

	var diff string
	var addedDiff, removedDiff string
	if stat == nil || stat.Error != nil || stat.IsEmpty() {
		// Don't show diff stats if there's an error or if they don't exist
		addedDiff = ""
		removedDiff = ""
		diff = ""
	} else {
		addedDiff = fmt.Sprintf("+%d", stat.Added)
		removedDiff = fmt.Sprintf("-%d ", stat.Removed)
		diff = lipgloss.JoinHorizontal(
			lipgloss.Center,
			addedLinesStyle.Background(descS.GetBackground()).Render(addedDiff),
			lipgloss.Style{}.Background(descS.GetBackground()).Foreground(descS.GetForeground()).Render(","),
			removedLinesStyle.Background(descS.GetBackground()).Render(removedDiff),
		)
	}

	remainingWidth := contentWidth
	remainingWidth -= runewidth.StringWidth(prefix)
	remainingWidth -= runewidth.StringWidth(branchIcon)
	remainingWidth -= 2 // for the literal " " and "-" in the branchLine format string

	diffWidth := runewidth.StringWidth(addedDiff) + runewidth.StringWidth(removedDiff)
	if diffWidth > 0 {
		diffWidth += 1
	}

	// Use fixed width for diff stats to avoid layout issues
	remainingWidth -= diffWidth

	branch := i.Branch
	if i.Started() && hasMultipleRepos {
		repoName, err := i.RepoName()
		if err != nil {
			log.ErrorLog.Printf("could not get repo name in instance renderer: %v", err)
		} else {
			branch += fmt.Sprintf(" (%s)", repoName)
		}
	}
	// Don't show branch if there's no space for it. Or show ellipsis if it's too long.
	branchWidth := runewidth.StringWidth(branch)
	if remainingWidth < 0 {
		branch = ""
	} else if remainingWidth < branchWidth {
		if remainingWidth < 3 {
			branch = ""
		} else {
			// We know the remainingWidth is at least 4 and branch is longer than that, so this is safe.
			branch = runewidth.Truncate(branch, remainingWidth-3, "...")
		}
	}
	remainingWidth -= runewidth.StringWidth(branch)

	// Add spaces to fill the remaining width.
	spaces := ""
	if remainingWidth > 0 {
		spaces = strings.Repeat(" ", remainingWidth)
	}

	branchLine := fmt.Sprintf("%s %s-%s%s%s", strings.Repeat(" ", len(prefix)), branchIcon, branch, spaces, diff)

	// join title and subtitle
	text := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		descS.Render(lipgloss.Place(contentWidth, 1, lipgloss.Left, lipgloss.Center, branchLine)),
	)

	return lipgloss.Place(r.width, lipgloss.Height(text), lipgloss.Left, lipgloss.Top, text)
}

func (l *List) String() string {
	const titleText = " Instances "
	const autoYesText = " auto-yes "

	// Write the title.
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("\n")

	// Write title line
	titleWidth := l.width
	if !l.autoyes {
		b.WriteString(lipgloss.Place(
			titleWidth, 1, lipgloss.Left, lipgloss.Bottom, mainTitle.Render(titleText)))
	} else {
		title := lipgloss.Place(
			titleWidth/2, 1, lipgloss.Left, lipgloss.Bottom, mainTitle.Render(titleText))
		autoYes := lipgloss.Place(
			titleWidth-(titleWidth/2), 1, lipgloss.Right, lipgloss.Bottom, autoYesStyle.Render(autoYesText))
		b.WriteString(lipgloss.JoinHorizontal(
			lipgloss.Top, title, autoYes))
	}

	b.WriteString("\n")

	// Show legend above instance list (only when there are items to display).
	if len(l.items) > 0 {
		legend := lipgloss.NewStyle().Foreground(mutedTextColor).Render("★ orchestrator  · worker  (model shown after account)")
		b.WriteString("  ")
		b.WriteString(legend)
	}

	b.WriteString("\n")

	// Render the list.
	for i, item := range l.items {
		b.WriteString(l.renderer.Render(item, i+1, i == l.selectedIdx, len(l.repos) > 1))
		if i != len(l.items)-1 {
			b.WriteString("\n\n")
		}
	}
	return lipgloss.Place(l.width, l.height, lipgloss.Left, lipgloss.Top, b.String())
}

// Down selects the next item in the list.
func (l *List) Down() {
	if len(l.items) == 0 {
		return
	}
	if l.selectedIdx < len(l.items)-1 {
		l.selectedIdx++
	}
}

// Kill removes the selected instance from the list (in-memory only).
// The caller is responsible for terminating the underlying tmux session
// before calling this method (e.g. via instance.Kill() inside a tea.Cmd).
func (l *List) Kill() {
	if len(l.items) == 0 {
		return
	}
	if l.selectedIdx < 0 || l.selectedIdx >= len(l.items) {
		return
	}
	targetInstance := l.items[l.selectedIdx]

	// If you delete the last one in the list, select the previous one.
	if l.selectedIdx == len(l.items)-1 {
		defer l.Up()
	}

	// Unregister the reponame.
	repoName, err := targetInstance.RepoName()
	if err != nil {
		log.ErrorLog.Printf("could not get repo name: %v", err)
	} else {
		l.rmRepo(repoName)
	}

	// Since there's items after this, the selectedIdx can stay the same.
	l.items = append(l.items[:l.selectedIdx], l.items[l.selectedIdx+1:]...)
}

// RemoveByName removes the instance with the given name from the list (in-memory only).
// No subprocess I/O is performed. If no instance with that name is found, this is a no-op.
func (l *List) RemoveByName(name string) {
	for i, item := range l.items {
		if item.Title == name {
			// Unregister the reponame.
			repoName, err := item.RepoName()
			if err != nil {
				log.ErrorLog.Printf("could not get repo name: %v", err)
			} else {
				l.rmRepo(repoName)
			}
			l.items = append(l.items[:i], l.items[i+1:]...)
			// Keep selectedIdx pointing at a valid item. Removing an item
			// before the selection shifts everything down by one, so the
			// selection must follow; then clamp to the new bounds.
			if i < l.selectedIdx {
				l.selectedIdx--
			}
			if l.selectedIdx >= len(l.items) {
				l.selectedIdx = len(l.items) - 1
			}
			if l.selectedIdx < 0 {
				l.selectedIdx = 0
			}
			return
		}
	}
}

func (l *List) Attach() (chan struct{}, error) {
	if len(l.items) == 0 {
		return nil, fmt.Errorf("no instances to attach to")
	}
	if l.selectedIdx < 0 || l.selectedIdx >= len(l.items) {
		return nil, fmt.Errorf("selected index %d is out of range", l.selectedIdx)
	}
	targetInstance := l.items[l.selectedIdx]
	return targetInstance.Attach()
}

// Up selects the prev item in the list.
func (l *List) Up() {
	if len(l.items) == 0 {
		return
	}
	if l.selectedIdx > 0 {
		l.selectedIdx--
	}
}

func (l *List) addRepo(repo string) {
	if _, ok := l.repos[repo]; !ok {
		l.repos[repo] = 0
	}
	l.repos[repo]++
}

func (l *List) rmRepo(repo string) {
	if _, ok := l.repos[repo]; !ok {
		log.ErrorLog.Printf("repo %s not found", repo)
		return
	}
	l.repos[repo]--
	if l.repos[repo] == 0 {
		delete(l.repos, repo)
	}
}

// AddInstance adds a new instance to the list. It returns a finalizer function that should be called when the instance
// is started. If the instance was restored from storage or is paused, you can call the finalizer immediately.
// When creating a new one and entering the name, you want to call the finalizer once the name is done.
func (l *List) AddInstance(instance *session.Instance) (finalize func()) {
	l.items = append(l.items, instance)
	// The finalizer registers the repo name once the instance is started.
	return func() {
		repoName, err := instance.RepoName()
		if err != nil {
			log.ErrorLog.Printf("could not get repo name: %v", err)
			return
		}

		l.addRepo(repoName)
	}
}

// GetSelectedInstance returns the currently selected instance
func (l *List) GetSelectedInstance() *session.Instance {
	if len(l.items) == 0 {
		return nil
	}
	// Defensive clamp: callers fire this constantly (every render/tick), and a
	// momentarily stale selectedIdx (e.g. after an async removal) must never
	// index out of range and panic the TUI.
	if l.selectedIdx >= len(l.items) {
		l.selectedIdx = len(l.items) - 1
	}
	if l.selectedIdx < 0 {
		l.selectedIdx = 0
	}
	return l.items[l.selectedIdx]
}

// SetSelectedInstance sets the selected index. Noop if the index is out of bounds.
func (l *List) SetSelectedInstance(idx int) {
	if idx < 0 || idx >= len(l.items) {
		return
	}
	l.selectedIdx = idx
}

// SelectInstance finds and selects the given instance in the list.
func (l *List) SelectInstance(target *session.Instance) {
	for i, inst := range l.items {
		if inst == target {
			l.SetSelectedInstance(i)
			return
		}
	}
}

// GetInstances returns all instances in the list
func (l *List) GetInstances() []*session.Instance {
	return l.items
}
