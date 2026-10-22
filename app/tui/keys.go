package tui

import "github.com/candy-tools/edit/libs/keymap"

// The scopes of edit's actions, next to keymap.Global: the pane with focus
// decides which of them is active. Keys type text in the editor.
const (
	scopeSidebar = "sidebar"
	scopeEditor  = "editor"
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
)

// actions are edit's actions with their default keys. quit must keep a key,
// so the config can't lock you in. ctrl+h deletes left too, since some
// terminals send it for backspace.
var actions = []keymap.Action{
	{ID: actQuit, Keys: []string{"ctrl+q", "ctrl+c"}, Required: true},
	{ID: actSwitchFocus, Keys: []string{"shift+tab"}},
	{ID: actNextTab, Keys: []string{"ctrl+pgdown"}},
	{ID: actPreviousTab, Keys: []string{"ctrl+pgup"}},
	{ID: actCloseTab, Keys: []string{"ctrl+w"}},

	{ID: sidebarUp, Keys: []string{"up"}},
	{ID: sidebarDown, Keys: []string{"down"}},
	{ID: sidebarPageUp, Keys: []string{"pgup"}},
	{ID: sidebarPageDown, Keys: []string{"pgdown"}},
	{ID: sidebarTop, Keys: []string{"home"}},
	{ID: sidebarBottom, Keys: []string{"end"}},
	{ID: sidebarExpand, Keys: []string{"right"}},
	{ID: sidebarCollapse, Keys: []string{"left"}},
	{ID: sidebarOpen, Keys: []string{"enter"}},

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
}

// NewKeymap builds edit's keymap: each action's default keys, except that
// an action overrides lists (scope → action name → keys) gets those keys
// instead. Keys type text in the editor, so a key that types text can't be
// bound there, nor in the global scope, whose actions work in the editor
// too. Its error lists every problem with overrides, one per line.
func NewKeymap(overrides map[string]map[string][]string) (*keymap.Map, error) {
	return keymap.New(actions, overrides, scopeEditor)
}
