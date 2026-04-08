package ui

import (
	"maestro/keys"
	"maestro/session"
)

// MenuState represents different footer contexts for the shell action bar.
type MenuState int

const (
	StateDefault MenuState = iota
	StateEmpty
	StateNewInstance
	StatePrompt
)

type Menu struct {
	height, width int
	state         MenuState
	instance      *session.Instance
	activeTab     int
	summary       string
	actionBar     *ActionBar

	// keyDown is the key currently highlighted in the footer.
	keyDown keys.KeyName
}

func NewMenu() *Menu {
	return &Menu{
		state:     StateEmpty,
		activeTab: 0,
		keyDown:   -1,
		actionBar: NewActionBar(),
	}
}

func (m *Menu) Keydown(name keys.KeyName) {
	m.keyDown = name
}

func (m *Menu) ClearKeydown() {
	m.keyDown = -1
}

func (m *Menu) SetState(state MenuState) {
	m.state = state
}

func (m *Menu) SetInstance(instance *session.Instance) {
	m.instance = instance
	if m.state != StateNewInstance && m.state != StatePrompt {
		if instance != nil {
			m.state = StateDefault
		} else {
			m.state = StateEmpty
		}
	}
}

func (m *Menu) SetActiveTab(tab int) {
	m.activeTab = tab
}

func (m *Menu) SetSummary(summary string) {
	m.summary = summary
}

func (m *Menu) SetSize(width, height int) {
	m.width = width
	m.height = height
	if m.actionBar != nil {
		m.actionBar.SetSize(width, height)
	}
}

func (m *Menu) String() string {
	if m.actionBar == nil {
		m.actionBar = NewActionBar()
	}
	m.actionBar.SetSize(m.width, m.height)
	return m.actionBar.Render(m.actions(), m.summary, m.keyDown)
}

func (m *Menu) actions() []ActionBarAction {
	switch m.state {
	case StateNewInstance:
		return []ActionBarAction{{Key: keys.KeySubmitName, Label: "submit name"}}
	case StatePrompt:
		return []ActionBarAction{{Key: keys.KeySubmitName, Label: "submit"}}
	case StateDefault:
		if m.instance != nil {
			return m.instanceActions()
		}
	}

	return []ActionBarAction{
		{Key: keys.KeyNew, Label: "new"},
		{Key: keys.KeyPrompt, Label: "prompt"},
		{Key: keys.KeyQuickDispatch, Label: "dispatch"},
		{Key: keys.KeyHelp, Label: "system"},
	}
}

func (m *Menu) instanceActions() []ActionBarAction {
	actions := []ActionBarAction{
		{Key: keys.KeyEnter, Label: "open"},
	}

	if m.instance != nil && m.instance.Status != session.Loading {
		if m.instance.Status == session.Paused {
			actions = append(actions, ActionBarAction{Key: keys.KeyResume, Label: "resume"})
		} else {
			actions = append(actions, ActionBarAction{Key: keys.KeyCheckout, Label: "pause"})
		}
	}

	actions = append(actions, ActionBarAction{Key: keys.KeyTab, Label: "switch"})

	if m.activeTab == DiffTab || m.activeTab == TerminalTab {
		actions = append(actions, ActionBarAction{Key: keys.KeyShiftUp, Label: "scroll"})
	}

	actions = append(actions,
		ActionBarAction{Key: keys.KeyQuickDispatch, Label: "dispatch"},
		ActionBarAction{Key: keys.KeyHistory, Label: "history"},
		ActionBarAction{Key: keys.KeyHelp, Label: "system"},
	)

	return actions
}
