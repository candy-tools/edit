package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/candy-tools/edit/libs/keymap"
)

// The scopes of edit's actions, next to keymap.Global: the pane with focus
// decides which of them is active, or the search popup, which takes every
// key while it is open. Keys type text in the editor and in the popup's
// query.
const (
	scopeSidebar = "sidebar"
	scopeEditor  = "editor"
	scopeSearch  = "search"
)

// edit's actions: what a key can do, named as the config file names them.
var (
	actQuit        = keymap.ID{Scope: keymap.Global, Name: "quit"}
	actSwitchFocus = keymap.ID{Scope: keymap.Global, Name: "switch-focus"}
	actNextTab     = keymap.ID{Scope: keymap.Global, Name: "next-tab"}
	actPreviousTab = keymap.ID{Scope: keymap.Global, Name: "previous-tab"}
	actCloseTab    = keymap.ID{Scope: keymap.Global, Name: "close-tab"}

	sidebarUp       = keymap.ID{Scope: scopeSidebar, Name: "up"}
	sidebarDown     = keymap.ID{Scope: scopeSidebar, Name: "down"}
	sidebarPageUp   = keymap.ID{Scope: scopeSidebar, Name: "page-up"}
	sidebarPageDown = keymap.ID{Scope: scopeSidebar, Name: "page-down"}
	sidebarTop      = keymap.ID{Scope: scopeSidebar, Name: "top"}
	sidebarBottom   = keymap.ID{Scope: scopeSidebar, Name: "bottom"}
	sidebarExpand   = keymap.ID{Scope: scopeSidebar, Name: "expand"}
	sidebarCollapse = keymap.ID{Scope: scopeSidebar, Name: "collapse"}
	sidebarOpen     = keymap.ID{Scope: scopeSidebar, Name: "open"}
	sidebarSearch   = keymap.ID{Scope: scopeSidebar, Name: "search"}

	editorLeft        = keymap.ID{Scope: scopeEditor, Name: "left"}
	editorRight       = keymap.ID{Scope: scopeEditor, Name: "right"}
	editorUp          = keymap.ID{Scope: scopeEditor, Name: "up"}
	editorDown        = keymap.ID{Scope: scopeEditor, Name: "down"}
	editorLineStart   = keymap.ID{Scope: scopeEditor, Name: "line-start"}
	editorLineEnd     = keymap.ID{Scope: scopeEditor, Name: "line-end"}
	editorPageUp      = keymap.ID{Scope: scopeEditor, Name: "page-up"}
	editorPageDown    = keymap.ID{Scope: scopeEditor, Name: "page-down"}
	editorTop         = keymap.ID{Scope: scopeEditor, Name: "top"}
	editorBottom      = keymap.ID{Scope: scopeEditor, Name: "bottom"}
	editorNewline     = keymap.ID{Scope: scopeEditor, Name: "newline"}
	editorInsertTab   = keymap.ID{Scope: scopeEditor, Name: "insert-tab"}
	editorDeleteLeft  = keymap.ID{Scope: scopeEditor, Name: "delete-left"}
	editorDeleteRight = keymap.ID{Scope: scopeEditor, Name: "delete-right"}
	editorDeleteLine  = keymap.ID{Scope: scopeEditor, Name: "delete-line"}
	editorUndo        = keymap.ID{Scope: scopeEditor, Name: "undo"}
	editorRedo        = keymap.ID{Scope: scopeEditor, Name: "redo"}
	editorSearch      = keymap.ID{Scope: scopeEditor, Name: "search"}

	searchUp          = keymap.ID{Scope: scopeSearch, Name: "up"}
	searchDown        = keymap.ID{Scope: scopeSearch, Name: "down"}
	searchPageUp      = keymap.ID{Scope: scopeSearch, Name: "page-up"}
	searchPageDown    = keymap.ID{Scope: scopeSearch, Name: "page-down"}
	searchLeft        = keymap.ID{Scope: scopeSearch, Name: "left"}
	searchRight       = keymap.ID{Scope: scopeSearch, Name: "right"}
	searchLineStart   = keymap.ID{Scope: scopeSearch, Name: "line-start"}
	searchLineEnd     = keymap.ID{Scope: scopeSearch, Name: "line-end"}
	searchDeleteLeft  = keymap.ID{Scope: scopeSearch, Name: "delete-left"}
	searchDeleteRight = keymap.ID{Scope: scopeSearch, Name: "delete-right"}
	searchOpen        = keymap.ID{Scope: scopeSearch, Name: "open"}
	searchClose       = keymap.ID{Scope: scopeSearch, Name: "close"}
)

