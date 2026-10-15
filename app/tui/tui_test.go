package tui

import (
	"strings"
	"testing"

	"github.com/candy-tools/edit/app/metainfo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestNewModelStartState(t *testing.T) {
	m := newModel("/work/project")
	if m.focus != paneSidebar {
		t.Errorf("start focus = %v, want the sidebar", m.focus)
	}
	if len(m.tabs) != 1 || m.tabs[0].title != "untitled" || m.active != 0 {
		t.Errorf("start tabs = %+v (active %d), want a single active untitled tab", m.tabs, m.active)
	}
	if m.Init() != nil {
		t.Error("Init should not schedule any command")
	}
}

func TestTabTogglesFocus(t *testing.T) {
	m := press(newTestModel(), "tab")
	if m.focus != paneEditor {
		t.Fatalf("after tab focus = %v, want the editor", m.focus)
	}
	m = press(m, "tab")
	if m.focus != paneSidebar {
		t.Fatalf("after a second tab focus = %v, want the sidebar", m.focus)
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []string{"ctrl+c", "ctrl+q"} {
		_, cmd := newTestModel().Update(keyMsg(k))
		if cmd == nil {
			t.Fatalf("%s should quit, got no command", k)
		}
		if msg := cmd(); msg != (tea.QuitMsg{}) {
			t.Errorf("%s should quit, got %T", k, msg)
		}
	}
}

// Plain letters must not trigger anything: once the editor takes input, they
// are text.
func TestLettersAreNotCommands(t *testing.T) {
	m := newTestModel()
	nm, cmd := m.Update(keyMsg("q"))
	if cmd != nil {
		t.Error("q must not quit")
	}
	if nm.(model).focus != m.focus {
		t.Error("q must not move focus")
	}
}

func TestViewFillsTerminal(t *testing.T) {
	lines := strings.Split(newTestModel().View(), "\n")
	if len(lines) != 30 {
		t.Fatalf("view is %d lines, want 30", len(lines))
	}
	for i, l := range lines[1 : len(lines)-1] {
		if w := lipgloss.Width(l); w != 100 {
			t.Errorf("body line %d is %d cells wide, want 100", i+1, w)
		}
	}
}

func TestViewDefaultsTo80x24(t *testing.T) {
	lines := strings.Split(newModel("/work/project").View(), "\n")
	if len(lines) != 24 || lipgloss.Width(lines[1]) != 80 {
		t.Errorf("before a WindowSizeMsg the view should be 80x24, got %d lines of %d cells",
			len(lines), lipgloss.Width(lines[1]))
	}
}

func TestViewChrome(t *testing.T) {
	lines := strings.Split(ansi.Strip(newTestModel().View()), "\n")
	if want := "edit " + metainfo.Version + "  /work/project"; !strings.Contains(lines[0], want) {
		t.Errorf("header %q should contain %q", lines[0], want)
	}
	if !strings.Contains(lines[1], "Files") || !strings.Contains(lines[1], "untitled") {
		t.Errorf("top border %q should carry the sidebar title and the tab strip", lines[1])
	}
	footer := lines[len(lines)-1]
	for _, want := range []string{"tab: Focus", "ctrl+q: Quit"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer %q should contain %q", footer, want)
		}
	}
}

func TestSidebarSitsLeftOfEditor(t *testing.T) {
	top := []rune(ansi.Strip(strings.Split(newTestModel().View(), "\n")[1]))
	if top[0] != '╭' || top[29] != '╮' || top[30] != '╭' || top[99] != '╮' {
		t.Errorf("want a 30-cell sidebar box, then a 70-cell editor box:\n%s", string(top))
	}
}

func TestSidebarWidth(t *testing.T) {
	for _, c := range []struct{ w, want int }{{200, 30}, {90, 30}, {60, 20}, {30, 10}} {
		if got := sidebarWidth(c.w); got != c.want {
			t.Errorf("sidebarWidth(%d) = %d, want %d", c.w, got, c.want)
		}
	}
}

func TestTabStripListsTabsInOrder(t *testing.T) {
	m := press(newTestModel(), "tab") // focus the editor
	m.tabs = []tab{{title: "main.go"}, {title: "go.mod"}, {title: "README.md"}}
	m.active = 1
	if got, want := ansi.Strip(m.tabStrip()), " main.go ─ go.mod ─ README.md "; got != want {
		t.Errorf("tab strip = %q, want %q", got, want)
	}
}

func TestTildePath(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	for in, want := range map[string]string{
		"/home/u":         "~",
		"/home/u/src/app": "~/src/app",
		"/home/user/app":  "/home/user/app", // shares the prefix, but is not under home
		"/etc":            "/etc",
	} {
		if got := tildePath(in); got != want {
			t.Errorf("tildePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTildePathWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	if got := tildePath("/home/u/src"); got != "/home/u/src" {
		t.Errorf("with no home directory the path should be unchanged, got %q", got)
	}
}
