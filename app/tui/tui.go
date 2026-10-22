// Package tui is the Bubble Tea frontend of the editor: a file sidebar on the
// left and the open files as tabs in an editor pane on the right, framed by a
// header and a key-hint footer. The sidebar browses the workspace as a folder
// tree, and enter opens a file in a tab of the editor pane, where it can be
// edited, its text colored by its syntax; edits are saved in the background,
// and when a tab closes or edit quits. Keys are looked up in a keymap, which
// the user can rebind in the config file, as they can pick the theme.
package tui

import (
	"errors"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

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
	theme Theme                          // the colors of the text, by its syntax
	fsys  fs.FS                          // the workspace, read when a file opens
	files *treeview.Tree[entry]          // the sidebar's file tree over the workspace
	tabs  *tabset.Set[string, *document] // the open files, keyed by path

	write   writeFunc      // writes the open files back to the workspace
	tick    func() tea.Cmd // makes the command that delivers the next saveTickMsg
	ticking bool           // a saveTickMsg is on its way
	confirm keymap.ID      // the close or quit whose save failed, waiting to be pressed again; zero for none
}

// Run starts the interactive editor on the workspace directory root, with
// keys bound as keys says and text colored in theme, and returns once it has
// quit and saved every edit it could.
func Run(root string, keys *keymap.Map, theme Theme) error {
	// edit catches SIGHUP, SIGINT and SIGTERM itself, until it is done, rather
	// than leave them to Bubble Tea: its handler can block for good on a
	// signal that arrives as the program ends, and a second signal during
	// the final save, such as the shell's SIGHUP after the terminal's, must
	// not kill edit between truncating a file and writing it.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)
	g := &gate{write: diskWriter(root)}
	defer g.close()
	return run(newModel(root, os.DirFS(root), g.pass, keys, theme), sigs, tea.WithAltScreen(), tea.WithoutSignalHandler())
}

// run runs a program on m until it quits, turning each signal from sigs into
// a quit, then saves what the final model left unsaved. A quit gives up once
// the program has ended, so a signal can't hold run up however late it
// comes. Split out from Run so tests can drive a real program with
// tea.WithInput and tea.WithOutput.
func run(m model, sigs <-chan os.Signal, opts ...tea.ProgramOption) error {
	p := tea.NewProgram(m, opts...)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-sigs:
				p.Quit()
			case <-done:
				return
			}
		}
	}()
	final, err := p.Run()
	if fm, ok := final.(model); ok { // there is none after a panic
		err = errors.Join(err, fm.saveAll())
	}
	return err
}

// newModel builds the start-up state: the sidebar focused on the file tree of
// fsys — the workspace at root — and no tab open, with keys bound as keys
// says, text colored in theme and the open files written back with write.
// Split out from Run so tests can drive the model on an in-memory
// filesystem without starting a real terminal program.
func newModel(root string, fsys fs.FS, write writeFunc, keys *keymap.Map, theme Theme) model {
	files := treeview.New(workspace{fsys}, rootEntry)
	files.SetHeight(paneHeight(defaultHeight))
	tabs := tabset.New(measureTabs)
	tabs.SetWidth(stripWidth(defaultWidth))
	return model{
		root:    root,
		version: metainfo.Version,
		focus:   paneSidebar,
		keys:    keys,
		theme:   theme,
		fsys:    fsys,
		files:   files,
		tabs:    tabs,
		write:   write,
		tick:    saveTick,
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.files.SetHeight(paneHeight(msg.Height))
		tabs, _ := m.tabs.Tabs()
		for _, t := range tabs {
			m.fit(t.Value)
		}
		m.tabs.SetWidth(stripWidth(msg.Width))
	case tea.KeyMsg:
		// A key held down, or keys typed fast, can arrive as one message:
		// each press acts in turn, in the pane that has focus by then. Only
		// quitting cuts the run short.
		for _, k := range keymap.Presses(msg) {
			var quit tea.Cmd
			if m, quit = m.keyPress(k); quit != nil {
				return m, quit
			}
		}
	case saveTickMsg:
		m, cmd = m.startWrites()
	case savedMsg:
		m = m.saved(msg)
	case parsedMsg:
		// Open tab or not: a closed tab's Highlighter frees the tree.
		msg.doc.syntax.Done(msg.result)
	}
	// Writes and their results can add or take away a tab title's ⚠.
	m.tabs.Remeasure()
	m, tick := m.scheduleSave()
	return m, tea.Batch(cmd, tick, m.parses())
}

// keyPress applies one key press: the action it triggers in the focused
// pane or, in the editor, the text it types when it triggers none. Only quit
// returns a command. Any press but the same close or quit again cancels a
// close or quit whose save failed.
func (m model) keyPress(msg tea.KeyMsg) (model, tea.Cmd) {
	confirm := m.confirm
	m.confirm = keymap.ID{}
	id, ok := m.keys.Lookup(m.focus.scope(), msg)
	if !ok {
		if text, ok := keymap.Typed(msg); ok && m.focus == paneEditor {
			doc := m.activeDocument()
			typeText(doc, text)
			m.fit(doc)
		}
		return m, nil
	}
	switch id {
	case actQuit:
		return m.quit(confirm == actQuit)
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
		m = m.closeTab(confirm == actCloseTab)
	default:
		m = m.paneAction(id)
	}
	return m, nil
}

