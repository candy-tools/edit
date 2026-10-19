// Package tui is the Bubble Tea frontend of the editor: a file sidebar on the
// left and the open files as tabs in an editor pane on the right, framed by a
// header and a key-hint footer. The sidebar browses the workspace as a folder
// tree, and enter opens a file, read-only for now, in a tab of the editor
// pane. Keys are looked up in a keymap, which the user can rebind in the
// config file.
package tui

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/candy-tools/edit/app/metainfo"
	"github.com/candy-tools/edit/libs/keymap"
	"github.com/candy-tools/edit/libs/tabset"
	"github.com/candy-tools/edit/libs/treeview"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// pane is one of the focusable areas of the screen.
type pane int

const (
	paneSidebar pane = iota
	paneEditor
)

// scope is the keymap scope whose actions work while p has focus, next to
// the global ones.
func (p pane) scope() string {
	if p == paneEditor {
		return scopeEditor
	}
	return scopeSidebar
}

// defaultWidth and defaultHeight are the terminal size assumed until the
// first tea.WindowSizeMsg arrives.
const defaultWidth, defaultHeight = 80, 24

type model struct {
	root    string // the workspace directory, shown in the header
	version string
	width   int
	height  int

	focus pane                           // the pane that receives keys, drawn with an accent border
	keys  *keymap.Map                    // which action each key triggers
	fsys  fs.FS                          // the workspace, read when a file opens
	files *treeview.Tree[entry]          // the sidebar's file tree over the workspace
	tabs  *tabset.Set[string, *document] // the open files, keyed by path
}

// Run starts the interactive editor on the workspace directory root, with
// keys bound as keys says.
func Run(root string, keys *keymap.Map) error {
	_, err := tea.NewProgram(newModel(root, os.DirFS(root), keys), tea.WithAltScreen()).Run()
	return err
}

// newModel builds the start-up state: the sidebar focused on the file tree of
// fsys — the workspace at root — and no tab open, with keys bound as keys
// says. Split out from Run so tests can drive the model on an in-memory
// filesystem without starting a real terminal program.
func newModel(root string, fsys fs.FS, keys *keymap.Map) model {
	files := treeview.New(workspace{fsys}, rootEntry)
	files.SetHeight(paneHeight(defaultHeight))
	tabs := tabset.New(measureTabs)
	tabs.SetWidth(stripWidth(defaultWidth))
	return model{
		root:    root,
		version: metainfo.Version,
		focus:   paneSidebar,
		keys:    keys,
		fsys:    fsys,
		files:   files,
		tabs:    tabs,
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.files.SetHeight(paneHeight(msg.Height))
		tabs, _ := m.tabs.Tabs()
		for _, t := range tabs {
			t.Value.setHeight(paneHeight(msg.Height))
		}
		m.tabs.SetWidth(stripWidth(msg.Width))
	case tea.KeyMsg:
		// A key held down, or keys typed fast, can arrive as one message:
		// each press acts in turn, in the pane that has focus by then.
		for _, k := range keymap.Presses(msg) {
			var cmd tea.Cmd
			if m, cmd = m.keyPress(k); cmd != nil {
				return m, cmd
			}
		}
	}
	return m, nil
}

// keyPress applies one key press: the action it triggers in the focused
// pane, if any. Only quit returns a command.
func (m model) keyPress(msg tea.KeyMsg) (model, tea.Cmd) {
	id, ok := m.keys.Lookup(m.focus.scope(), msg)
	if !ok {
		return m, nil
	}
	switch id {
	case actQuit:
		return m, tea.Quit
	case actSwitchFocus:
		if m.focus == paneSidebar {
			m.focus = paneEditor
		} else {
			m.focus = paneSidebar
		}
	case actNextTab:
		m.tabs.Next()
	case actPreviousTab:
		m.tabs.Prev()
	case actCloseTab:
		m.tabs.Close()
		if m.tabs.Len() == 0 {
			m.focus = paneSidebar
		}
	default:
		m = m.paneAction(id)
	}
	return m, nil
}

