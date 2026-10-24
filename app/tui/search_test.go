package tui

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/candy-tools/trazo/libs/grep"
	"github.com/candy-tools/trazo/libs/picker"
	"github.com/charmbracelet/x/ansi"
)

// notesText is notes.txt's text: four matches of beta, smart case.
const notesText = "alpha beta\n\tbeta gamma beta\nBETA\n"

// notesFS is a workspace for the search tests: a text, and a binary file.
var notesFS = fstest.MapFS{
	"bin.dat":   {Data: []byte("beta\x00")},
	"notes.txt": {Data: []byte(notesText)},
}

// newSearchModel returns a model on fsys with a 100x30 terminal. Its tests
// run the commands Update returns: a search that is never run leaves its
// popup searching.
func newSearchModel(fsys fs.FS) model {
	return send(newModel("/w", fsys, noWrites, testKeys, noColors, noMouse), tea.WindowSizeMsg{Width: 100, Height: 30})
}

// act presses keys, as press does, and runs the commands each key's Update
// returns, as drain does, so the searches the keys start land.
func act(m model, keys ...string) model {
	for _, k := range keys {
		var msgs []tea.KeyPressMsg
		if isKeyName(k) {
			msgs = append(msgs, keyMsg(k))
		} else {
			for _, r := range k {
				msgs = append(msgs, keyMsg(string(r)))
			}
		}
		for _, msg := range msgs {
			var cmd tea.Cmd
			m, cmd = update(m, msg)
			m, _ = drain(m, cmd)
		}
	}
	return m
}

// openNotes returns a search model on notesFS with notes.txt open in the
// editor, its cursor at the start.
func openNotes() model { return act(newSearchModel(notesFS), "down", "enter") }

// viewLines returns the lines of m's view, without styling.
func viewLines(m model) []string { return strings.Split(ansi.Strip(m.View().Content), "\n") }

// popupRows returns the rows inside m's popup, without its border, styling
// or the spaces at their ends.
func popupRows(m model) []string {
	w, h := m.size()
	x, y, width, height := searchBox(w, h)
	var rows []string
	for _, l := range viewLines(m)[y+1 : y+height-1] {
		rows = append(rows, strings.TrimRight(ansi.Cut(l, x+1, x+width-1), " "))
	}
	return rows
}

// listed returns the rows of m's popup below its first, up to the first
// empty one: the matches in view.
func listed(m model) []string {
	rows := popupRows(m)[1:]
	for i, r := range rows {
		if r == "" {
			return rows[:i]
		}
	}
	return rows
}

func TestCtrlFInTheEditorSearchesTheFile(t *testing.T) {
	m := act(openNotes(), "ctrl+f")
	_, y, _, _ := searchBox(m.size())
	if top := viewLines(m)[y]; !strings.Contains(top, "Search notes.txt") {
		t.Errorf("the popup's top border %q should carry Search and the file's name", top)
	}
	m = act(m, "beta")
	if first := popupRows(m)[0]; !strings.HasPrefix(first, " Find: beta") || !strings.HasSuffix(first, "4 matches") {
		t.Errorf("first row %q, want the query and 4 matches", first)
	}
	want := []string{"❯ 1  alpha beta", "  2  beta gamma beta", "  2  beta gamma beta", "  3  BETA"}
	if got := listed(m); !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
	if got, want := footerText(m), " enter: Go to  ↑↓: Move  esc: Close"; got != want {
		t.Errorf("footer %q, want %q", got, want)
	}
}

func TestCtrlFInTheEditorNeedsAText(t *testing.T) {
	if m := act(newSearchModel(notesFS), "shift+tab", "ctrl+f"); m.search != nil {
		t.Error("with no tab open, ctrl+f in the editor should open nothing")
	}
	if m := act(newSearchModel(notesFS), "enter", "ctrl+f"); m.search != nil {
		t.Error("on bin.dat's tab, which shows a note, ctrl+f should open nothing")
	}
}

func TestTheListStartsOnTheFirstMatchAfterTheCursor(t *testing.T) {
	for _, c := range []struct {
		name         string
		moves        []string // put the editor's cursor somewhere first
		line, column int      // the match the list starts on
	}{
		{"from the start", nil, 0, 6},
		{"on a match, the next one", []string{"down", "right"}, 1, 12},
		{"past the last match, the first", []string{"down", "down", "end"}, 0, 6},
	} {
		m := act(act(openNotes(), c.moves...), "ctrl+f", "beta")
		if match, _ := m.search.picker.Current(); match.Line != c.line || match.Column != c.column {
			t.Errorf("%s: the list starts on line %d column %d, want line %d column %d", c.name, match.Line, match.Column, c.line, c.column)
		}
	}
}

