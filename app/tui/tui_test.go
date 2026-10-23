package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/candy-tools/edit/app/metainfo"
	"github.com/candy-tools/edit/libs/keymap"
	"github.com/charmbracelet/x/ansi"
)

func TestNewModelStartState(t *testing.T) {
	m := newModel("/work/project", testFS, noWrites, testKeys, noColors, noMouse)
	if m.focus != paneSidebar {
		t.Errorf("start focus = %v, want the sidebar", m.focus)
	}
	if m.tabs.Len() != 0 {
		t.Errorf("start tabs = %d, want none open", m.tabs.Len())
	}
	// Init only asks for grapheme widths (TestInitAsksForGraphemeWidths):
	// nothing to save, no tick, no parse.
	if _, ok := m.Init()().(tea.ModeReportMsg); !ok {
		t.Error("Init should schedule nothing but the grapheme widths")
	}
}

func TestShiftTabTogglesFocus(t *testing.T) {
	m := press(newTestModel(), "shift+tab")
	if m.focus != paneEditor {
		t.Fatalf("after shift+tab focus = %v, want the editor", m.focus)
	}
	m = press(m, "shift+tab")
	if m.focus != paneSidebar {
		t.Fatalf("after a second shift+tab focus = %v, want the sidebar", m.focus)
	}
}

