// Package tui is the Bubble Tea frontend of the editor: a file sidebar on the
// left and the open files as tabs in an editor pane on the right, framed by a
// header and a key-hint footer. The sidebar browses the workspace as a folder
// tree; the single "untitled" tab holds no buffer yet.
package tui

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/candy-tools/edit/app/metainfo"
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

// defaultWidth and defaultHeight are the terminal size assumed until the
// first tea.WindowSizeMsg arrives.
const defaultWidth, defaultHeight = 80, 24

// tab is one open editor tab.
type tab struct {
	title string
}

type model struct {
	root    string // the workspace directory, shown in the header
	version string
	width   int
	height  int

	focus  pane                  // the pane that receives keys, drawn with an accent border
	files  *treeview.Tree[entry] // the sidebar's file tree over the workspace
	tabs   []tab                 // the open tabs, left to right
	active int                   // index into tabs of the tab shown in the editor pane
}

// Run starts the interactive editor on the workspace directory root.
func Run(root string) error {
	_, err := tea.NewProgram(newModel(root, os.DirFS(root)), tea.WithAltScreen()).Run()
	return err
}

// newModel builds the start-up state: the sidebar focused on the file tree of
// fsys — the workspace at root — and a single empty "untitled" tab. Split out
// from Run so tests can drive the model on an in-memory filesystem without
// starting a real terminal program.
func newModel(root string, fsys fs.FS) model {
	files := treeview.New(workspace{fsys}, rootEntry)
	files.SetHeight(treeHeight(defaultHeight))
	return model{
		root:    root,
		version: metainfo.Version,
		focus:   paneSidebar,
		files:   files,
		tabs:    []tab{{title: "untitled"}},
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.files.SetHeight(treeHeight(msg.Height))
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			return m, tea.Quit
		case "tab":
			if m.focus == paneSidebar {
				m.focus = paneEditor
			} else {
				m.focus = paneSidebar
			}
		default:
			if m.focus == paneSidebar {
				sidebarKey(m.files, msg.String())
			}
		}
	}
	return m, nil
}

// View composes the header, the sidebar and editor panes side by side, and the
// footer to the terminal size.
func (m model) View() string {
	w, h := m.width, m.height
	if w == 0 {
		w, h = defaultWidth, defaultHeight // before the first WindowSizeMsg
	}
	bodyH := bodyHeight(h)
	sw := sidebarWidth(w)
	sidebar := titledBox("Files", m.sidebarBody(), sw, bodyH, m.focus == paneSidebar)
	editor := frame(m.tabStrip(), "", w-sw, bodyH, m.focus == paneEditor)
	return m.header(w) + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, sidebar, editor) + "\n" + m.footer(w)
}

// bodyHeight is the height of the sidebar and editor boxes in a terminal h
// rows tall: all of it but the header and footer lines, and at least 3.
func bodyHeight(h int) int {
	return max(h-2, 3)
}

// treeHeight is how many file tree rows fit inside the sidebar box in a
// terminal h rows tall: the box's height less its top and bottom borders.
func treeHeight(h int) int {
	return bodyHeight(h) - 2
}

// sidebarWidth is the file sidebar's width at terminal width w: 30 columns,
// but never more than a third of the screen, so a narrow terminal keeps most
// of its width for the editor.
func sidebarWidth(w int) int {
	return min(30, w/3)
}

// tabStrip renders the open tabs for the editor pane's top border, left to
// right: the active tab accented while the pane has focus (bold otherwise),
// the rest dim, separated by border dashes.
func (m model) tabStrip() string {
	focused := m.focus == paneEditor
	parts := make([]string, len(m.tabs))
	for i, t := range m.tabs {
		st := titleDimStyle
		if i == m.active {
			st = activeTabStyle
			if focused {
				st = titleActiveStyle
			}
		}
		parts[i] = st.Render(" " + t.title + " ")
	}
	return strings.Join(parts, borderStyle(focused).Render("─"))
}

func (m model) header(width int) string {
	left := headerAppStyle.Render("edit") + " " + headerHintSty.Render(m.version) + "  " + headerHintSty.Render(tildePath(m.root))
	return ansi.Truncate(" "+left, width, "")
}

// footer lists the key hints: the file tree's while the sidebar has focus,
// then the global ones.
func (m model) footer(width int) string {
	var parts []string
	if m.focus == paneSidebar {
		parts = append(parts, hint("↑↓", "Move"), hint("←→", "Fold"))
	}
	parts = append(parts, hint("tab", "Focus"), hint("ctrl+q", "Quit"))
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