// paneAction applies an action of the focused pane: sidebar.open on a file
// opens it, other sidebar actions drive the file tree, and editor actions
// scroll the active tab's text.
func (m model) paneAction(id keymap.ID) model {
	if m.focus == paneEditor {
		editorAction(m.activeDocument(), id)
		return m
	}
	if id == sidebarOpen {
		if row, ok := m.files.Selected(); ok && row.Err == nil && !row.Branch {
			return m.open(row.Node)
		}
	}
	sidebarAction(m.files, id)
	return m
}

// open opens the file e in a new tab, or switches to its tab when it is
// already open, without reading it again, and moves focus to the editor.
func (m model) open(e entry) model {
	m.tabs.Open(e.path, func() *document {
		doc := loadDocument(m.fsys, e.path)
		_, h := m.size()
		doc.setHeight(paneHeight(h))
		return doc
	})
	m.focus = paneEditor
	return m
}

// activeDocument returns the active tab's document, or nil when no tab is
// open.
func (m model) activeDocument() *document {
	t, ok := m.tabs.Active()
	if !ok {
		return nil
	}
	return t.Value
}

// size is the terminal's width and height, or the 80x24 default before the
// first tea.WindowSizeMsg.
func (m model) size() (w, h int) {
	if m.width == 0 {
		return defaultWidth, defaultHeight
	}
	return m.width, m.height
}

// View composes the header, the sidebar and editor panes side by side, and the
// footer to the terminal size.
func (m model) View() string {
	w, h := m.size()
	bodyH := bodyHeight(h)
	sw := sidebarWidth(w)
	sidebar := titledBox("Files", m.sidebarBody(), sw, bodyH, m.focus == paneSidebar)
	body := editorBody(m.activeDocument(), w-sw-2)
	editor := frame(tabStrip(m.tabs, m.focus == paneEditor), body, w-sw, bodyH, m.focus == paneEditor)
	return m.header(w) + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, sidebar, editor) + "\n" + m.footer(w)
}

// bodyHeight is the height of the sidebar and editor boxes in a terminal h
// rows tall: all of it but the header and footer lines, and at least 3.
func bodyHeight(h int) int {
	return max(h-2, 3)
}

// paneHeight is how many rows fit inside the sidebar and editor boxes in a
// terminal h rows tall: the boxes' height less their top and bottom borders.
// It is the height of the file tree's window and of every tab's text window.
func paneHeight(h int) int {
	return bodyHeight(h) - 2
}

// sidebarWidth is the file sidebar's width at terminal width w: 30 columns,
// but never more than a third of the screen, so a narrow terminal keeps most
// of its width for the editor.
func sidebarWidth(w int) int {
	return min(30, w/3)
}

func (m model) header(width int) string {
	left := headerAppStyle.Render("edit") + " " + headerHintSty.Render(m.version) + "  " + headerHintSty.Render(tildePath(m.root))
	return ansi.Truncate(" "+left, width, "")
}

// footer lists the key hints: the file tree's while the sidebar has focus,
// the scroll and tab keys while the editor has focus and a tab is open, then
// the global ones. Each hint shows the keys its actions are bound to, and a
// hint whose actions have no keys is left out.
func (m model) footer(width int) string {
	var parts []string
	add := func(label string, ids ...keymap.ID) {
		if keys := m.keys.Label(ids...); keys != "" {
			parts = append(parts, hint(keys, label))
		}
	}
	switch {
	case m.focus == paneSidebar:
		add("Move", sidebarUp, sidebarDown)
		add("Fold", sidebarCollapse, sidebarExpand)
		add("Open", sidebarOpen)
	case m.tabs.Len() > 0:
		add("Scroll", editorUp, editorDown)
		add("Tabs", actPreviousTab, actNextTab)
		add("Close", actCloseTab)
	}
	add("Focus", actSwitchFocus)
	add("Quit", actQuit)
	return ansi.Truncate(" "+strings.Join(parts, "  "), width, "")
}

// tildePath abbreviates the user's home directory at the start of path to "~",
// the way a shell prompt shows it.
func tildePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return filepath.Join("~", rest)
	}
	return path
}