// tab types a tab in the editor, so it no longer switches focus, and in the
// sidebar it does nothing.
func TestTabTypesATabInTheEditor(t *testing.T) {
	m := newTestModel()
	tree := sidebarRows(m)
	if m = press(m, "tab"); m.focus != paneSidebar || !slices.Equal(sidebarRows(m), tree) {
		t.Errorf("tab in the sidebar: focus %v, tree %q; want nothing to change", m.focus, sidebarRows(m))
	}
	m = press(m, "down", "enter", "tab") // go.mod
	if got, want := m.activeDocument().text.String(), "\tmodule example.com/project\n\ngo 1.26\n"; got != want {
		t.Errorf("tab in the editor: text %q, want %q", got, want)
	}
	if m.focus != paneEditor {
		t.Errorf("tab in the editor moved focus to %v", m.focus)
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

// In the editor letters are text: they type at the cursor. In the sidebar
// they do nothing by default, since the file tree is driven by the arrows,
// not vim-style letters; they are commands there only when the config binds
// them.
func TestLettersTypeOnlyInTheEditor(t *testing.T) {
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
	press(m, "shift+tab", "x") // the editor with no tab open: nothing to type into
	m = press(openBig(30), "j", "k", "f", "b", "G", " ")
	if got, want := editorText(m)[0], "   1 jkfbG line 1"; got != want {
		t.Errorf("after typing, the first row = %q, want %q", got, want)
	}
	if line, offset := cursor(m); line != 0 || offset != 6 {
		t.Errorf("cursor at %d:%d, want after the typed letters, 0:6", line, offset)
	}
}

// Review focus: Bubble Tea measures the frame's characters as edit lays them
// out, as grapheme clusters, whatever the terminal answers to its query.
func TestInitAsksForGraphemeWidths(t *testing.T) {
	cmd := newTestModel().Init()
	if cmd == nil {
		t.Fatal("Init should return a command")
	}
	if got, want := cmd(), (tea.ModeReportMsg{Mode: ansi.ModeUnicodeCore, Value: ansi.ModeSet}); got != want {
		t.Errorf("Init's command = %#v, want %#v", got, want)
	}
}

// Review focus: edit asks the kitty keyboard protocol for alternate keys,
// the shifted sign and the PC-101 key beside each key, which name a key the
// same way in every encoding: alt+? as alt+?, ctrl+й as ctrl+q.
func TestTheViewAsksForAlternateKeys(t *testing.T) {
	if v := newTestModel().View(); !v.KeyboardEnhancements.ReportAlternateKeys || !v.AltScreen {
		t.Errorf("the view asks for %+v, alt screen %t; want alternate keys on the alt screen", v.KeyboardEnhancements, v.AltScreen)
	}
}

// syncBuffer is a bytes.Buffer a program can write from its goroutines
// while a test reads it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// Review focus: in a real program, the renderer switches to grapheme widths
// — it turns the terminal's mode 2027 on — before edit quits.
func TestTheRendererMeasuresGraphemes(t *testing.T) {
	in, keys := io.Pipe()
	out := &syncBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel) // a failed test doesn't leave the program running
	done := make(chan error)
	go func() {
		done <- run(newTestModel(), make(chan os.Signal), tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out),
			tea.WithoutSignalHandler(), tea.WithWindowSize(100, 30))
	}()
	for deadline := time.Now().Add(5 * time.Second); !strings.Contains(out.String(), ansi.SetModeUnicodeCore); {
		if time.Now().After(deadline) {
			t.Fatal("the renderer never switched to grapheme widths")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := keys.Write([]byte("\x11")); err != nil { // ctrl+q
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// Review focus: Bubble Tea v2 sends a size of 0x0 when it has no terminal;
// the model keeps the size it had.
func TestAZeroSizeIsIgnored(t *testing.T) {
	m := send(newTestModel(), tea.WindowSizeMsg{})
	if w, h := m.size(); w != 100 || h != 30 {
		t.Errorf("after a 0x0 size the model is %dx%d, want the 100x30 it had", w, h)
	}
	if got := len(sidebarRows(m)); got != 3 {
		t.Errorf("after a 0x0 size the tree shows %d rows, want all 3", got)
	}
}

func TestTheEditingKeys(t *testing.T) {
	m := press(newTestModel(), "down", "down", "enter") // README.md: "# project\n"
	for _, step := range []struct {
		key, want string
	}{
		{"end", "# project\n"},
		{"enter", "# project\n\n"},
		{"x", "# project\nx\n"},
		{"backspace", "# project\n\n"},
		{"ctrl+h", "# project\n"}, // some terminals send ctrl+h for backspace
		{"home", "# project\n"},
		{"delete", " project\n"},
		{"space", "  project\n"},
		{"ctrl+k", "\n"},
	} {
		m = press(m, step.key)
		if got := m.activeDocument().text.String(); got != step.want {
			t.Errorf("after %s the text is %q, want %q", step.key, got, step.want)
		}
	}
}

func TestPastedTextGoesInAtTheCursor(t *testing.T) {
	m := paste(openBig(30), "a\r\nb\rc\nd")
	if got, want := editorText(m)[:4], []string{"   1 a", "   2 b", "   3 c", "   4 dline 1"}; !slices.Equal(got, want) {
		t.Errorf("after pasting, the rows = %q, want %q", got, want)
	}
	if line, offset := cursor(m); line != 3 || offset != 1 {
		t.Errorf("cursor at %d:%d, want after the pasted text, 3:1", line, offset)
	}
}

// Review focus: narrowing the terminal while the cursor is far to the right
// keeps the cursor in view.
func TestAResizeKeepsTheCursorInView(t *testing.T) {
	fsys := fstest.MapFS{"long.txt": {Data: []byte(strings.Repeat("abcdefghij", 10) + "END\n")}}
	m := press(send(newModel("/w", fsys, noWrites, testKeys, noColors, noMouse), tea.WindowSizeMsg{Width: 200, Height: 10}), "enter", "end")
	m = send(m, tea.WindowSizeMsg{Width: 60, Height: 10})
	if got := editorText(m)[0]; !strings.HasSuffix(got, "END") {
		t.Errorf("after narrowing, the first row = %q, want the line's end in view", got)
	}
}

func TestViewFillsTerminal(t *testing.T) {
	lines := strings.Split(newTestModel().View().Content, "\n")
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
	lines := strings.Split(newModel("/work/project", testFS, noWrites, testKeys, noColors, noMouse).View().Content, "\n")
	if len(lines) != 24 || lipgloss.Width(lines[1]) != 80 {
		t.Errorf("before a WindowSizeMsg the view should be 80x24, got %d lines of %d cells",
			len(lines), lipgloss.Width(lines[1]))
	}
}

func TestViewChrome(t *testing.T) {
	lines := strings.Split(ansi.Strip(newTestModel().View().Content), "\n")
	if want := "edit " + metainfo.Version + "  /work/project"; !strings.Contains(lines[0], want) {
		t.Errorf("header %q should contain %q", lines[0], want)
	}
	if !strings.Contains(lines[1], "Files") {
		t.Errorf("top border %q should carry the sidebar title", lines[1])
	}
	footer := lines[len(lines)-1]
	for _, want := range []string{"shift+tab: Focus", "ctrl+q: Quit"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer %q should contain %q", footer, want)
		}
	}
}

func TestSidebarSitsLeftOfEditor(t *testing.T) {
	top := []rune(ansi.Strip(strings.Split(newTestModel().View().Content, "\n")[1]))
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

// topBorder is the top line of m's view, which holds the sidebar's title and
// the tab strip, without styling.
func topBorder(m model) string {
	return ansi.Strip(strings.Split(m.View().Content, "\n")[1])
}

// openDocs opens a tab in m for each path, in order, holding a document with
// a note instead of text, so the last one is active.
func openDocs(m model, paths ...string) model {
	for _, p := range paths {
		m.tabs.Open(p, func() *document { return &document{path: p, note: "test"} })
	}
	return m
}

func TestTheEditorBorderCarriesTheTabStrip(t *testing.T) {
	m := openDocs(newTestModel(), "app/main.go", "go.mod", "README.md")
	if top, want := topBorder(m), "╭─ main.go ─ go.mod ─ README.md ─"; !strings.Contains(top, want) {
		t.Errorf("top border %q should hold the tab strip %q", top, want)
	}
}

func TestNoTabOpenLeavesThePlainBorder(t *testing.T) {
	if got, want := string([]rune(topBorder(newTestModel()))[30:]), "╭"+strings.Repeat("─", 68)+"╮"; got != want {
		t.Errorf("editor's top border = %q, want a plain line", got)
	}
}

// eightFiles are the paths of eight tabs that take 8 cells each in the strip.
var eightFiles = []string{"f0.go", "f1.go", "f2.go", "f3.go", "f4.go", "f5.go", "f6.go", "f7.go"}

func TestTheTabStripFollowsTheTerminalWidth(t *testing.T) {
	m := openDocs(newTestModel(), eightFiles...) // 64 cells of tabs, 62 to show them in
	if top, want := topBorder(m), "╭─…─ f1.go ─"; !strings.Contains(top, want) {
		t.Errorf("at 100 columns the top border %q should hide f0.go: want %q", top, want)
	}
	m = send(m, tea.WindowSizeMsg{Width: 200, Height: 30})
	if top, want := topBorder(m), "╭─ f0.go ─"; !strings.Contains(top, want) {
		t.Errorf("at 200 columns the top border %q should show every tab: want %q", top, want)
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
	lines := strings.Split(ansi.Strip(newTestModel().View().Content), "\n")
	for i, row := range []string{"❯ ▸ app", "    go.mod", "    README.md"} {
		if got := lines[2+i]; !strings.HasPrefix(got, "│"+row+" ") {
			t.Errorf("view line %d = %q, want the tree row %q inside the Files box", 2+i, got, row)
		}
	}
}

func TestSidebarShowsAWorkspaceItCannotRead(t *testing.T) {
	if got, want := ansi.Strip(newModel("/work/project", deniedFS{}, noWrites, testKeys, noColors, noMouse).sidebarBody()), "❯ ⚠ permission denied"; got != want {
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
	m := press(newTestModel(), "shift+tab") // focus the editor
	tree := ansi.Strip(m.sidebarBody())
	m = press(m, "right", "down", "end", "enter", "left")
	if got := ansi.Strip(m.sidebarBody()); got != tree {
		t.Errorf("with the editor focused the tree must not change, got\n%s\nwant\n%s", got, tree)
	}
}

// footerText is the last line of m's view, the footer, without styling.
func footerText(m model) string {
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	return lines[len(lines)-1]
}

func TestFooterHintsFollowFocusAndOpenTabs(t *testing.T) {
	// Each case starts from its own model: copies of a model share its tree
	// and tab set.
	for _, c := range []struct {
		name string
		m    model
		want string
	}{
		{"sidebar focused", newTestModel(), " ↑↓: Move  ←→: Fold  enter: Open  shift+tab: Focus  ctrl+q: Quit"},
		{"editor focused, no tab open", press(newTestModel(), "shift+tab"), " shift+tab: Focus  ctrl+q: Quit"},
		{"editor focused, a tab open", press(newTestModel(), "down", "enter"),
			" ctrl+pgup/pgdown: Tabs  ctrl+w: Close  shift+tab: Focus  ctrl+q: Quit"},
	} {
		if got := footerText(c.m); got != c.want {
			t.Errorf("%s: footer = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSidebarScrollsWithinTheBox(t *testing.T) {
	m := send(newModel("/w", manyFiles(20), noWrites, testKeys, noColors, noMouse), tea.WindowSizeMsg{Width: 100, Height: 10})
	if rows := sidebarRows(m); len(rows) != 6 || rows[0] != "❯   f00" {
		t.Errorf("a 10-row terminal leaves 6 tree rows starting at f00, got %q", rows)
	}
	m = press(m, "end")
	if rows := sidebarRows(m); len(rows) != 6 || rows[0] != "    f14" || rows[5] != "❯   f19" {
		t.Errorf("after end the window should show f14 to f19, cursor last; got %q", rows)
	}
}

func TestSidebarUsesTheDefaultSizeBeforeTheFirstResize(t *testing.T) {
	if n := len(sidebarRows(newModel("/w", manyFiles(30), noWrites, testKeys, noColors, noMouse))); n != 20 {
		t.Errorf("before a WindowSizeMsg the tree should fill the 80x24 sidebar's 20 rows, got %d", n)
	}
}

// Review focus: long names and deep folders in a narrow sidebar are clipped
// at the box edge and never widen the layout, even when the edge splits a
// double-width character.
func TestLongNamesAndDeepFoldersKeepTheLayout(t *testing.T) {
	deep := "a/b/c/d/" + strings.Repeat("long-name-", 8)
	fsys := fstest.MapFS{"日本語日本語日本語日本語.txt": {}, deep: {}}
	m := send(newModel("/w", fsys, noWrites, testKeys, noColors, noMouse), tea.WindowSizeMsg{Width: 63, Height: 12})
	// → alternately opens a folder and steps into it, down to the deep file.
	m = press(m, "right", "right", "right", "right", "right", "right", "right", "right")
	if row, _ := m.files.Selected(); row.Depth != 4 || row.Node.dir {
		t.Fatalf("cursor on %+v, want the file four folders down", row)
	}
	lines := strings.Split(m.View().Content, "\n")
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
	m := send(newModel("/w", fsys, noWrites, testKeys, noColors, noMouse), tea.WindowSizeMsg{Width: 100, Height: 30})
	if got, want := ansi.Strip(m.sidebarBody()), "❯   ?[2Jclear\n    two?lines"; got != want {
		t.Errorf("sidebar = %q, want %q", got, want)
	}
	lines := strings.Split(m.View().Content, "\n")
	for i, l := range lines[1 : len(lines)-1] {
		if w := lipgloss.Width(l); w != 100 {
			t.Errorf("body line %d is %d cells wide, want 100", i+1, w)
		}
	}
}

// bigFS is a workspace holding one file of 100 numbered lines.
var bigFS = fstest.MapFS{"big.txt": {Data: []byte(numberedLines(100))}}

// openBig returns a model on bigFS, 100 columns wide and h rows tall, with
// big.txt open and the editor focused.
func openBig(h int) model {
	return press(send(newModel("/w", bigFS, noWrites, testKeys, noColors, noMouse), tea.WindowSizeMsg{Width: 100, Height: h}), "enter")
}

// editorText is the editor pane's body as m's view draws it, one string per
// row, without the box's borders, its trailing padding or styling.
func editorText(m model) []string {
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	w, _ := m.size()
	sw := sidebarWidth(w)
	var rows []string
	for _, l := range lines[2 : len(lines)-2] { // between the boxes' top and bottom borders
		r := []rune(l)
		rows = append(rows, strings.TrimRight(string(r[sw+1:w-1]), " "))
	}
	return rows
}

// activeKey is the path of m's active tab, or "" when no tab is open.
func activeKey(m model) string {
	t, _ := m.tabs.Active()
	return t.Key
}

// threeTabs is newTestModel with every file of testFS open, in tree order:
// app/main.go, go.mod, then README.md, which is active, with the editor
// focused.
func threeTabs() model {
	return openAll(newTestModel())
}

// openAll opens every file of testFS in m, fresh from newModel, as
// threeTabs does.
func openAll(m model) model {
	return press(m, "right", "down", "enter", "shift+tab", "down", "enter", "shift+tab", "down", "enter")
}

func TestEnterOpensAFileInATab(t *testing.T) {
	m := press(newTestModel(), "down", "enter") // the cursor on go.mod
	if m.focus != paneEditor {
		t.Errorf("after enter focus = %v, want the editor", m.focus)
	}
	if got, want := editorText(m)[:4], []string{" 1 module example.com/project", " 2", " 3 go 1.26", ""}; !slices.Equal(got, want) {
		t.Errorf("editor = %q, want %q", got, want)
	}
	if top, want := topBorder(m), "╭─ go.mod ─"; !strings.Contains(top, want) {
		t.Errorf("top border %q should hold the tab %q", top, want)
	}
}

// The flow this feature was asked for: opening a second file leaves the first
// one open in its own tab.
func TestOpeningASecondFileKeepsTheFirstOpen(t *testing.T) {
	m := press(newTestModel(), "down", "enter", "shift+tab", "down", "enter") // go.mod, then README.md
	tabs, active := m.tabs.Tabs()
	if len(tabs) != 2 || tabs[0].Key != "go.mod" || tabs[1].Key != "README.md" || active != 1 {
		t.Fatalf("tabs = %+v (active %d), want go.mod then README.md, README.md active", tabs, active)
	}
	if top, want := topBorder(m), "╭─ go.mod ─ README.md ─"; !strings.Contains(top, want) {
		t.Errorf("top border %q should hold both tabs: %q", top, want)
	}
	if got, want := editorText(m)[0], " 1 # project"; got != want {
		t.Errorf("editor's first row = %q, want %q", got, want)
	}
}

// countingFS is a filesystem that counts how many times each file is opened.
type countingFS struct {
	fstest.MapFS
	opens map[string]int
}

func (c countingFS) Open(name string) (fs.File, error) {
	c.opens[name]++
	return c.MapFS.Open(name)
}

func TestEnterOnAnOpenFileSwitchesToItsTab(t *testing.T) {
	fsys := countingFS{MapFS: testFS, opens: map[string]int{}}
	m := send(newModel("/work/project", fsys, noWrites, testKeys, noColors, noMouse), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = press(m, "down", "enter", "shift+tab", "down", "enter", "shift+tab", "up", "enter") // go.mod, README.md, go.mod
	if tabs, active := m.tabs.Tabs(); len(tabs) != 2 || active != 0 {
		t.Errorf("tabs = %+v (active %d), want the two tabs with go.mod active", tabs, active)
	}
	if fsys.opens["go.mod"] != 1 {
		t.Errorf("go.mod was read %d times, want once", fsys.opens["go.mod"])
	}
	if m.focus != paneEditor {
		t.Errorf("focus = %v, want the editor", m.focus)
	}
}

// Review focus: a symlink and the file it names, or two hard links, are one
// file: it gets one tab, since two would overwrite each other's saves.
func TestOneFileNeverGetsTwoTabs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b-real.txt"), []byte("text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(dir, "b-real.txt"), filepath.Join(dir, "a-hard.txt")); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, "b-real.txt", filepath.Join(dir, "c-sym.txt"))
	m := send(newModel(dir, os.DirFS(dir), noWrites, testKeys, noColors, noMouse), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = press(m, "enter", "shift+tab", "down", "enter", "shift+tab", "down", "enter") // a-hard, b-real, c-sym
	if tabs, _ := m.tabs.Tabs(); len(tabs) != 1 || tabs[0].Key != "a-hard.txt" {
		t.Errorf("tabs = %+v, want the one tab for the one file", tabs)
	}
}

func TestEnterOnAFolderOpensNoTab(t *testing.T) {
	m := press(newTestModel(), "enter") // the cursor on app
	if m.tabs.Len() != 0 || m.focus != paneSidebar {
		t.Errorf("enter on a folder: %d tabs, focus %v; want none and the sidebar", m.tabs.Len(), m.focus)
	}
	if got, want := sidebarRows(m), []string{"❯ ▾ app", "      main.go", "    go.mod", "    README.md"}; !slices.Equal(got, want) {
		t.Errorf("sidebar = %q, want the folder opened: %q", got, want)
	}
}

func TestEnterOnAnErrorRowDoesNothing(t *testing.T) {
	m := press(newModel("/w", deniedFS{}, noWrites, testKeys, noColors, noMouse), "enter")
	if m.tabs.Len() != 0 || m.focus != paneSidebar {
		t.Errorf("enter on an error row: %d tabs, focus %v; want none and the sidebar", m.tabs.Len(), m.focus)
	}
}

func TestTheEditorShowsAHintWithNoFileOpen(t *testing.T) {
	if got, want := editorText(newTestModel())[0], " Select a file and press enter to open it"; got != want {
		t.Errorf("editor's first row = %q, want %q", got, want)
	}
}

func TestCtrlPgUpAndPgDownSwitchTabs(t *testing.T) {
	m := threeTabs()
	for _, step := range []struct{ key, want string }{
		{"ctrl+pgdown", "app/main.go"}, // wraps around
		{"ctrl+pgdown", "go.mod"},
		{"ctrl+pgup", "app/main.go"},
		{"ctrl+pgup", "README.md"}, // wraps around
	} {
		m = press(m, step.key)
		if got := activeKey(m); got != step.want {
			t.Errorf("after %s the active tab is %q, want %q", step.key, got, step.want)
		}
	}
	m = press(m, "shift+tab", "ctrl+pgdown") // from the sidebar too
	if got := activeKey(m); got != "app/main.go" || m.focus != paneSidebar {
		t.Errorf("from the sidebar: active %q, focus %v; want app/main.go, the sidebar still focused", got, m.focus)
	}
	if got, want := editorText(m)[0], " 1 package main"; got != want {
		t.Errorf("editor's first row = %q, want the active tab's text %q", got, want)
	}
}

func TestCtrlWClosesTheActiveTab(t *testing.T) {
	m := press(threeTabs(), "ctrl+pgup") // go.mod, in the middle
	for _, want := range []string{"README.md", "app/main.go", ""} {
		m = press(m, "ctrl+w")
		if got := activeKey(m); got != want {
			t.Errorf("after ctrl+w the active tab is %q, want %q", got, want)
		}
	}
	if m.focus != paneSidebar {
		t.Errorf("closing the last tab should focus the sidebar, focus = %v", m.focus)
	}
	if got, want := editorText(m)[0], " Select a file and press enter to open it"; got != want {
		t.Errorf("editor's first row = %q, want the hint %q", got, want)
	}
	m = press(m, "ctrl+w") // nothing left to close
	if m.tabs.Len() != 0 {
		t.Errorf("ctrl+w with no tab open: %d tabs", m.tabs.Len())
	}
}

func TestDoubleEscClosesTheActiveTab(t *testing.T) {
	m := press(threeTabs(), "esc", "esc")
	if got := activeKey(m); m.tabs.Len() != 2 || got != "go.mod" {
		t.Errorf("after esc esc: %d tabs, active %q; want README.md closed and go.mod active", m.tabs.Len(), got)
	}
	m = press(m, "shift+tab", "esc", "esc") // from the sidebar too
	if got := activeKey(m); m.tabs.Len() != 1 || got != "app/main.go" {
		t.Errorf("from the sidebar: %d tabs, active %q; want go.mod closed and app/main.go active", m.tabs.Len(), got)
	}
}

// A double esc pressed fast arrives as alt+esc from a terminal without the
// kitty keyboard protocol.
func TestAltEscClosesTheActiveTab(t *testing.T) {
	if m := press(threeTabs(), "alt+esc"); m.tabs.Len() != 2 {
		t.Errorf("after alt+esc: %d tabs, want README.md closed", m.tabs.Len())
	}
}

func TestAKeyAfterALoneEscCountsOnItsOwn(t *testing.T) {
	m := press(newTestModel(), "down", "enter", "esc", "x", "esc", "right", "y") // go.mod
	if got, want := editorText(m)[0], " 1 xmyodule example.com/project"; got != want {
		t.Errorf("editor's first row = %q, want %q: x typed, the cursor moved, y typed", got, want)
	}
}

// instantTimeout makes the wait for a sequence's next key end at once, so a
// test lets the wait run out by draining the command that started it.
func instantTimeout(press int) tea.Cmd {
	return func() tea.Msg { return sequenceTimeoutMsg{press} }
}

// The wait a key press starts ends for that press, so the sequence it
// started or went on with ends unless another key came since.
func TestTheWaitForASequencesNextKeyRunsOut(t *testing.T) {
	if got, want := sequenceTimer(7)(), (sequenceTimeoutMsg{7}); got != want {
		t.Errorf("the wait ends with %v, want %v", got, want)
	}
}

func TestADoubleEscMustComeWithinTheTimeout(t *testing.T) {
	m := threeTabs()
	m.timeout = instantTimeout
	m, cmd := update(m, keyMsg("esc"))
	m, _ = drain(m, cmd) // the wait for the second esc runs out
	if m = press(m, "esc"); m.tabs.Len() != 3 {
		t.Fatalf("esc, a wait, esc: %d tabs, want all 3 open", m.tabs.Len())
	}
	if m = press(m, "esc"); m.tabs.Len() != 2 {
		t.Errorf("the esc after the wait starts a double esc again: %d tabs, want 2", m.tabs.Len())
	}
}

// Review focus: the wait an earlier esc started never ends the double esc
// a later one starts.
func TestAnOldWaitDoesntEndANewerSequence(t *testing.T) {
	m := threeTabs()
	m.timeout = instantTimeout
	m, first := update(m, keyMsg("esc"))
	m = press(m, "right", "esc")
	m, _ = drain(m, first) // the first esc's wait runs out
	if m = press(m, "esc"); m.tabs.Len() != 2 {
		t.Errorf("the later esc esc: %d tabs, want README.md closed", m.tabs.Len())
	}
}

func TestAPasteBreaksASequence(t *testing.T) {
	m := press(threeTabs(), "esc")
	m = press(paste(m, "x"), "esc")
	if m.tabs.Len() != 3 {
		t.Errorf("esc, a paste, esc: %d tabs, want all 3 open", m.tabs.Len())
	}
}

// newMouseModel is newTestModel with the mouse on.
func newMouseModel() model {
	return send(newModel("/work/project", testFS, noWrites, testKeys, noColors, true), tea.WindowSizeMsg{Width: 100, Height: 30})
}

// click clicks button on the cell where m's view draws the nth (from 0)
// text in the top border: a tab's title, or a ×. It counts a cell per rune,
// which holds for the box's lines and ASCII titles.
func click(t *testing.T, m model, button tea.MouseButton, text string, nth int) model {
	t.Helper()
	top, find := []rune(topBorder(m)), []rune(text)
	for x := range top {
		if x+len(find) <= len(top) && string(top[x:x+len(find)]) == text {
			if nth == 0 {
				return send(m, tea.MouseClickMsg{X: x, Y: 1, Button: button})
			}
			nth--
		}
	}
	t.Fatalf("the top border %q holds no such %q", string(top), text)
	return m
}

// tabKeys returns the keys of m's open tabs, left to right, and the active
// one's.
func tabKeys(m model) (keys []string, active string) {
	tabs, _ := m.tabs.Tabs()
	for _, t := range tabs {
		keys = append(keys, t.Key)
	}
	return keys, activeKey(m)
}

func TestTheMouseIsReportedOnlyWhenItIsOn(t *testing.T) {
	if got := newTestModel().View().MouseMode; got != tea.MouseModeNone {
		t.Errorf("with the mouse off, mouse mode %v, want none", got)
	}
	if got := newMouseModel().View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("with the mouse on, mouse mode %v, want clicks and the wheel", got)
	}
}

func TestEachTabHasAnXWithTheMouseOn(t *testing.T) {
	if top, want := topBorder(openAll(newMouseModel())), "╭─ main.go × ─ go.mod × ─ README.md × ─"; !strings.Contains(top, want) {
		t.Errorf("top border %q should hold %q", top, want)
	}
	if top := topBorder(threeTabs()); strings.Contains(top, "×") {
		t.Errorf("with the mouse off, top border %q should hold no ×", top)
	}
}

func TestClickingATabSwitchesToIt(t *testing.T) {
	m := press(openAll(newMouseModel()), "shift+tab") // the sidebar focused
	m = click(t, m, tea.MouseLeft, "go.mod", 0)
	if got := activeKey(m); got != "go.mod" || m.focus != paneEditor {
		t.Errorf("after clicking go.mod: active %q, focus %v; want go.mod and the editor", got, m.focus)
	}
}

func TestClickingATabsXClosesIt(t *testing.T) {
	m := click(t, openAll(newMouseModel()), tea.MouseLeft, "×", 0) // main.go's
	if keys, active := tabKeys(m); !slices.Equal(keys, []string{"go.mod", "README.md"}) || active != "README.md" {
		t.Errorf("after main.go's ×: tabs %q, active %q; want main.go closed, README.md still active", keys, active)
	}
	m = click(t, m, tea.MouseLeft, "×", 1) // README.md's, the active tab's
	if keys, active := tabKeys(m); !slices.Equal(keys, []string{"go.mod"}) || active != "go.mod" {
		t.Errorf("after README.md's ×: tabs %q, active %q; want just go.mod", keys, active)
	}
	m = click(t, m, tea.MouseLeft, "×", 0)
	if m.tabs.Len() != 0 || m.focus != paneSidebar {
		t.Errorf("after the last ×: %d tabs, focus %v; want none and the sidebar", m.tabs.Len(), m.focus)
	}
}

func TestMiddleClickingATabClosesIt(t *testing.T) {
	m := click(t, openAll(newMouseModel()), tea.MouseMiddle, "go.mod", 0)
	if keys, active := tabKeys(m); !slices.Equal(keys, []string{"app/main.go", "README.md"}) || active != "README.md" {
		t.Errorf("after middle-clicking go.mod: tabs %q, active %q; want go.mod closed", keys, active)
	}
}

func TestOtherClicksDoNothing(t *testing.T) {
	m := openAll(newMouseModel())
	for _, c := range []struct {
		name string
		m    model
	}{
		{"a right click on a tab", click(t, m, tea.MouseRight, "go.mod", 0)},
		{"a click on the dash between two tabs", click(t, m, tea.MouseLeft, "─ go.mod", 0)},
		{"a click in the text", send(m, tea.MouseClickMsg{X: 50, Y: 10, Button: tea.MouseLeft})},
		{"a click in the tree", send(m, tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})},
		{"a click on the border after the tabs", send(m, tea.MouseClickMsg{X: 90, Y: 1, Button: tea.MouseLeft})},
	} {
		if keys, active := tabKeys(c.m); len(keys) != 3 || active != "README.md" || c.m.focus != paneEditor {
			t.Errorf("%s: tabs %q, active %q, focus %v; want nothing changed", c.name, keys, active, c.m.focus)
		}
	}
}

func TestWithTheMouseOffTheMouseDoesNothing(t *testing.T) {
	if m := click(t, threeTabs(), tea.MouseMiddle, "go.mod", 0); m.tabs.Len() != 3 {
		t.Errorf("a middle click with the mouse off: %d tabs, want all 3", m.tabs.Len())
	}
	m := send(openBig(30), tea.MouseWheelMsg{X: 50, Y: 10, Button: tea.MouseWheelDown})
	if line, _ := cursor(m); line != 0 {
		t.Errorf("the wheel with the mouse off: cursor on line %d, want 0", line)
	}
}

func TestAClickBreaksASequence(t *testing.T) {
	m := press(openAll(newMouseModel()), "esc")
	m = press(send(m, tea.MouseClickMsg{X: 50, Y: 10, Button: tea.MouseLeft}), "esc")
	if m.tabs.Len() != 3 {
		t.Errorf("esc, a click, esc: %d tabs, want all 3 open", m.tabs.Len())
	}
}

func TestTheWheelMovesTheCursorOfThePaneUnderIt(t *testing.T) {
	m := send(newModel("/w", bigFS, noWrites, testKeys, noColors, true), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = press(m, "enter", "shift+tab") // big.txt open, the sidebar focused
	wheel := func(m model, button tea.MouseButton, x, y int) model {
		return send(m, tea.MouseWheelMsg{X: x, Y: y, Button: button})
	}
	m = wheel(m, tea.MouseWheelDown, 50, 10)
	if line, _ := cursor(m); line != 3 || m.focus != paneSidebar {
		t.Errorf("a notch down over the text: cursor on line %d, focus %v; want line 3, the sidebar still focused", line, m.focus)
	}
	m = wheel(wheel(m, tea.MouseWheelDown, 50, 10), tea.MouseWheelUp, 50, 10)
	if line, _ := cursor(m); line != 3 {
		t.Errorf("a notch down and one up: cursor on line %d, want 3", line)
	}
	tree := sidebarRows(m)
	m = wheel(m, tea.MouseWheelUp, 50, 0)  // the header
	m = wheel(m, tea.MouseWheelUp, 50, 29) // the footer
	if line, _ := cursor(m); line != 3 || !slices.Equal(sidebarRows(m), tree) {
		t.Errorf("over the header and the footer: cursor on line %d; want the wheel to do nothing", line)
	}
}

func TestTheWheelMovesThroughTheTree(t *testing.T) {
	m := send(newModel("/w", manyFiles(20), noWrites, testKeys, noColors, true), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = send(m, tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	if row, _ := m.files.Selected(); row.Node.path != "f03" {
		t.Errorf("a notch down over the tree: cursor on %q, want f03", row.Node.path)
	}
}

func TestMoveKeysNeedEditorFocus(t *testing.T) {
	m := openBig(30) // a page is 26 lines: the last one starts at line 75
	for _, step := range []struct {
		key          string
		first        string // the window's first row
		line, offset int    // the cursor
	}{
		{"down", "   1 line 1", 1, 0},
		{"pgdown", "  27 line 27", 27, 0},
		{"end", "  27 line 27", 27, 7},
		{"right", "  27 line 27", 27, 7}, // at a line end: the cursor stays on the line
		{"home", "  27 line 27", 27, 0},
		{"left", "  27 line 27", 27, 0}, // at a line start, likewise
		{"ctrl+end", "  75 line 75", 99, 8},
		{"pgup", "  49 line 49", 73, 7},
		{"ctrl+home", "   1 line 1", 0, 0},
		{"right", "   1 line 1", 0, 1},
		{"left", "   1 line 1", 0, 0},
		{"up", "   1 line 1", 0, 0},
	} {
		m = press(m, step.key)
		line, offset := cursor(m)
		if got := editorText(m)[0]; got != step.first || line != step.line || offset != step.offset {
			t.Errorf("after %s: first row %q, cursor %d:%d; want %q, %d:%d", step.key, got, line, offset, step.first, step.line, step.offset)
		}
	}
	m = press(m, "shift+tab", "down", "ctrl+end", "x") // the sidebar focused: the text stays put
	if line, offset := cursor(m); line != 0 || offset != 0 || editorText(m)[0] != "   1 line 1" {
		t.Errorf("with the sidebar focused: cursor %d:%d, first row %q; want 0:0 and line 1", line, offset, editorText(m)[0])
	}
}

func TestEachTabKeepsItsCursor(t *testing.T) {
	lines := []byte(numberedLines(100))
	fsys := fstest.MapFS{"a.txt": {Data: lines}, "b.txt": {Data: lines}}
	m := send(newModel("/w", fsys, noWrites, testKeys, noColors, noMouse), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = press(m, "enter", "pgdown", "right")   // a.txt, a page down
	m = press(m, "shift+tab", "down", "enter") // b.txt, at the top
	if got, want := editorText(m)[0], "   1 line 1"; got != want {
		t.Errorf("b.txt's first row = %q, want %q", got, want)
	}
	m = press(m, "ctrl+pgup") // back to a.txt
	if got, want := editorText(m)[0], "  27 line 27"; got != want {
		t.Errorf("a.txt's first row = %q, want %q: it should keep its scroll position", got, want)
	}
	if line, offset := cursor(m); line != 26 || offset != 1 {
		t.Errorf("a.txt's cursor at %d:%d, want 26:1", line, offset)
	}
}

func TestResizeSetsTheTextWindowHeight(t *testing.T) {
	m := send(openBig(30), tea.WindowSizeMsg{Width: 100, Height: 10})
	m = press(m, "ctrl+end") // a 6-line window: the last page starts at line 95
	rows := editorText(m)
	if len(rows) != 6 || rows[0] != "  95 line 95" || rows[5] != " 100 line 100" {
		t.Errorf("after a resize to 10 rows and end, editor = %q; want lines 95 to 100", rows)
	}
}

func TestAFileOpenedBeforeTheFirstResizeFillsTheDefaultPane(t *testing.T) {
	m := press(newModel("/w", bigFS, noWrites, testKeys, noColors, noMouse), "enter", "ctrl+end") // 80x24: a 20-line window
	rows := editorText(m)
	if len(rows) != 20 || rows[0] != "  81 line 81" || rows[19] != " 100 line 100" {
		t.Errorf("editor = %q; want lines 81 to 100", rows)
	}
}

// Review focus: a tiny terminal still shows one line of the open file, and
// scrolling moves it.
func TestTinyTerminalShowsOneLineOfText(t *testing.T) {
	m := press(send(newModel("/w", bigFS, noWrites, testKeys, noColors, noMouse), tea.WindowSizeMsg{Width: 40, Height: 3}), "enter", "down")
	if rows := editorText(m); len(rows) != 1 || rows[0] != "   2 line 2" {
		t.Errorf("editor = %q, want just line 2", rows)
	}
}

// Review focus: file names and text with wide characters, tabs, escape codes
// and invalid UTF-8, more tabs than fit, and odd or narrow widths still give
// lines exactly as wide as the screen, and no escape code from a file reaches
// the terminal.
func TestOpenFilesKeepTheLayout(t *testing.T) {
	text := strings.Repeat("日本\tx\x1b[2J", 40) + "\n" + strings.Repeat("a", 500) + "\n\xff\xfe broken\n" +
		"\U0001F3FDskin\n\t\U0000093Fsign\n" // marks with nothing to join: a skin tone, a vowel sign
	fsys := fstest.MapFS{}
	for i := range 12 {
		fsys[fmt.Sprintf("ファイル-%02d.txt", i)] = &fstest.MapFile{Data: []byte(text)}
	}
	m := send(newModel("/w", fsys, noWrites, testKeys, noColors, noMouse), tea.WindowSizeMsg{Width: 83, Height: 12})
	for range 12 {
		m = press(m, "enter", "shift+tab", "down") // open each file in turn
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "?[2J") {
		t.Error("an escape code in a file should be drawn as ?, not sent to the terminal")
	}
	m = press(m, "shift+tab") // the editor focused, so the cursor is drawn
	for _, moves := range [][]string{nil, {"end"}, {"down", "end"}, {"ctrl+end"}, {"ctrl+home", "right"}} {
		m = press(m, moves...) // the cursor on a wide character, a tab, past the line, …
		for _, w := range []int{83, 40, 12} {
			m = send(m, tea.WindowSizeMsg{Width: w, Height: 12})
			lines := strings.Split(m.View().Content, "\n")
			for i, l := range lines[1 : len(lines)-1] {
				if got := lipgloss.Width(l); got != w {
					t.Errorf("after %q at width %d: body line %d is %d cells wide", moves, w, i+1, got)
				}
			}
		}
	}
}

func TestARebindingReplacesTheDefaultKeys(t *testing.T) {
	m := press(newReboundModel(t, overrides{"sidebar": {"down": {"j"}}}), "down")
	if got := sidebarRows(m)[0]; got != "❯ ▸ app" {
		t.Errorf("down should no longer move the cursor, first row = %q", got)
	}
	m = press(m, "j")
	if got := sidebarRows(m)[1]; got != "❯   go.mod" {
		t.Errorf("j should move the cursor down, second row = %q", got)
	}
}

func TestAReboundGlobalKeyWorksFromBothPanes(t *testing.T) {
	m := newReboundModel(t, overrides{"global": {"next-tab": {"alt+right"}}})
	m = press(m, "right", "down", "enter", "shift+tab", "down", "enter", "shift+tab", "down", "enter") // three tabs, README.md active
	m = press(m, "ctrl+pgdown")
	if got := activeKey(m); got != "README.md" {
		t.Errorf("ctrl+pgdown should no longer switch tabs, active = %q", got)
	}
	m = press(m, "alt+right")
	if got := activeKey(m); got != "app/main.go" {
		t.Errorf("from the editor alt+right made %q active, want app/main.go", got)
	}
	m = press(m, "shift+tab", "alt+right")
	if got := activeKey(m); got != "go.mod" || m.focus != paneSidebar {
		t.Errorf("from the sidebar: active %q, focus %v; want go.mod, the sidebar still focused", got, m.focus)
	}
}

func TestAReboundQuitKey(t *testing.T) {
	m := newReboundModel(t, overrides{"global": {"quit": {"ctrl+x"}}})
	for _, k := range []string{"ctrl+c", "ctrl+q"} {
		if _, cmd := m.Update(keyMsg(k)); cmd != nil {
			t.Errorf("%s should no longer quit", k)
		}
	}
	if _, cmd := m.Update(keyMsg("ctrl+x")); cmd == nil || cmd() != (tea.QuitMsg{}) {
		t.Error("ctrl+x should quit")
	}
}

func TestAReboundOpenKey(t *testing.T) {
	m := press(newReboundModel(t, overrides{"sidebar": {"open": {"l"}}}), "enter")
	if got := sidebarRows(m); m.tabs.Len() != 0 || len(got) != 3 {
		t.Errorf("enter should no longer open anything: %d tabs, sidebar %q", m.tabs.Len(), got)
	}
	m = press(m, "l") // the cursor on app
	if got, want := sidebarRows(m), []string{"❯ ▾ app", "      main.go", "    go.mod", "    README.md"}; !slices.Equal(got, want) {
		t.Errorf("l on a folder: sidebar = %q, want it opened: %q", got, want)
	}
	m = press(m, "down", "l") // the cursor on app/main.go
	if got := activeKey(m); got != "app/main.go" || m.focus != paneEditor {
		t.Errorf("l on a file: active %q, focus %v; want app/main.go open, the editor focused", got, m.focus)
	}
}

func TestTheFooterShowsTheReboundKeys(t *testing.T) {
	example := overrides{ // the example in the README
		"global":  {"next-tab": {"alt+right"}, "previous-tab": {"alt+left"}, "switch-focus": {"shift+tab", "f6"}},
		"sidebar": {"down": {"down", "j"}, "up": {"up", "k"}, "collapse": {"left", "h"}, "expand": {"right", "l"}},
		"editor":  {"line-start": {"home", "ctrl+a"}, "line-end": {"end", "ctrl+e"}},
	}
	unbound := overrides{"global": {"close-tab": {}, "previous-tab": {}}}
	for _, c := range []struct {
		name string
		m    model
		want string
	}{
		{"sidebar focused", newReboundModel(t, example), " ↑↓: Move  ←→: Fold  enter: Open  shift+tab: Focus  ctrl+q: Quit"},
		{"editor focused", press(newReboundModel(t, example), "down", "enter"),
			" alt+←/→: Tabs  ctrl+w: Close  shift+tab: Focus  ctrl+q: Quit"},
		{"unbound actions", press(newReboundModel(t, unbound), "down", "enter"),
			" ctrl+pgdown: Tabs  shift+tab: Focus  ctrl+q: Quit"},
	} {
		if got := footerText(c.m); got != c.want {
			t.Errorf("%s: footer = %q, want %q", c.name, got, c.want)
		}
	}
}

// Review focus: pasted text never acts, even when it holds bound keys.
func TestPastedTextTriggersNothing(t *testing.T) {
	m := newReboundModel(t, overrides{"sidebar": {"down": {"j"}, "open": {"q"}}})
	for _, text := range []string{"j", "jqj"} {
		nm, cmd := m.Update(tea.PasteMsg{Content: text})
		if cmd != nil {
			t.Errorf("pasting %q must not return a command", text)
		}
		if got := sidebarRows(nm.(model))[0]; got != "❯ ▸ app" || nm.(model).tabs.Len() != 0 {
			t.Errorf("pasting %q must not move the cursor or open anything, first row = %q", text, got)
		}
	}
}

// Review focus: a paste types in the editor, keys and all, and acts on
// nothing.
func TestPasteTypesInTheEditor(t *testing.T) {
	m := press(newTestModel(), "down", "enter") // go.mod open, the editor focused
	m = paste(m, "\x11j\n")                     // ctrl+q's byte, a letter, a line break
	if got, want := m.activeDocument().text.String(), "\x11j\n"+goModText; got != want {
		t.Errorf("after the paste the text is %q, want %q", got, want)
	}
}

// Review focus: capitals and accented letters still type themselves.
func TestTypingCapitalsAndAccents(t *testing.T) {
	m := press(newTestModel(), "down", "enter", "G", "ñ", "é")
	if got, want := m.activeDocument().text.LineText(0), "Gñémodule example.com/project"; got != want {
		t.Errorf("the first line is %q, want %q", got, want)
	}
}

// Review focus: a paste cancels a quit waiting to be pressed again, as any
// key does.
func TestAPasteCancelsAWaitingQuit(t *testing.T) {
	m := newTestModel()
	m.confirm = actQuit
	if m = paste(m, "x"); m.confirm != (keymap.ID{}) {
		t.Error("a paste should cancel the quit waiting for confirmation")
	}
}