// actions are edit's actions with their default keys. quit must keep a key,
// so the config can't lock you in. A double esc closes a tab too. ctrl+h
// deletes left too, since some terminals send it for backspace.
// ctrl+shift+z only reaches edit from a terminal that reports it; where it
// arrives as ctrl+z, it undoes, so redo has ctrl+y too. The search popup
// closes on esc, which esc esc doesn't take there, its scope being modal, and
// on ctrl+c, as quit's ctrl+c waits for it to close; search.close must keep a
// key, as quit must, so the popup can always be closed.
var actions = []keymap.Action{
	{ID: actQuit, Keys: []string{"ctrl+q", "ctrl+c"}, Required: true},
	{ID: actSwitchFocus, Keys: []string{"shift+tab"}},
	{ID: actNextTab, Keys: []string{"ctrl+pgdown"}},
	{ID: actPreviousTab, Keys: []string{"ctrl+pgup"}},
	{ID: actCloseTab, Keys: []string{"ctrl+w", "esc esc"}},

	{ID: sidebarUp, Keys: []string{"up"}},
	{ID: sidebarDown, Keys: []string{"down"}},
	{ID: sidebarPageUp, Keys: []string{"pgup"}},
	{ID: sidebarPageDown, Keys: []string{"pgdown"}},
	{ID: sidebarTop, Keys: []string{"home"}},
	{ID: sidebarBottom, Keys: []string{"end"}},
	{ID: sidebarExpand, Keys: []string{"right"}},
	{ID: sidebarCollapse, Keys: []string{"left"}},
	{ID: sidebarOpen, Keys: []string{"enter"}},
	{ID: sidebarSearch, Keys: []string{"ctrl+f"}},

	{ID: editorLeft, Keys: []string{"left"}},
	{ID: editorRight, Keys: []string{"right"}},
	{ID: editorUp, Keys: []string{"up"}},
	{ID: editorDown, Keys: []string{"down"}},
	{ID: editorLineStart, Keys: []string{"home"}},
	{ID: editorLineEnd, Keys: []string{"end"}},
	{ID: editorPageUp, Keys: []string{"pgup"}},
	{ID: editorPageDown, Keys: []string{"pgdown"}},
	{ID: editorTop, Keys: []string{"ctrl+home"}},
	{ID: editorBottom, Keys: []string{"ctrl+end"}},
	{ID: editorNewline, Keys: []string{"enter"}},
	{ID: editorInsertTab, Keys: []string{"tab"}},
	{ID: editorDeleteLeft, Keys: []string{"backspace", "ctrl+h"}},
	{ID: editorDeleteRight, Keys: []string{"delete"}},
	{ID: editorDeleteLine, Keys: []string{"ctrl+k"}},
	{ID: editorUndo, Keys: []string{"ctrl+z"}},
	{ID: editorRedo, Keys: []string{"ctrl+shift+z", "ctrl+y"}},
	{ID: editorSearch, Keys: []string{"ctrl+f"}},

	{ID: searchUp, Keys: []string{"up"}},
	{ID: searchDown, Keys: []string{"down"}},
	{ID: searchPageUp, Keys: []string{"pgup"}},
	{ID: searchPageDown, Keys: []string{"pgdown"}},
	{ID: searchLeft, Keys: []string{"left"}},
	{ID: searchRight, Keys: []string{"right"}},
	{ID: searchLineStart, Keys: []string{"home"}},
	{ID: searchLineEnd, Keys: []string{"end"}},
	{ID: searchDeleteLeft, Keys: []string{"backspace", "ctrl+h"}},
	{ID: searchDeleteRight, Keys: []string{"delete"}},
	{ID: searchOpen, Keys: []string{"enter"}},
	{ID: searchClose, Keys: []string{"esc", "ctrl+c"}, Required: true},
}

// sequenceTimeout is how long a sequence waits for its next key, as long as
// a double-click may take: the second esc of a double esc must come within
// it.
const sequenceTimeout = 500 * time.Millisecond

// sequenceTimeoutMsg ends the wait for a sequence's next key that key press
// press started.
type sequenceTimeoutMsg struct{ press int }

// sequenceTimer returns the command that delivers press's
// sequenceTimeoutMsg, sequenceTimeout from now.
func sequenceTimer(press int) tea.Cmd {
	return tea.Tick(sequenceTimeout, func(time.Time) tea.Msg { return sequenceTimeoutMsg{press} })
}

// NewKeymap builds edit's keymap: each action's default keys, except that
// an action overrides lists (scope → action name → keys) gets those keys
// instead. Keys type text in the editor and in the search popup's query, so
// a key that types text can't be bound there, nor in the global scope,
// whose actions work in the editor too. The popup's scope is modal: the
// global actions don't work while it is open, so its keys, esc among them,
// don't clash with theirs. Its error lists every problem with overrides,
// one per line.
func NewKeymap(overrides map[string]map[string][]string) (*keymap.Map, error) {
	return keymap.New(actions, overrides,
		keymap.Scope{Name: scopeEditor, Typing: true},
		keymap.Scope{Name: scopeSearch, Typing: true, Modal: true})
}
