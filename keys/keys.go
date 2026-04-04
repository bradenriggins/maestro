package keys

import (
	"github.com/charmbracelet/bubbles/key"
)

type KeyName int

const (
	KeyUp KeyName = iota
	KeyDown
	KeyEnter
	KeyNew
	KeyKill
	KeyQuit
	KeySubmit

	KeyTab        // Tab is a special keybinding for switching between panes.
	KeySubmitName // SubmitName is a special keybinding for submitting the name of a new instance.

	KeyCheckout
	KeyResume
	KeyPrompt // New key for entering a prompt
	KeyHelp   // Key for showing help screen

	// Diff keybindings
	KeyShiftUp
	KeyShiftDown

	// Conductor-specific keybindings
	KeyOrchestration // o key — orchestration overlay
	KeyQuickDispatch // / key — quick dispatch
	KeyLogViewer     // l key — log viewer
	KeyPreviewToggle // f key — toggle preview mode
	KeyDiff          // d key — show diff
	KeyHistory       // h key — task history
)

// GlobalKeyStringsMap is a global, immutable map string to keybinding.
var GlobalKeyStringsMap = map[string]KeyName{
	"up":         KeyUp,
	"k":          KeyUp,
	"down":       KeyDown,
	"j":          KeyDown,
	"shift+up":   KeyShiftUp,
	"shift+down": KeyShiftDown,
	"N":          KeyPrompt,
	"enter":      KeyEnter,
	"n":          KeyNew,
	"x":          KeyKill,
	"q":          KeyQuit,
	"tab":        KeyTab,
	"p":          KeyCheckout,
	"r":          KeyResume,
	"P":          KeySubmit,
	"?":          KeyHelp,
	"o":          KeyOrchestration,
	"/":          KeyQuickDispatch,
	"l":          KeyLogViewer,
	"f":          KeyPreviewToggle,
	"d":          KeyDiff,
	"h":          KeyHistory,
}

// GlobalkeyBindings is a global, immutable map of KeyName tot keybinding.
var GlobalkeyBindings = map[KeyName]key.Binding{
	KeyUp: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	KeyDown: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	KeyShiftUp: key.NewBinding(
		key.WithKeys("shift+up"),
		key.WithHelp("shift+↑", "scroll"),
	),
	KeyShiftDown: key.NewBinding(
		key.WithKeys("shift+down"),
		key.WithHelp("shift+↓", "scroll"),
	),
	KeyEnter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("↵", "attach"),
	),
	KeyNew: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new"),
	),
	KeyKill: key.NewBinding(
		key.WithKeys("x"),
		key.WithHelp("x", "kill"),
	),
	KeyHelp: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	KeyQuit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	),
	KeySubmit: key.NewBinding(
		key.WithKeys("P"),
		key.WithHelp("P", "push branch"),
	),
	KeyPrompt: key.NewBinding(
		key.WithKeys("N"),
		key.WithHelp("N", "new with prompt"),
	),
	KeyCheckout: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "pause"),
	),
	KeyTab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch tab"),
	),
	KeyResume: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "resume"),
	),

	// -- Conductor keybindings --

	KeyOrchestration: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "orchestration"),
	),
	KeyQuickDispatch: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "dispatch"),
	),
	KeyLogViewer: key.NewBinding(
		key.WithKeys("l"),
		key.WithHelp("l", "logs"),
	),
	KeyPreviewToggle: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "preview"),
	),
	KeyDiff: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "diff"),
	),
	KeyHistory: key.NewBinding(
		key.WithKeys("h"),
		key.WithHelp("h", "history"),
	),

	// -- Special keybindings --

	KeySubmitName: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "submit name"),
	),
}
