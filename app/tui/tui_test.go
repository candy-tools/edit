package tui

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/candy-tools/edit/app/metainfo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestNewModelStartState(t *testing.T) {
	m := newModel("/work/project", testFS)
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
// are text, and the file tree is driven by the arrows, not vim-style letters.
func TestLettersAreNotCommands(t *testing.T) {
	m := newTestModel()
	tree := ansi.Strip(m.sidebarBody())
	nm, cmd := m.Update(keyMsg("q"))
	if cmd != nil {
		t.Error("q must not quit")
	}
	if nm.(model).focus != m.focus {
		t.Error("q must not move focus")
	}
	m = press(nm.(model), "j", "k", "l", "h")
	if got := ansi.Strip(m.sidebarBody()); got != tree {
		t.Errorf("letters must not drive the tree, got\n%s\nwant\n%s", got, tree)
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
	lines := strings.Split(newModel("/work/project", testFS).View(), "\n")
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

// deniedFS is a filesystem that refuses to open anything.
type deniedFS struct{}

func (deniedFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
}

// manyFiles is a workspace of n files named f00, f01, ….
func manyFiles(n int) fstest.MapFS {
	fsys := fstest.MapFS{}
	for i := range n {
		fsys[fmt.Sprintf("f%02d", i)] = &fstest.MapFile{}
	}
	return fsys
}

// sidebarRows is the sidebar's tree, one string per row, without styling.
func sidebarRows(m model) []string {
	return strings.Split(ansi.Strip(m.sidebarBody()), "\n")
}

func TestSidebarListsTheWorkspaceAtStart(t *testing.T) {
	if got, want := ansi.Strip(newTestModel().sidebarBody()), "❯ ▸ app\n    go.mod\n    README.md"; got != want {
		t.Errorf("sidebar =\n%s\nwant\n%s", got, want)
	}
}

// The tree reaches the screen: its rows fill the Files box, one per line
// under the box's top border.
func TestViewDrawsTheTreeInTheFilesBox(t *testing.T) {
	lines := strings.Split(ansi.Strip(newTestModel().View()), "\n")
	for i, row := range []string{"❯ ▸ app", "    go.mod", "    README.md"} {
		if got := lines[2+i]; !strings.HasPrefix(got, "│"+row+" ") {
			t.Errorf("view line %d = %q, want the tree row %q inside the Files box", 2+i, got, row)
		}
	}
}

func TestSidebarShowsAWorkspaceItCannotRead(t *testing.T) {
	if got, want := ansi.Strip(newModel("/work/project", deniedFS{}).sidebarBody()), "❯ ⚠ permission denied"; got != want {
		t.Errorf("sidebar = %q, want %q", got, want)
	}
}

func TestSidebarKeysDriveTheTree(t *testing.T) {
	m := newTestModel()
	for _, step := range []struct{ key, want string }{
		{"right", "❯ ▾ app\n      main.go\n    go.mod\n    README.md"},
		{"down", "  ▾ app\n❯     main.go\n    go.mod\n    README.md"},
		{"left", "❯ ▾ app\n      main.go\n    go.mod\n    README.md"},
		{"enter", "❯ ▸ app\n    go.mod\n    README.md"},
		{"end", "  ▸ app\n    go.mod\n❯   README.md"},
		{"pgup", "❯ ▸ app\n    go.mod\n    README.md"},
		{"pgdown", "  ▸ app\n    go.mod\n❯   README.md"},
		{"home", "❯ ▸ app\n    go.mod\n    README.md"},
		{"up", "❯ ▸ app\n    go.mod\n    README.md"},
	} {
		m = press(m, step.key)
		if got := ansi.Strip(m.sidebarBody()); got != step.want {
			t.Errorf("after %s sidebar =\n%s\nwant\n%s", step.key, got, step.want)
		}
	}
}

func TestTreeKeysNeedSidebarFocus(t *testing.T) {
	m := press(newTestModel(), "tab") // focus the editor
	tree := ansi.Strip(m.sidebarBody())
	m = press(m, "right", "down", "end", "enter", "left")
	if got := ansi.Strip(m.sidebarBody()); got != tree {
		t.Errorf("with the editor focused the tree must not change, got\n%s\nwant\n%s", got, tree)
	}
}

func TestFooterHintsFollowFocus(t *testing.T) {
	footer := func(m model) string {
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		return lines[len(lines)-1]
	}
	m := newTestModel()
	for _, want := range []string{"↑↓: Move", "←→: Fold", "tab: Focus", "ctrl+q: Quit"} {
		if !strings.Contains(footer(m), want) {
			t.Errorf("sidebar focused: footer %q should contain %q", footer(m), want)
		}
	}
	if f := footer(press(m, "tab")); strings.Contains(f, "Move") || strings.Contains(f, "Fold") {
		t.Errorf("editor focused: footer %q should not offer the tree keys", f)
	}
}

func TestSidebarScrollsWithinTheBox(t *testing.T) {
	m := send(newModel("/w", manyFiles(20)), tea.WindowSizeMsg{Width: 100, Height: 10})
	if rows := sidebarRows(m); len(rows) != 6 || rows[0] != "❯   f00" {
		t.Errorf("a 10-row terminal leaves 6 tree rows starting at f00, got %q", rows)
	}
	m = press(m, "end")
	if rows := sidebarRows(m); len(rows) != 6 || rows[0] != "    f14" || rows[5] != "❯   f19" {
		t.Errorf("after end the window should show f14 to f19, cursor last; got %q", rows)
	}
}

func TestSidebarUsesTheDefaultSizeBeforeTheFirstResize(t *testing.T) {
	if n := len(sidebarRows(newModel("/w", manyFiles(30)))); n != 20 {
		t.Errorf("before a WindowSizeMsg the tree should fill the 80x24 sidebar's 20 rows, got %d", n)
	}
}

// Review focus: long names and deep folders in a narrow sidebar are clipped
// at the box edge and never widen the layout, even when the edge splits a
// double-width character.
func TestLongNamesAndDeepFoldersKeepTheLayout(t *testing.T) {
	deep := "a/b/c/d/" + strings.Repeat("long-name-", 8)
	fsys := fstest.MapFS{"日本語日本語日本語日本語.txt": {}, deep: {}}
	m := send(newModel("/w", fsys), tea.WindowSizeMsg{Width: 63, Height: 12})
	// → alternately opens a folder and steps into it, down to the deep file.
	m = press(m, "right", "right", "right", "right", "right", "right", "right", "right")
	if row, _ := m.files.Selected(); row.Depth != 4 || row.Node.dir {
		t.Fatalf("cursor on %+v, want the file four folders down", row)
	}
	lines := strings.Split(m.View(), "\n")
	for i, l := range lines[1 : len(lines)-1] {
		if w := lipgloss.Width(l); w != 63 {
			t.Errorf("body line %d is %d cells wide, want 63", i+1, w)
		}
	}
}

// Review focus: a tiny terminal still shows one tree row, and the cursor row
// scrolls into it.
func TestTinyTerminalShowsOneTreeRow(t *testing.T) {
	m := send(newTestModel(), tea.WindowSizeMsg{Width: 20, Height: 3})
	if got := ansi.Strip(m.sidebarBody()); got != "❯ ▸ app" {
		t.Errorf("sidebar = %q, want just the cursor row", got)
	}
	m = press(m, "down")
	if got := ansi.Strip(m.sidebarBody()); got != "❯   go.mod" {
		t.Errorf("after down sidebar = %q, want the new cursor row", got)
	}
}

// Review focus: a file name holding a newline or an escape code keeps one
// row per entry and the box aligned.
func TestUnsafeNamesKeepOneRowPerEntry(t *testing.T) {
	fsys := fstest.MapFS{"two\nlines": {}, "\x1b[2Jclear": {}}
	m := send(newModel("/w", fsys), tea.WindowSizeMsg{Width: 100, Height: 30})
	if got, want := ansi.Strip(m.sidebarBody()), "❯   ?[2Jclear\n    two?lines"; got != want {
		t.Errorf("sidebar = %q, want %q", got, want)
	}
	lines := strings.Split(m.View(), "\n")
	for i, l := range lines[1 : len(lines)-1] {
		if w := lipgloss.Width(l); w != 100 {
			t.Errorf("body line %d is %d cells wide, want 100", i+1, w)
		}
	}
}