// quit saves every tab and quits. When a tab can't be saved, edit stays and
// the footer asks for quit again, which quits anyway: anyway says this is
// that second press, which leaves the last try to the final save.
func (m model) quit(anyway bool) (model, tea.Cmd) {
	if anyway || m.flushAll() {
		return m, tea.Quit
	}
	m.confirm = actQuit
	return m, nil
}

// closeTab saves the active tab and closes it. When it can't be saved, the
// tab stays and the footer asks for close again, which closes it anyway:
// anyway says this is that second press. Every close retires the tab's file,
// so no write queued before the close can land after it.
func (m model) closeTab(anyway bool) model {
	doc := m.activeDocument()
	if doc == nil {
		return m
	}
	if err := doc.flush(); err != nil && !anyway {
		m.confirm = actCloseTab
		return m
	}
	doc.retire()
	m.tabs.Close()
	if m.tabs.Len() == 0 {
		m.focus = paneSidebar
	}
	return m
}

// paneAction applies an action of the focused pane: sidebar.open on a file
// opens it, other sidebar actions drive the file tree, and editor actions
// move the cursor in the active tab's text or edit it.
func (m model) paneAction(id keymap.ID) model {
	if m.focus == paneEditor {
		doc := m.activeDocument()
		editorAction(doc, id)
		m.fit(doc)
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
// already open, under its own path or another that names the same file,
// such as a symlink or a hard link, without reading it again; then it moves
// focus to the editor. A file never gets two tabs, whose saves would
// overwrite each other's.
func (m model) open(e entry) model {
	m.tabs.Open(m.tabKey(e.path), func() *document {
		doc := loadDocument(m.fsys, m.write, e.path)
		doc.highlight(m.theme)
		m.fit(doc)
		return doc
	})
	m.focus = paneEditor
	return m
}

// tabKey returns the key of the open tab that holds the file at path, under
// path or another name for the same file, or path itself when no tab does.
func (m model) tabKey(path string) string {
	fi, err := fs.Stat(m.fsys, path)
	if err != nil {
		return path
	}
	tabs, _ := m.tabs.Tabs()
	for _, t := range tabs {
		if t.Key == path {
			return path
		}
		if other, err := fs.Stat(m.fsys, t.Key); err == nil && os.SameFile(fi, other) {
			return t.Key
		}
	}
	return path
}

// fit sizes the text window of doc to the editor pane's body: whenever the
// terminal is resized, a tab opens, or an editor key is pressed, since an
// edit can give the line numbers another digit. With no tab open (doc is
// nil) it does nothing.
func (m model) fit(doc *document) {
	if doc != nil {
		w, h := m.size()
		doc.setSize(editorWidth(w), paneHeight(h))
	}
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
	body := editorBody(m.activeDocument(), m.focus == paneEditor, m.theme)
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

// editorWidth is the width of the editor pane's body at terminal width w:
// what the sidebar leaves, less the editor box's side borders.
func editorWidth(w int) int {
	return w - sidebarWidth(w) - 2
}

// header shows the app's name, its version and the workspace's path or,
// while a tab's write fails, why it can't be saved.
func (m model) header(width int) string {
	where := headerHintSty.Render(tildePath(m.root))
	if doc := m.saveProblem(); doc != nil {
		where = warnStyle.Render("⚠ Can't save " + displayName(doc.path) + ": " + errText(doc.saveErr))
	}
	left := headerAppStyle.Render("edit") + " " + headerHintSty.Render(m.version) + "  " + where
	return ansi.Truncate(" "+left, width, "")
}

// footer lists the key hints: the file tree's while the sidebar has focus,
// the tab keys while the editor has focus and a tab is open, then the global
// ones. Each hint shows the keys its actions are bound to, and a hint whose
// actions have no keys is left out. The editing keys are left to the README:
// four hints is what fits in 80 columns. After a close or a quit that could
// not save, the footer asks for it again instead.
func (m model) footer(width int) string {
	var parts []string
	add := func(label string, ids ...keymap.ID) {
		if keys := m.keys.Label(ids...); keys != "" {
			parts = append(parts, hint(keys, label))
		}
	}
	switch {
	case m.confirm == actCloseTab:
		add("Close anyway", actCloseTab)
		parts = append(parts, hint("any other key", "Cancel"))
		return ansi.Truncate(" "+strings.Join(parts, "  "), width, "")
	case m.confirm == actQuit:
		add("Quit anyway", actQuit)
		parts = append(parts, hint("any other key", "Cancel"))
		return ansi.Truncate(" "+strings.Join(parts, "  "), width, "")
	case m.focus == paneSidebar:
		add("Move", sidebarUp, sidebarDown)
		add("Fold", sidebarCollapse, sidebarExpand)
		add("Open", sidebarOpen)
	case m.tabs.Len() > 0:
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
