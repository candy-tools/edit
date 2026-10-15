// Package tui is the Bubble Tea frontend of the editor: a file sidebar on the
// left and the open files as tabs in an editor pane on the right, framed by a
// header and a key-hint footer. For now it is only that layout — the sidebar
// lists nothing and the single "untitled" tab holds no buffer.
package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/candy-tools/edit/app/metainfo"
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

// tab is one open editor tab.
type tab struct {
	title string
}

type model struct {
	root    string // the workspace directory, shown in the header
	version string
	width   int
	height  int

	focus  pane  // the pane that receives keys, drawn with an accent border
	tabs   []tab // the open tabs, left to right
	active int   // index into tabs of the tab shown in the editor pane
}

// Run starts the interactive editor on the workspace directory root.
func Run(root string) error {
	_, err := tea.NewProgram(newModel(root), tea.WithAltScreen()).Run()
	return err
}

// newModel builds the start-up state: the sidebar focused and a single empty
// "untitled" tab. Split out from Run so tests can drive the model without
// starting a real terminal program.
func newModel(root string) model {
	return model{
		root:    root,
		version: metainfo.Version,
		focus:   paneSidebar,
		tabs:    []tab{{title: "untitled"}},
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
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
		}
	}
	return m, nil
}

// View composes the header, the sidebar and editor panes side by side, and the
// footer to the terminal size.
func (m model) View() string {
	w, h := m.width, m.height
	if w == 0 {
		w, h = 80, 24 // before the first WindowSizeMsg
	}
	bodyH := max(h-2, 3)
	sw := sidebarWidth(w)
	sidebar := titledBox("Files", "", sw, bodyH, m.focus == paneSidebar)
	editor := frame(m.tabStrip(), "", w-sw, bodyH, m.focus == paneEditor)
	return m.header(w) + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, sidebar, editor) + "\n" + m.footer(w)
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

func (m model) footer(width int) string {
	parts := []string{hint("tab", "Focus"), hint("ctrl+q", "Quit")}
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