func TestEnterGoesToTheMatch(t *testing.T) {
	m := act(openNotes(), "ctrl+f", "beta", "down", "down", "enter")
	if m.search != nil || m.focus != paneEditor {
		t.Fatal("enter should close the popup and leave the editor focused")
	}
	if line, offset := cursor(m); line != 1 || offset != 12 {
		t.Errorf("cursor at line %d offset %d, want the third match's, line 1 offset 12", line, offset)
	}
	if m = act(m, "ctrl+f", "zzz", "enter"); m.search == nil {
		t.Error("enter with no match should leave the popup open")
	}
}

// Review focus: in a CRLF file, enter puts the cursor on the match itself.
func TestEnterInACRLFFile(t *testing.T) {
	m := act(newSearchModel(fstest.MapFS{"crlf.txt": {Data: []byte("one\r\ntwo beta\r\n")}}), "enter", "ctrl+f", "beta", "enter")
	if line, offset := cursor(m); line != 1 || offset != 4 {
		t.Errorf("cursor at line %d offset %d, want line 1 offset 4", line, offset)
	}
}

func TestTheLastQueryComesBackSelected(t *testing.T) {
	m := act(openNotes(), "ctrl+f", "beta", "esc")
	if m.search != nil || m.query != "beta" {
		t.Fatalf("esc should close the popup and keep its query, got %q", m.query)
	}
	if m = act(m, "ctrl+f"); !m.search.picker.Prefilled() || len(listed(m)) != 4 {
		t.Error("the popup should open with beta selected, and its matches")
	}
	for _, c := range []struct {
		keys []string
		want string
	}{
		{[]string{"g"}, "g"},
		{[]string{"backspace"}, ""},
		{[]string{"delete"}, ""},
		{[]string{"left", "x"}, "betxa"},
		{[]string{"down", "x"}, "x"}, // a move in the list keeps it selected
	} {
		m := act(openNotes(), "ctrl+f", "beta", "esc", "ctrl+f")
		if got := act(m, c.keys...).search.picker.Query(); got != c.want {
			t.Errorf("%q: query %q, want %q", c.keys, got, c.want)
		}
	}
}

// The query the popup opens with is drawn, not just kept: sized after the
// query was put in, its window would show nothing but the cursor's cell.
func TestTheLastQueryIsDrawnWhenItComesBack(t *testing.T) {
	m := act(openNotes(), "ctrl+f", "beta", "esc", "ctrl+f")
	if first := popupRows(m)[0]; !strings.HasPrefix(first, " Find: beta") {
		t.Errorf("first row %q, want the last query, beta", first)
	}
}

// A paste types in the query and searches, as typed text does.
func TestAPasteSearches(t *testing.T) {
	m, cmd := update(act(openNotes(), "ctrl+f"), tea.PasteMsg{Content: "gamma"})
	m, _ = drain(m, cmd)
	if got, want := listed(m), []string{"❯ 2  beta gamma beta"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
}

func TestTheRowsStayUntilTheNewResultsLand(t *testing.T) {
	m := act(openNotes(), "ctrl+f", "beta")
	m, cmd := update(m, keyMsg("x"))
	if len(listed(m)) != 4 || !strings.HasSuffix(popupRows(m)[0], "searching…") {
		t.Errorf("before betax's results land: rows %q; want beta's, and searching…", popupRows(m))
	}
	m, _ = drain(m, cmd)
	if len(listed(m)) != 0 || !strings.HasSuffix(popupRows(m)[0], "no matches") {
		t.Errorf("once they land: rows %q; want none, and no matches", popupRows(m))
	}
}

func TestAnOlderSearchsResultsAreDropped(t *testing.T) {
	m := act(openNotes(), "ctrl+f", "beta")
	before := listed(m)
	m = send(m, searchFoundMsg{n: m.searches - 1, matches: []grep.Match{{Line: 2, Text: "BETA", End: 4}}, files: 1, done: true})
	if got := listed(m); !slices.Equal(got, before) {
		t.Errorf("rows %q after an older search's results, want %q", got, before)
	}
}

func TestThePopupTakesEveryKey(t *testing.T) {
	m := act(openNotes(), "ctrl+f", "q")
	if got := m.activeDocument().text.String(); got != notesText {
		t.Errorf("typed text reached the file: %q", got)
	}
	if _, quit := m.keyPress(keyMsg("ctrl+q")); quit != nil {
		t.Error("ctrl+q should not quit while the popup is open")
	}
	if m = act(m, "shift+tab", "ctrl+pgdown"); m.search == nil || m.focus != paneEditor {
		t.Error("the global keys should do nothing while the popup is open")
	}
	if m = act(m, "esc"); m.search != nil {
		t.Fatal("esc should close the popup")
	}
	if m = press(m, "esc"); m.tabs.Len() != 1 {
		t.Error("esc twice should close the popup, not the tab")
	}
}

func TestAFastDoubleEscAndCtrlCClose(t *testing.T) {
	for _, key := range []string{"alt+esc", "ctrl+c"} {
		if m := act(openNotes(), "ctrl+f", key); m.search != nil || m.tabs.Len() != 1 || m.pending != "" {
			t.Errorf("%s should close the popup, and nothing else", key)
		}
	}
}

func TestTheCloseKeyCanBeRebound(t *testing.T) {
	m := send(newModel("/w", notesFS, noWrites, rebound(t, overrides{"search": {"close": {"ctrl+g"}}}), noColors, noMouse),
		tea.WindowSizeMsg{Width: 100, Height: 30})
	if m = act(m, "down", "enter", "ctrl+f", "ctrl+g"); m.search != nil {
		t.Error("ctrl+g, search.close's key, should close the popup")
	}
	if _, err := NewKeymap(overrides{"search": {"close": {}}}); err == nil || err.Error() != "search.close: needs at least one key" {
		t.Errorf("NewKeymap = %v, want search.close to need a key", err)
	}
}

func TestTheMouseDoesNothingWhileThePopupIsOpen(t *testing.T) {
	m := send(newModel("/w", notesFS, noWrites, testKeys, noColors, true), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = act(m, "enter", "shift+tab", "down", "enter", "ctrl+f") // bin.dat, then notes.txt, the active tab
	m = click(t, m, tea.MouseLeft, "bin.dat", 0)
	m = send(m, tea.MouseWheelMsg{X: 50, Y: 10, Button: tea.MouseWheelDown})
	if active, _ := m.tabs.Active(); active.Key != "notes.txt" || m.search == nil {
		t.Errorf("the active tab is %s, the popup open %t: a click must not switch tabs under the popup", active.Key, m.search != nil)
	}
}

// Review focus: the popup covers its box, and nothing else of the frame
// changes but the footer and the panes' focus.
func TestThePopupCoversItsBoxOnly(t *testing.T) {
	for _, size := range []struct{ w, h int }{{80, 24}, {100, 30}} {
		m := send(openNotes(), tea.WindowSizeMsg{Width: size.w, Height: size.h})
		before := viewLines(m)
		after := viewLines(act(m, "ctrl+f", "beta"))
		x, y, width, height := searchBox(size.w, size.h)
		if len(after) != len(before) {
			t.Fatalf("%dx%d: %d lines with the popup, %d without", size.w, size.h, len(after), len(before))
		}
		for i := range len(before) - 1 { // the footer shows the popup's keys
			outside := func(l string) string { return ansi.Cut(l, 0, x) + "|" + ansi.Cut(l, x+width, size.w) }
			if i >= y && i < y+height {
				if outside(after[i]) != outside(before[i]) {
					t.Errorf("%dx%d line %d: %q beside the popup, want %q", size.w, size.h, i, outside(after[i]), outside(before[i]))
				}
				continue
			}
			if after[i] != before[i] {
				t.Errorf("%dx%d line %d: %q, want %q", size.w, size.h, i, after[i], before[i])
			}
		}
		if corner := ansi.Cut(after[y], x, x+1); corner != "╭" {
			t.Errorf("%dx%d: the popup's corner is %q at column %d, row %d", size.w, size.h, corner, x, y)
		}
	}
}

// Review focus: a resize while the popup is open keeps it on the screen,
// its list sized to it.
func TestAResizeWhileThePopupIsOpen(t *testing.T) {
	m := send(act(openNotes(), "ctrl+f", "beta"), tea.WindowSizeMsg{Width: 40, Height: 8})
	for i, l := range viewLines(m) {
		if w := ansi.StringWidth(l); w > 40 {
			t.Errorf("line %d is %d cells wide in a 40-cell terminal", i, w)
		}
	}
	if items, _ := m.search.picker.Window(); len(items) != 1 {
		t.Errorf("the list shows %d rows in a popup 4 rows tall, want 1", len(items))
	}
}

func TestATinyScreen(t *testing.T) {
	m := send(act(openNotes(), "ctrl+f", "beta"), tea.WindowSizeMsg{Width: 20, Height: 6})
	if first := popupRows(m)[0]; !strings.HasPrefix(first, " Find: beta") {
		t.Errorf("first row %q in a 20x6 terminal", first)
	}
}

func TestSearchBox(t *testing.T) {
	for _, c := range []struct{ w, h, x, y, width, height int }{
		{80, 24, 2, 2, 76, 20},
		{100, 30, 2, 2, 96, 26},
		{300, 80, 100, 25, 100, 30},
		{20, 6, 0, 1, 20, 3},
		{10, 2, 0, 0, 10, 2},
	} {
		if x, y, width, height := searchBox(c.w, c.h); x != c.x || y != c.y || width != c.width || height != c.height {
			t.Errorf("searchBox(%d, %d) = %d, %d, %d, %d; want %d, %d, %d, %d", c.w, c.h, x, y, width, height, c.x, c.y, c.width, c.height)
		}
	}
}

func TestMatchLine(t *testing.T) {
	hl := matchStyle.Render
	for _, c := range []struct {
		name string
		m    grep.Match
		room int
		want string
	}{
		{"the indentation dropped", grep.Match{Text: "\t  beta x", Start: 3, End: 7}, 20, hl("beta") + " x"},
		{"but never the match", grep.Match{Text: "  x", Start: 1, End: 3}, 20, hl(" x")},
		{"tabs and control characters", grep.Match{Text: "a\tb\x01beta", Start: 4, End: 8}, 20, "a b?" + hl("beta")},
		{"a long line cut at its end", grep.Match{Text: "beta " + strings.Repeat("x", 30), End: 4}, 12, hl("beta") + " xxxxxx…"},
		{"and at its start too", grep.Match{Text: strings.Repeat("x", 30) + "beta", Start: 30, End: 34}, 12, "…xx" + hl("beta")},
		{"no room", grep.Match{Text: "beta", End: 4}, 0, ""},
	} {
		if got := matchLine(c.m, c.room); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// The match is bold and underlined, the attributes that show without colors.
func TestTheMatchIsBoldAndUnderlined(t *testing.T) {
	if !matchStyle.GetBold() || !matchStyle.GetUnderline() {
		t.Errorf("matchStyle is bold %t, underlined %t; want both", matchStyle.GetBold(), matchStyle.GetUnderline())
	}
}

func TestFitPlace(t *testing.T) {
	for _, c := range []struct {
		place  string
		col    int
		isPath bool
		want   string
	}{
		{"a.go:7", 10, true, "a.go:7    "},
		{"7", 3, false, "  7"},
		{"app/tui/keys.go:7", 10, true, "…keys.go:7"},
		{"a日本.go:1", 7, true, "….go:1 "}, // 本 straddles the cut: cut it whole
	} {
		if got := fitPlace(c.place, c.col, c.isPath); got != c.want {
			t.Errorf("fitPlace(%q, %d, %t) = %q, want %q", c.place, c.col, c.isPath, got, c.want)
		}
	}
}

func TestTheCountAndTheTitle(t *testing.T) {
	file := &document{path: "app/keys.go"}
	for _, c := range []struct {
		doc            *document
		query          string
		running, more  bool
		err            error
		matches, files int
		want           string
	}{
		{file, "", false, false, nil, 0, 0, ""},
		{file, "x", true, false, nil, 0, 0, "searching…"},
		{nil, "x", false, false, &fs.PathError{Op: "open", Path: ".", Err: fs.ErrPermission}, 0, 0, "⚠ permission denied"},
		{file, "x", false, false, nil, 0, 0, "no matches"},
		{file, "x", false, false, nil, 1, 1, "1 match"},
		{file, "x", false, true, nil, 10000, 1, "10000+ matches"},
		{nil, "x", false, false, nil, 1, 1, "1 match in 1 file"},
		{nil, "x", false, true, nil, 1000, 37, "1000+ matches in 37 files"},
	} {
		p := &searchPopup{doc: c.doc, picker: picker.New[grep.Match](), running: c.running, more: c.more, err: c.err, matches: c.matches, files: c.files}
		p.picker.Type(c.query)
		if got := p.count(); got != c.want {
			t.Errorf("count %q, want %q", got, c.want)
		}
	}
	if got := (&searchPopup{doc: file}).title(); got != "Search keys.go" {
		t.Errorf("a file's title %q", got)
	}
	if got := (&searchPopup{}).title(); got != "Search" {
		t.Errorf("the workspace's title %q", got)
	}
}

// waitFound takes every message already sent as one and waits for no more,
// which a search that sends its matches in several messages, as the
// workspace's does, relies on; a file's search sends one.
func TestWaitFoundMergesWhatIsReady(t *testing.T) {
	a, b := grep.Match{Path: "a", Line: 1}, grep.Match{Path: "b", Line: 2}
	for _, c := range []struct {
		name   string
		sent   []searchFoundMsg
		closed bool
		want   tea.Msg
	}{
		{"every message ready, merged in order", []searchFoundMsg{
			{n: 7, matches: []grep.Match{a}, files: 1},
			{n: 7, matches: []grep.Match{b}, files: 1},
			{n: 7, done: true, more: true, err: fs.ErrPermission},
		}, false, searchFoundMsg{n: 7, matches: []grep.Match{a, b}, files: 2, done: true, more: true, err: fs.ErrPermission}},
		{"what isn't sent yet isn't waited for", []searchFoundMsg{{n: 7, matches: []grep.Match{a}, files: 1}}, false,
			searchFoundMsg{n: 7, matches: []grep.Match{a}, files: 1}},
		{"a channel closed after some", []searchFoundMsg{{n: 7, matches: []grep.Match{a}, files: 1}}, true,
			searchFoundMsg{n: 7, matches: []grep.Match{a}, files: 1}},
		{"a channel closed with nothing", nil, true, nil},
	} {
		found := make(chan searchFoundMsg, len(c.sent))
		for _, msg := range c.sent {
			found <- msg
		}
		if c.closed {
			close(found)
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- waitFound(found)() }()
		select {
		case got := <-done:
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: waitFound waited for results nobody sent", c.name)
		}
	}
}

// The first matches of a query replace the rows, later ones add to them, in
// order, and the last ends the search.
func TestMatchesInPartsAddUp(t *testing.T) {
	m := act(openNotes(), "ctrl+f", "beta")
	found := make(chan searchFoundMsg, 1)
	m.search.found, m.search.running, m.search.fresh = found, true, true // a search the test feeds
	part := func(line int, done bool) searchFoundMsg {
		return searchFoundMsg{n: m.searches, matches: []grep.Match{{Line: line, Text: "beta", End: 4}}, files: 1, done: done}
	}
	found <- part(8, false)
	m, cmd := update(m, waitFound(found)())
	if got, want := listed(m), []string{"❯ 9  beta"}; !slices.Equal(got, want) || !strings.HasSuffix(popupRows(m)[0], "searching…") {
		t.Errorf("after the first part: rows %q, want %q, and searching…", got, want)
	}
	found <- part(11, true)
	m, _ = drain(m, cmd)
	if got, want := listed(m), []string{"❯  9  beta", "  12  beta"}; !slices.Equal(got, want) || !strings.HasSuffix(popupRows(m)[0], "2 matches") {
		t.Errorf("after the last: rows %q, want %q, and 2 matches", got, want)
	}
	if m.search.found != nil {
		t.Error("the search is over: the popup should wait for no more results")
	}
}

// A popup with no file, as the workspace's is, lists each match under its
// path, and its footer says Open.
func TestAPopupWithoutAFileListsPaths(t *testing.T) {
	m := newSearchModel(notesFS)
	m.search = &searchPopup{picker: picker.New[grep.Match](), matches: 2, files: 2}
	m.fitSearch()
	m.search.picker.Type("beta")
	m.search.picker.SetItems([]grep.Match{
		{Path: "app/main.go", Line: 2, Text: "// beta", Start: 3, End: 7},
		{Path: "README.md", Text: "# beta", Start: 2, End: 6},
	})
	if got, want := listed(m), []string{"❯ app/main.go:3  // beta", "  README.md:1    # beta"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
	if first := popupRows(m)[0]; !strings.HasSuffix(first, "2 matches in 2 files") {
		t.Errorf("first row %q, want 2 matches in 2 files", first)
	}
	if got, want := footerText(m), " enter: Open  ↑↓: Move  esc: Close"; got != want {
		t.Errorf("footer %q, want %q", got, want)
	}
}

// workspaceFS is a workspace for the workspace search: folders, a
// .gitignore, git's own folder, a binary file and a log git ignores.
var workspaceFS = fstest.MapFS{
	".git/config":   {Data: []byte("needle in git\n")},
	".gitignore":    {Data: []byte("*.log\nbuild/\n")},
	"README.md":     {Data: []byte("# needle\n")},
	"app/main.go":   {Data: []byte("package main\n\n// needle\n")},
	"app/x.log":     {Data: []byte("needle in a log\n")},
	"build/out.txt": {Data: []byte("needle built\n")},
	"data.bin":      {Data: []byte("needle\x00")},
	"zz/two.txt":    {Data: []byte("a needle and a needle\n")},
}

// newWorkspaceModel returns a search model on fsys whose typing pauses are
// over at once.
func newWorkspaceModel(fsys fs.FS) model {
	m := newModel("/w", fsys, noWrites, testKeys, noColors, noMouse)
	m.pause = instantPause
	return send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
}

// instantPause makes typing pauses that are over at once.
func instantPause(n int) tea.Cmd {
	return func() tea.Msg { return searchDueMsg{n} }
}

// needles is what a search for needle in workspaceFS lists.
var needles = []string{
	"❯ app/main.go:3  // needle",
	"  zz/two.txt:1   a needle and a needle",
	"  zz/two.txt:1   a needle and a needle",
	"  README.md:1    # needle",
}

func TestCtrlFInTheSidebarSearchesTheWorkspace(t *testing.T) {
	m := act(newWorkspaceModel(workspaceFS), "ctrl+f")
	_, y, _, _ := searchBox(m.size())
	if top := viewLines(m)[y]; !strings.Contains(top, "─ Search ─") {
		t.Errorf("the popup's top border %q should carry Search alone", top)
	}
	m = act(m, "needle")
	if first := popupRows(m)[0]; !strings.HasSuffix(first, "4 matches in 3 files") {
		t.Errorf("first row %q, want 4 matches in 3 files", first)
	}
	if got := listed(m); !slices.Equal(got, needles) {
		t.Errorf("rows %q, want %q: in the sidebar's order, without .git, what git ignores and the binary file", got, needles)
	}
	if got, want := footerText(m), " enter: Open  ↑↓: Move  esc: Close"; got != want {
		t.Errorf("footer %q, want %q", got, want)
	}
}

func TestTheWorkspaceSearchWaitsForAPause(t *testing.T) {
	m := newWorkspaceModel(workspaceFS)
	var paused []int
	m.pause = func(n int) tea.Cmd {
		paused = append(paused, n)
		return nil
	}
	m = act(m, "ctrl+f", "ne")
	if len(paused) != 2 || m.search.found != nil || !strings.HasSuffix(popupRows(m)[0], "searching…") {
		t.Fatalf("each key should wait for its own pause and start no search: pauses %v", paused)
	}
	if m, _ = update(m, searchDueMsg{paused[0]}); m.search.found != nil {
		t.Error("the pause of a query typed over should start nothing")
	}
	m, cmd := update(m, searchDueMsg{paused[1]})
	if m, _ = drain(m, cmd); len(listed(m)) != 4 {
		t.Errorf("the last pause's search should list the matches, rows %q", listed(m))
	}
}

// The model's own pause, which a key starts and which ends searchPause
// later: the search for the query starts then, and not before.
func TestTheTypingPauseEndsForItsSearch(t *testing.T) {
	m := act(newSearchModel(workspaceFS), "ctrl+f")
	start := time.Now()
	m, cmd := update(m, keyMsg("#")) // the pause's timer runs from here
	if m.search.found != nil || len(listed(m)) != 0 {
		t.Fatal("the key should start a pause, and no search")
	}
	m, _ = drain(m, cmd)
	if waited := time.Since(start); waited < searchPause {
		t.Errorf("the pause lasted %v, want at least %v", waited, searchPause)
	}
	if got, want := listed(m), []string{"❯ README.md:1  # needle"}; !slices.Equal(got, want) {
		t.Errorf("the search should start when the pause ends: rows %q, want %q", got, want)
	}
}

// A file's search has no walk to hold back: typing in its popup searches at
// once.
func TestAFilesSearchDoesNotWaitForAPause(t *testing.T) {
	m := openNotes()
	m.pause = func(int) tea.Cmd {
		t.Error("typing in a file's popup should start no pause")
		return nil
	}
	if got := listed(act(m, "ctrl+f", "beta")); len(got) != 4 {
		t.Errorf("rows %q, want the 4 matches", got)
	}
}

func TestReopeningDropsTheClosedPopupsPause(t *testing.T) {
	m := newWorkspaceModel(workspaceFS)
	var paused []int
	m.pause = func(n int) tea.Cmd {
		paused = append(paused, n)
		return nil
	}
	m = act(m, "ctrl+f", "needle", "esc", "ctrl+f") // the popup opens with needle and searches at once
	if got := listed(m); !slices.Equal(got, needles) {
		t.Fatalf("rows %q, want %q", got, needles)
	}
	if m, _ = update(m, searchDueMsg{paused[len(paused)-1]}); m.search.found != nil || m.search.running {
		t.Error("the closed popup's pause, landing late, should start nothing")
	}
}

// gateFS is fsys, each Open but the root's waiting until gate closes.
type gateFS struct {
	fsys fs.FS
	gate chan struct{}
}

func (f gateFS) Open(name string) (fs.File, error) {
	if name != "." {
		<-f.gate
	}
	return f.fsys.Open(name)
}

// Review focus: typing, or closing the popup, while a workspace search
// walks stops that search: it sends nothing more and closes its channel.
func TestAStoppedSearchSendsNothing(t *testing.T) {
	for _, stop := range []string{"e", "esc"} {
		gate := make(chan struct{})
		m := act(newWorkspaceModel(gateFS{workspaceFS, gate}), "ctrl+f")
		m, _ = update(m, keyMsg("n"))
		m, _ = update(m, searchDueMsg{m.searches}) // the search starts, and waits at the gate
		running := m.search.found
		_, _ = update(m, keyMsg(stop))
		close(gate)
		if _, ok := <-running; ok {
			t.Errorf("%s: the stopped search sent results; it should close its channel with nothing on it", stop)
		}
	}
}

func TestAnOpenTabIsSearchedAsItHoldsTheFile(t *testing.T) {
	m := newWorkspaceModel(workspaceFS)
	m.tick = instantTick
	m = act(m, "end", "enter", "ctrl+k", "fresh", "shift+tab") // README.md, its line replaced, unsaved
	m = act(m, "ctrl+f", "needle")
	for _, row := range listed(m) {
		if strings.Contains(row, "README.md") {
			t.Errorf("row %q: README.md's text on disk should not be searched, its tab's should", row)
		}
	}
	if got, want := listed(act(m, "esc", "ctrl+f", "fresh")), []string{"❯ README.md:1  fresh"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
}

func TestEnterOpensTheMatchsFile(t *testing.T) {
	m := act(newWorkspaceModel(workspaceFS), "ctrl+f", "needle", "down", "down", "enter")
	if m.search != nil || m.focus != paneEditor || m.activeDocument().path != "zz/two.txt" {
		t.Fatal("enter should open zz/two.txt, focus the editor and close the popup")
	}
	if line, offset := cursor(m); line != 0 || offset != 15 {
		t.Errorf("cursor at line %d offset %d, want the second needle's, offset 15", line, offset)
	}
	m = act(m, "shift+tab", "ctrl+f", "enter")         // app/main.go: a second tab
	m = act(m, "shift+tab", "ctrl+f", "down", "enter") // zz/two.txt again: its tab
	if m.tabs.Len() != 2 || m.activeDocument().path != "zz/two.txt" {
		t.Errorf("%d tabs, %s active: a match in an open file should switch to its tab", m.tabs.Len(), m.activeDocument().path)
	}
}

// The query of a workspace search carries into a file's: the editor's popup
// opens with it, selected, and searches the file for it.
func TestTheQueryCarriesIntoAFilesSearch(t *testing.T) {
	m := act(newWorkspaceModel(workspaceFS), "ctrl+f", "needle", "enter") // app/main.go, the first row
	m = act(m, "ctrl+f")
	if got := m.search.picker.Query(); got != "needle" || !m.search.picker.Prefilled() {
		t.Errorf("query %q, selected %t; want needle, selected", got, m.search.picker.Prefilled())
	}
	if got, want := listed(m), []string{"❯ 3  // needle"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
}

// Review focus: enter on a match of a file that is no text any more, since
// the search, opens its tab with the note: there is no cursor to move.
func TestEnterOnAMatchOfAFileThatChanged(t *testing.T) {
	fsys := fstest.MapFS{"a.txt": {Data: []byte("needle\n")}}
	m := act(newWorkspaceModel(fsys), "ctrl+f", "needle")
	fsys["a.txt"] = &fstest.MapFile{Data: []byte("needle\x00")} // binary now
	m = act(m, "enter")
	if doc := m.activeDocument(); m.search != nil || m.focus != paneEditor || doc == nil || doc.path != "a.txt" || doc.note != "binary file" {
		t.Errorf("enter should close the popup and open a.txt's tab with its note, focusing the editor: %+v", doc)
	}
}

// newHeldModel returns a workspace model whose typing pauses are held: each
// is recorded in paused, and ends only when the test delivers it, as
// lastPause does.
func newHeldModel(paused *[]int) model {
	m := newWorkspaceModel(workspaceFS)
	m.pause = func(n int) tea.Cmd {
		*paused = append(*paused, n)
		return nil
	}
	return m
}

// lastPause delivers the last of paused, the typing pauses a test held, and
// runs the search it starts.
func lastPause(m model, paused []int) model {
	m, cmd := update(m, searchDueMsg{paused[len(paused)-1]})
	m, _ = drain(m, cmd)
	return m
}

// Review focus: enter pressed before the matches of what was just typed are
// listed waits for them, then goes to the match the list starts on.
func TestEnterWaitsForTheQuerysOwnMatches(t *testing.T) {
	var paused []int
	m := act(newHeldModel(&paused), "ctrl+f", "package", "enter")
	if m.search == nil || m.tabs.Len() != 0 {
		t.Fatalf("enter should wait for the matches: the popup is open %t, %d tabs", m.search != nil, m.tabs.Len())
	}
	m = lastPause(m, paused)
	if doc := m.activeDocument(); m.search != nil || m.focus != paneEditor || doc == nil || doc.path != "app/main.go" {
		t.Fatalf("once the matches land, the popup should close with app/main.go open and the editor focused: the popup is open %t, %d tabs", m.search != nil, m.tabs.Len())
	}
	if line, offset := cursor(m); line != 0 || offset != 0 {
		t.Errorf("cursor at line %d offset %d, want the match's, line 0 offset 0", line, offset)
	}
}

// When the search of a waiting enter finds no match, the popup stays open,
// saying so.
func TestAWaitingEnterWithNoMatchesStays(t *testing.T) {
	var paused []int
	m := act(newHeldModel(&paused), "ctrl+f", "zzz", "enter")
	m = lastPause(m, paused)
	if m.search == nil || m.tabs.Len() != 0 {
		t.Fatalf("the popup should stay open and open no tab: it is open %t, %d tabs", m.search != nil, m.tabs.Len())
	}
	if first := popupRows(m)[0]; !strings.HasSuffix(first, "no matches") {
		t.Errorf("first row %q, want it to end with no matches", first)
	}
}

// Review focus: a key that changes the query drops the enter that waits.
func TestAQueryChangeDropsAWaitingEnter(t *testing.T) {
	var paused []int
	m := act(newHeldModel(&paused), "ctrl+f", "pac", "enter", "k")
	m = lastPause(m, paused)
	if m.search == nil || m.tabs.Len() != 0 {
		t.Fatalf("the popup should stay open and open no tab: it is open %t, %d tabs", m.search != nil, m.tabs.Len())
	}
	if got, want := listed(m), []string{"❯ app/main.go:1  package main"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
}

// Review focus: enter while the previous query's rows are showing waits
// for the new query's own matches, and never goes to one of the old rows.
func TestEnterDoesNotGoToThePreviousQuerysMatches(t *testing.T) {
	var paused []int
	m := act(newHeldModel(&paused), "ctrl+f", "needle")
	m = lastPause(m, paused)    // needle's 4 rows
	m = act(m, "esc", "ctrl+f") // reopens with needle's rows
	if len(listed(m)) != 4 {
		t.Fatalf("setup: rows %q", listed(m))
	}
	m = act(m, "zzz", "enter") // zzz's pause is held
	if m.search == nil || m.tabs.Len() != 0 {
		t.Fatalf("enter went to the previous query's match: popup open %t, %d tabs", m.search != nil, m.tabs.Len())
	}
	m = lastPause(m, paused)
	if m.search == nil || m.tabs.Len() != 0 || !strings.HasSuffix(popupRows(m)[0], "no matches") {
		t.Errorf("popup open %t, %d tabs, first row %q", m.search != nil, m.tabs.Len(), popupRows(m)[0])
	}
}

// Review focus: the workspace search stops at workspaceMatches matches, and
// leaves out a file over the size a tab shows, whose matches would come
// first here.
func TestTheWorkspaceSearchHasLimits(t *testing.T) {
	m := newWorkspaceModel(fstest.MapFS{
		"huge.txt": {Data: bytes.Repeat([]byte("needle\n"), maxDocumentSize/len("needle\n")+1)},
		"many.txt": {Data: []byte(strings.Repeat("needle\n", workspaceMatches+1))},
	})
	m = act(m, "ctrl+f", "needle")
	if first, want := popupRows(m)[0], "1000+ matches in 1 file"; !strings.HasSuffix(first, want) {
		t.Errorf("first row %q, want it to end with %q", first, want)
	}
	if got, want := listed(m)[0], "❯ many.txt:1 "; !strings.HasPrefix(got, want) {
		t.Errorf("first match %q, want it to start with %q: huge.txt is over the size limit", got, want)
	}
}

func TestIgnorePatternsFromTheRepositoryAbove(t *testing.T) {
	m := newWorkspaceModel(fstest.MapFS{"a.md": {Data: []byte("needle\n")}, "b.txt": {Data: []byte("needle\n")}})
	m.ignores, m.inside = fstest.MapFS{".gitignore": {Data: []byte("*.md\n")}}, "sub"
	if got, want := listed(act(m, "ctrl+f", "needle")), []string{"❯ b.txt:1  needle"}; !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
}

// Review focus: a file name holding a control character or an escape
// sequence is drawn safe, and the popup keeps its width.
func TestAStrangeFileNameIsDrawnSafe(t *testing.T) {
	m := act(newWorkspaceModel(fstest.MapFS{"a\x1b[31mb.txt": {Data: []byte("needle\n")}}), "ctrl+f", "needle")
	if got := listed(m); len(got) != 1 || !strings.HasPrefix(got[0], "❯ a?[31mb.txt:1") {
		t.Errorf("rows %q, want the name with its escape as ?", got)
	}
	for i, l := range viewLines(m) {
		if w := ansi.StringWidth(l); w > 100 {
			t.Errorf("line %d is %d cells wide in a 100-cell terminal", i, w)
		}
	}
}

func TestAWorkspaceItCantReadShowsWhy(t *testing.T) {
	m := newWorkspaceModel(deniedFS{})
	if first := popupRows(act(m, "ctrl+f", "needle"))[0]; !strings.HasSuffix(first, "⚠ permission denied") {
		t.Errorf("first row %q, want the cause", first)
	}
}

func TestRepository(t *testing.T) {
	root := t.TempDir()
	mkdirs := func(dirs ...string) {
		for _, d := range dirs {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	repo, tree, none := filepath.Join(root, "repo"), filepath.Join(root, "tree"), filepath.Join(root, "none")
	mkdirs(filepath.Join(repo, ".git"), filepath.Join(repo, "a", "b"), filepath.Join(tree, "x"), none)
	if err := os.WriteFile(filepath.Join(tree, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ dir, wantDir, wantInside string }{
		{filepath.Join(repo, "a", "b"), repo, "a/b"},
		{repo, repo, "."},
		{filepath.Join(tree, "x"), tree, "x"}, // a worktree's .git is a file
		{none, none, "."},
	} {
		if dir, inside := repository(c.dir); dir != c.wantDir || inside != c.wantInside {
			t.Errorf("repository(%s) = %s, %s; want %s, %s", c.dir, dir, inside, c.wantDir, c.wantInside)
		}
	}
}
