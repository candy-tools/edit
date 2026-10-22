package textedit

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// numbered returns a text of n lines, "l0" to "l<n-1>", each ended by "\n".
func numbered(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "l%d\n", i)
	}
	return b.String()
}

// sized returns a Buffer holding text, with tab stops 4 columns apart and a
// window w columns wide and h lines tall.
func sized(text string, w, h int) *Buffer {
	b := New(text, 4)
	b.SetSize(w, h)
	return b
}

// rows returns b's window as a terminal shows it: each line's cells, the
// cursor's included.
func rows(b *Buffer) []string {
	var out []string
	for _, l := range b.Window(nil) {
		out = append(out, cells(l))
	}
	return out
}

// cells returns the text of all of l's runs.
func cells(l Line) string {
	var s strings.Builder
	for _, r := range l.Runs {
		s.WriteString(r.Text)
	}
	return s.String()
}

// split returns l's cells left of the cursor's, the cursor's and right of
// it. On a line without the cursor, they are all left of it.
func split(l Line) (before, cursor, after string) {
	for i, r := range l.Runs {
		switch {
		case l.Cursor < 0 || i < l.Cursor:
			before += r.Text
		case i == l.Cursor:
			cursor = r.Text
		default:
			after += r.Text
		}
	}
	return before, cursor, after
}

// checkCursor fails the test unless b's cursor is on line at offset.
func checkCursor(t *testing.T, b *Buffer, line, offset int) {
	t.Helper()
	if l, o := b.Cursor(); l != line || o != offset {
		t.Errorf("cursor at line %d offset %d, want line %d offset %d", l, o, line, offset)
	}
}

// checkRows fails the test unless b's window shows want, from line first.
func checkRows(t *testing.T, b *Buffer, first int, want ...string) {
	t.Helper()
	lines := b.Window(nil)
	if len(lines) == 0 || lines[0].Number != first || !slices.Equal(rows(b), want) {
		t.Errorf("window = %q from line %d, want %q from line %d", rows(b), number(lines), want, first)
	}
}

// number is the index of the first of lines, or -1 when there are none.
func number(lines []Line) int {
	if len(lines) == 0 {
		return -1
	}
	return lines[0].Number
}

// splitCases are texts and the lines New splits them into, with the line
// break String writes between them and whether it adds one at the end.
var splitCases = []struct {
	text  string
	lines []string
	eol   string
	final bool
}{
	{"", []string{""}, "\n", false},
	{"a", []string{"a"}, "\n", false},
	{"a\n", []string{"a"}, "\n", true},
	{"a\n\n", []string{"a", ""}, "\n", true},
	{"\n", []string{""}, "\n", true},
	{"a\nb", []string{"a", "b"}, "\n", false},
	{"a\r\nb\r\n", []string{"a", "b"}, "\r\n", true},
	{"a\r\nb", []string{"a", "b"}, "\r\n", false},
	{"a\r\nb\n", []string{"a\r", "b"}, "\n", true}, // mixed: each \r stays
	{"a\rb", []string{"a\rb"}, "\n", false},
	{"a\r", []string{"a\r"}, "\n", false},
	{"a\r\r\nb", []string{"a\r", "b"}, "\r\n", false},
	{"\tx\x1b[0m\xff", []string{"\tx\x1b[0m\xff"}, "\n", false},
}

func TestNewSplitsLines(t *testing.T) {
	for _, c := range splitCases {
		b := New(c.text, 4)
		if !slices.Equal(b.lines, c.lines) || b.eol != c.eol || b.final != c.final || b.Len() != len(c.lines) {
			t.Errorf("New(%q): lines %q, eol %q, final %t, Len %d; want %q, %q, %t",
				c.text, b.lines, b.eol, b.final, b.Len(), c.lines, c.eol, c.final)
		}
	}
}

func TestStringRoundTrips(t *testing.T) {
	for _, c := range splitCases {
		if got := New(c.text, 4).String(); got != c.text {
			t.Errorf("New(%q).String() = %q", c.text, got)
		}
	}
}

func FuzzRoundTrip(f *testing.F) {
	for _, c := range splitCases {
		f.Add(c.text)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if got := New(s, 4).String(); got != s {
			t.Errorf("New(%q).String() = %q", s, got)
		}
	})
}

// Review focus: whatever the text, the window's size and the keys pressed,
// the cursor stays on a cell boundary of a line that exists, its line is in
// the window with a cursor cell, no line of the window is wider than the
// window, the window leaves no empty space below the last line while
// earlier lines are hidden, the version changes exactly when the text does,
// each edit that changes the text reports one change that turns the old
// text into the new, and Size is the text's length.
func FuzzEdits(f *testing.F) {
	f.Add("ab\ncd", []byte{0, 1, 2, 3, 9, 10, 13, 14, 15, 16})
	f.Add("e\U00000301\xff\t日👨👩\r\nx", []byte{1, 1, 1, 12, 5, 0, 13, 11, 7, 6, 0x50})
	f.Add(numbered(9), []byte{9, 15, 15, 7, 6, 0xd0, 3, 15})
	f.Add("a\r\nb\r\n", []byte{15, 11, 10, 9, 15, 13, 14, 15})
	f.Fuzz(func(t *testing.T, text string, ops []byte) {
		b := sized(text, 7, 3)
		changes := record(b)
		for _, op := range ops {
			s, v := b.String(), b.Version()
			*changes = (*changes)[:0]
			edit(b, op)
			if changed := b.String() != s; changed != (b.Version() != v) {
				t.Fatalf("op %d: the text changed %t, the version went from %d to %d", op, changed, v, b.Version())
			}
			checkChanges(t, b, s, *changes)
			if b.Size() != len(b.String()) {
				t.Fatalf("op %d: Size is %d, String %d bytes", op, b.Size(), len(b.String()))
			}
			checkInvariants(t, b)
		}
	})
}

// record makes b record every change it reports, in the slice it returns.
func record(b *Buffer) *[]Change {
	changes := new([]Change)
	b.OnChange(func(c Change) { *changes = append(*changes, c) })
	return changes
}

// checkChanges fails the test unless changes are what an edit that turned
// old into b's text reported: none when the text is the same, otherwise one
// that turns old into the new text, its positions agreeing with their
// offsets.
func checkChanges(t *testing.T, b *Buffer, old string, changes []Change) {
	t.Helper()
	text := b.String()
	if text == old {
		if len(changes) != 0 {
			t.Fatalf("an edit that changed nothing reported %+v", changes)
		}
		return
	}
	if len(changes) != 1 {
		t.Fatalf("an edit reported %d changes, want 1: %+v", len(changes), changes)
	}
	c := changes[0]
	if c.Start.Offset < 0 || c.OldEnd.Offset < c.Start.Offset || c.OldEnd.Offset > len(old) ||
		c.NewEnd.Offset < c.Start.Offset || c.NewEnd.Offset > len(text) {
		t.Fatalf("change %+v lies outside %q or %q", c, old, text)
	}
	if got := old[:c.Start.Offset] + text[c.Start.Offset:c.NewEnd.Offset] + old[c.OldEnd.Offset:]; got != text {
		t.Fatalf("change %+v turns %q into %q, want %q", c, old, got, text)
	}
	checkPosition(t, old, b.eol, c.OldEnd)
	checkPosition(t, text, b.eol, c.Start)
	checkPosition(t, text, b.eol, c.NewEnd)
}

// checkPosition fails the test unless p's line and column are at its offset
// in text, whose line break is eol.
func checkPosition(t *testing.T, text, eol string, p Position) {
	t.Helper()
	lines := strings.Split(text, eol)
	if p.Line < 0 || p.Line >= len(lines) || p.Column < 0 || p.Column > len(lines[p.Line]) {
		t.Fatalf("position %+v is not in %q", p, text)
	}
	off := p.Column
	for _, l := range lines[:p.Line] {
		off += len(l) + len(eol)
	}
	if off != p.Offset {
		t.Fatalf("position %+v: its line and column are offset %d of %q", p, off, text)
	}
}

func TestOnChangeReportsWhatEachEditChanged(t *testing.T) {
	at := func(off, line, col int) Position { return Position{Offset: off, Line: line, Column: col} }
	for _, c := range []struct {
		name, text string
		before     func(b *Buffer)
		edit       func(b *Buffer)
		want       Change
	}{
		{"insert inside a line", "abc", (*Buffer).Right, func(b *Buffer) { b.Insert("x") },
			Change{at(1, 0, 1), at(1, 0, 1), at(2, 0, 2)}},
		{"insert at the start", "abc", func(*Buffer) {}, func(b *Buffer) { b.Insert("x") },
			Change{at(0, 0, 0), at(0, 0, 0), at(1, 0, 1)}},
		{"insert at the end", "ab\ncd", (*Buffer).Bottom, func(b *Buffer) { b.Insert("x") },
			Change{at(5, 1, 2), at(5, 1, 2), at(6, 1, 3)}},
		{"insert lines into CRLF text", "a\r\nz", (*Buffer).LineEnd, func(b *Buffer) { b.Insert("b\nc\r\nd") },
			Change{at(1, 0, 1), at(1, 0, 1), at(8, 2, 1)}},
		{"delete left inside a line", "abc", func(b *Buffer) { b.Right(); b.Right() }, (*Buffer).DeleteLeft,
			Change{at(1, 0, 1), at(2, 0, 2), at(1, 0, 1)}},
		{"delete left a whole cluster", "ae\U00000301", (*Buffer).LineEnd, (*Buffer).DeleteLeft,
			Change{at(1, 0, 1), at(4, 0, 4), at(1, 0, 1)}},
		{"delete left at a CRLF line start", "ab\r\ncd", (*Buffer).Down, (*Buffer).DeleteLeft,
			Change{at(2, 0, 2), at(4, 1, 0), at(2, 0, 2)}},
		{"delete right inside a line", "abc", (*Buffer).Right, (*Buffer).DeleteRight,
			Change{at(1, 0, 1), at(2, 0, 2), at(1, 0, 1)}},
		{"delete right at a line end", "ab\ncd", (*Buffer).LineEnd, (*Buffer).DeleteRight,
			Change{at(2, 0, 2), at(3, 1, 0), at(2, 0, 2)}},
		{"delete a line in the middle", "one\ntwo\nthree\n", (*Buffer).Down, (*Buffer).DeleteLine,
			Change{at(4, 1, 0), at(8, 2, 0), at(4, 1, 0)}},
		{"delete the last of several lines", "one\r\ntwo", (*Buffer).Bottom, (*Buffer).DeleteLine,
			Change{at(3, 0, 3), at(8, 1, 3), at(3, 0, 3)}},
		{"delete the only line", "one\n", func(*Buffer) {}, (*Buffer).DeleteLine,
			Change{at(0, 0, 0), at(3, 0, 3), at(0, 0, 0)}},
	} {
		b := sized(c.text, 80, 5)
		c.before(b)
		changes := record(b)
		c.edit(b)
		if len(*changes) != 1 || (*changes)[0] != c.want {
			t.Errorf("%s: changes %+v, want %+v", c.name, *changes, c.want)
		}
	}
}

func TestOnChangeOnlyForEditsThatChangeTheText(t *testing.T) {
	b := sized("ab\ncd", 80, 5)
	changes := record(b)
	b.Right()
	b.Down()
	b.PageDown()
	b.SetSize(10, 2)
	b.Insert("")
	b.Top()
	b.DeleteLeft()
	b.Bottom()
	b.DeleteRight()
	empty := New("", 4)
	emptyChanges := record(empty)
	empty.DeleteLine()
	if len(*changes)+len(*emptyChanges) != 0 {
		t.Errorf("moves and edits that change nothing reported %+v and %+v", *changes, *emptyChanges)
	}
	b.Insert("x")
	b.OnChange(nil)
	b.Insert("y")
	if len(*changes) != 1 {
		t.Errorf("changes %+v, want just the one before OnChange(nil)", *changes)
	}
}

func TestVisibleFollowsTheWindow(t *testing.T) {
	b := sized(numbered(10), 80, 3)
	if from, to := b.Visible(); from != 0 || to != 3 {
		t.Errorf("Visible = %d, %d; want 0, 3", from, to)
	}
	b.Bottom()
	if from, to := b.Visible(); from != 7 || to != 10 {
		t.Errorf("at the bottom, Visible = %d, %d; want 7, 10", from, to)
	}
	if from, to := sized("a\nb", 80, 5).Visible(); from != 0 || to != 2 {
		t.Errorf("a text shorter than the window: Visible = %d, %d; want 0, 2", from, to)
	}
}

func TestLineTextAndSize(t *testing.T) {
	b := New("a\r\nbc\r\n", 4)
	if got := []string{b.LineText(0), b.LineText(1)}; !slices.Equal(got, []string{"a", "bc"}) {
		t.Errorf("lines %q, want a and bc without their line breaks", got)
	}
	for _, c := range splitCases {
		if got := New(c.text, 4).Size(); got != len(c.text) {
			t.Errorf("New(%q).Size() = %d, want %d", c.text, got, len(c.text))
		}
	}
}

// edit applies to b the move, edit or resize op picks.
func edit(b *Buffer, op byte) {
	switch op % 17 {
	case 0:
		b.Left()
	case 1:
		b.Right()
	case 2:
		b.Up()
	case 3:
		b.Down()
	case 4:
		b.LineStart()
	case 5:
		b.LineEnd()
	case 6:
		b.PageUp()
	case 7:
		b.PageDown()
	case 8:
		b.Top()
	case 9:
		b.Bottom()
	case 10:
		b.Insert("x")
	case 11:
		b.Insert("\r\n")
	case 12:
		b.Insert("\U0000200D") // a joiner, which can merge clusters
	case 13:
		b.DeleteLeft()
	case 14:
		b.DeleteRight()
	case 15:
		b.DeleteLine()
	case 16:
		b.SetSize(1+int(op>>4)%8, 1+int(op>>6)%3)
	}
}

// checkInvariants fails the test unless b's cursor is on a cell boundary of
// one of its lines, in the window, and the window is no wider than b's.
func checkInvariants(t *testing.T, b *Buffer) {
	t.Helper()
	line, off := b.Cursor()
	if line < 0 || line >= b.Len() {
		t.Fatalf("cursor on line %d of %d", line, b.Len())
	}
	s, i := b.lines[line], 0
	for i < off && i < len(s) {
		size, _, _ := b.cell(s[i:], 0)
		i += size
	}
	if i != off {
		t.Fatalf("cursor at offset %d of %q, not a cell boundary", off, s)
	}
	window := b.Window(nil)
	if line < number(window) || line >= number(window)+len(window) {
		t.Fatalf("cursor on line %d, window from line %d, %d lines", line, number(window), len(window))
	}
	if number(window) > 0 && len(window) < b.height {
		t.Fatalf("window of %d lines from line %d shows %d: empty space below the last line", b.height, number(window), len(window))
	}
	for _, l := range window {
		if (l.Cursor >= 0) != (l.Number == line) {
			t.Fatalf("line %d: cursor run %d, cursor on line %d", l.Number, l.Cursor, line)
		}
		if w := ansi.StringWidth(cells(l)); w > b.width {
			t.Fatalf("line %d is drawn %d columns wide in a window of %d: %+v", l.Number, w, b.width, l)
		}
		if len(l.Runs) > 3 || slices.ContainsFunc(l.Runs, func(r Run) bool { return r.Text == "" }) {
			t.Fatalf("line %d: runs %+v, want at most three, none empty, without a classifier", l.Number, l.Runs)
		}
		checkCellsStandAlone(t, b, l.Number)
	}
}

// checkCellsStandAlone fails the test unless each cell of line i drawn as
// itself takes as many columns between two other characters as b gives it:
// otherwise it would join what is drawn beside it, and the terminal would
// draw the line narrower than b laid it out.
func checkCellsStandAlone(t *testing.T, b *Buffer, i int) {
	t.Helper()
	s := b.lines[i]
	for off, col := 0, 0; off < len(s); {
		size, w, k := b.cell(s[off:], col)
		if text := s[off : off+size]; k == shown && ansi.StringWidth("|"+text+"|") != w+2 {
			t.Fatalf("cell %q of line %d takes %d columns, but %d between two others", text, i, w, ansi.StringWidth("|"+text+"|")-2)
		}
		off, col = off+size, col+w
	}
}

func TestLineBreaksSurviveEdits(t *testing.T) {
	b := New("a\r\nb\r\n", 4)
	b.LineEnd()
	b.Insert("x\ny")
	if got, want := b.String(), "ax\r\ny\r\nb\r\n"; got != want {
		t.Errorf("CRLF text after an insert = %q, want %q", got, want)
	}
	b = New("a\nb", 4)
	b.Bottom()
	b.Insert("c")
	if got, want := b.String(), "a\nbc"; got != want {
		t.Errorf("text without a final line break after an insert = %q, want %q", got, want)
	}
	b.DeleteLine()
	if got, want := b.String(), "a"; got != want {
		t.Errorf("after deleting the last line = %q, want %q", got, want)
	}
	b = New("abc\n", 4)
	b.DeleteLine()
	if got, want := b.String(), "\n"; got != want {
		t.Errorf("after deleting the only line = %q, want %q: the final line break stays", got, want)
	}
}

func TestCells(t *testing.T) {
	for _, c := range []struct {
		name, line string
		width      int
		want       string
	}{
		{"plain", "func main() {}", 80, "func main() {}"},
		{"tab at the start", "\tx", 80, "    x"},
		{"tab mid-stop", "ab\tc", 80, "ab  c"},
		{"tab on a stop", "abcd\te", 80, "abcd    e"},
		{"tab after a wide character", "日\tx", 80, "日  x"},
		{"control characters", "a\x1b[2Jb\rc\x7f", 80, "a?[2Jb?c?"},
		{"a C1 control", "a\U00000085b", 80, "a?b"},
		{"one ? per invalid byte", "bad\xff\xfebytes\xff", 80, "bad??bytes?"},
		{"an encoded U+FFFD is kept", "a\U0000FFFDb", 80, "a\U0000FFFDb"},
		{"a stray combining mark", "\U00000301x", 80, "?x"},
		{"a combining mark joins its letter", "e\U00000301x", 80, "e\U00000301x"},
		{"a BOM", "\U0000FEFFx", 80, "?x"},
		{"a zero-width space", "a\U0000200Bb", 80, "a?b"},
		{"a bidi control", "a\U0000202Eb", 80, "a?b"},
		{"a lone skin tone modifier", "\U0001F3FDx", 80, "?x"},
		{"a lone vowel sign after a tab", "\t\U0000093Fx", 80, "    ?x"},
		{"a vowel sign on its consonant", "\U00000915\U0000093Fx", 80, "\U00000915\U0000093Fx"},
		{"a prepended mark before a tab", "\U00000D4E\tx", 80, "?   x"},
		{"emoji with a skin tone", "👍🏽x", 80, "👍🏽x"},
		{"a flag", "🇪🇸x", 80, "🇪🇸x"},
		{"a ZWJ family", "👨\U0000200D👩\U0000200D👧x", 80, "👨\U0000200D👩\U0000200D👧x"},
		{"CJK", "日本", 80, "日本"},
		{"cut at the width", "abcdef", 4, "abcd"},
		{"tab crossing the edge", "ab\tc", 3, "ab"},
		{"wide character crossing the edge", "ab日", 3, "ab"},
	} {
		if got := rows(sized(c.line, c.width, 1)); len(got) != 1 || got[0] != c.want {
			t.Errorf("%s: %q in %d columns is drawn %q, want %q", c.name, c.line, c.width, got, c.want)
		}
	}
}

// moves applies each move to b in turn, checking the cursor after each.
func moves(t *testing.T, b *Buffer, steps []struct {
	name         string
	move         func()
	line, offset int
}) {
	t.Helper()
	for _, s := range steps {
		s.move()
		if l, o := b.Cursor(); l != s.line || o != s.offset {
			t.Errorf("after %s the cursor is at line %d offset %d, want line %d offset %d", s.name, l, o, s.line, s.offset)
		}
	}
}

type step = struct {
	name         string
	move         func()
	line, offset int
}

func TestLeftAndRight(t *testing.T) {
	b := sized("cé\t\xff", 80, 5) // "é" is 2 bytes
	moves(t, b, []step{
		{"left at the start", b.Left, 0, 0},
		{"right", b.Right, 0, 1},
		{"right over é", b.Right, 0, 3},
		{"right over the tab", b.Right, 0, 4},
		{"right over the invalid byte", b.Right, 0, 5},
		{"right at the end", b.Right, 0, 5},
		{"left over the invalid byte", b.Left, 0, 4},
		{"left over the tab", b.Left, 0, 3},
		{"left over é", b.Left, 0, 1},
		{"left", b.Left, 0, 0},
	})
}

func TestLeftAndRightStayOnTheLine(t *testing.T) {
	b := sized("ab\ncd\nef", 80, 5)
	b.Down()
	moves(t, b, []step{
		{"left at a line start", b.Left, 1, 0},
		{"right", b.Right, 1, 1},
		{"right", b.Right, 1, 2},
		{"right at a line end", b.Right, 1, 2},
	})
}

// A Left or Right that can't move does nothing, so a run of vertical moves
// goes on through it.
func TestAStuckLeftOrRightKeepsTheGoalColumn(t *testing.T) {
	b := sized("abcdefgh\n\nabcdefgh", 80, 5)
	for range 6 {
		b.Right() // column 6
	}
	moves(t, b, []step{
		{"down to an empty line", b.Down, 1, 0},
		{"left at its start", b.Left, 1, 0},
		{"right at its end", b.Right, 1, 0},
		{"down, back to column 6", b.Down, 2, 6},
	})
}

func TestLeftAndRightStepOverWholeClusters(t *testing.T) {
	family := "👨\U0000200D👩\U0000200D👧"
	b := sized("e\U00000301"+family+"x", 80, 1)
	moves(t, b, []step{
		{"right over e and its accent", b.Right, 0, 3},
		{"right over the family", b.Right, 0, 3 + len(family)},
		{"left over the family", b.Left, 0, 3},
		{"left over e and its accent", b.Left, 0, 0},
	})
}

func TestUpAndDownKeepTheGoalColumn(t *testing.T) {
	b := sized("abcdefgh\nab\n\tdefgh\n日本語x", 80, 5)
	b.LineStart()
	for range 6 {
		b.Right() // column 6
	}
	moves(t, b, []step{
		{"up on the first line", b.Up, 0, 6},
		{"down to a short line", b.Down, 1, 2},
		{"down, back to column 6, past the tab", b.Down, 2, 3},
		{"down past three wide characters", b.Down, 3, 9},
		{"down on the last line", b.Down, 3, 9},
		{"up", b.Up, 2, 3},
		{"up", b.Up, 1, 2},
		{"up to column 6", b.Up, 0, 6},
	})
	// A horizontal move ends the run: the goal is the new column.
	b.Down() // line 1, at its end, column 2
	b.Left() // column 1
	moves(t, b, []step{
		{"down onto the tab, which covers column 1", b.Down, 2, 0},
	})
}

func TestUpAndDownLandOnACellCoveringTheGoal(t *testing.T) {
	b := sized("abcdef\n\tx\n日本", 80, 5)
	b.LineStart()
	b.Right() // column 1
	moves(t, b, []step{
		{"down into the tab", b.Down, 1, 0},
		{"down into 日", b.Down, 2, 0},
	})
	b = sized("abcdef\n日本", 80, 5)
	b.LineStart()
	for range 3 {
		b.Right() // column 3
	}
	moves(t, b, []step{{"down into 本", b.Down, 1, 3}})
}

func TestLineStartLineEndTopAndBottom(t *testing.T) {
	b := sized("one\ntwo\nthree", 80, 5)
	moves(t, b, []step{
		{"line end", b.LineEnd, 0, 3},
		{"line start", b.LineStart, 0, 0},
		{"bottom", b.Bottom, 2, 5},
		{"line start", b.LineStart, 2, 0},
		{"top", b.Top, 0, 0},
	})
}

func TestPageUpAndDown(t *testing.T) {
	b := sized(numbered(10), 80, 3) // the last page starts at line 7
	moves(t, b, []step{
		{"page down", b.PageDown, 3, 0},
		{"page down", b.PageDown, 6, 0},
		{"page down", b.PageDown, 9, 0},
		{"page down on the last line", b.PageDown, 9, 0},
		{"page up", b.PageUp, 6, 0},
	})
	checkRows(t, b, 4, "l4", "l5", "l6")
	moves(t, b, []step{
		{"page up", b.PageUp, 3, 0},
		{"page up", b.PageUp, 0, 0},
	})
	checkRows(t, b, 0, "l0", "l1", "l2")
	// The column is kept, as for up and down.
	b = sized("abcdef\nab\nabcdef\nabcdef", 80, 1)
	b.LineEnd()
	b.PageDown()
	b.PageDown()
	checkCursor(t, b, 2, 6)
}

func TestPageDownScrollsAPage(t *testing.T) {
	b := sized(numbered(10), 80, 3)
	b.Down() // line 1, the window still at line 0
	b.PageDown()
	checkCursor(t, b, 4, 0)
	checkRows(t, b, 3, "l3", "l4", "l5") // the cursor keeps its row in the window
}

func TestInsert(t *testing.T) {
	for _, c := range []struct {
		name, text string
		before     func(b *Buffer)
		insert     string
		want       string
		line, off  int
	}{
		{"inside a line", "ac", (*Buffer).Right, "b", "abc", 0, 2},
		{"at the start", "bc", func(*Buffer) {}, "a", "abc", 0, 1},
		{"at the end", "ab", (*Buffer).LineEnd, "c", "abc", 0, 3},
		{"a line break", "ab", (*Buffer).Right, "\n", "a\nb", 1, 0},
		{"a CRLF line break", "ab", (*Buffer).Right, "\r\n", "a\nb", 1, 0},
		{"a lone CR", "ab", (*Buffer).Right, "\r", "a\nb", 1, 0},
		{"several lines", "a\nz", (*Buffer).LineEnd, "b\r\nc\nd", "ab\nc\nd\nz", 2, 1},
		{"a tab", "ab", (*Buffer).Right, "\t", "a\tb", 0, 2},
		{"nothing", "ab", (*Buffer).Right, "", "ab", 0, 1},
	} {
		b := sized(c.text, 80, 5)
		c.before(b)
		b.Insert(c.insert)
		if got := b.String(); got != c.want {
			t.Errorf("%s: text = %q, want %q", c.name, got, c.want)
		}
		if l, o := b.Cursor(); l != c.line || o != c.off {
			t.Errorf("%s: cursor at line %d offset %d, want line %d offset %d", c.name, l, o, c.line, c.off)
		}
	}
}

func TestDeleteLeft(t *testing.T) {
	for _, c := range []struct {
		name, text string
		before     func(b *Buffer)
		want       string
		line, off  int
	}{
		{"inside a line", "abc", func(b *Buffer) { b.Right(); b.Right() }, "ac", 0, 1},
		{"a whole cluster", "ae\U00000301", (*Buffer).LineEnd, "a", 0, 1},
		{"a wide character", "a日", (*Buffer).LineEnd, "a", 0, 1},
		{"one invalid byte", "a\xff\xfe", (*Buffer).LineEnd, "a\xff", 0, 2},
		{"joining lines", "ab\ncd", func(b *Buffer) { b.Down(); b.LineStart() }, "abcd", 0, 2},
		{"at the start of the text", "ab", func(*Buffer) {}, "ab", 0, 0},
	} {
		b := sized(c.text, 80, 5)
		c.before(b)
		b.DeleteLeft()
		if got := b.String(); got != c.want {
			t.Errorf("%s: text = %q, want %q", c.name, got, c.want)
		}
		if l, o := b.Cursor(); l != c.line || o != c.off {
			t.Errorf("%s: cursor at line %d offset %d, want line %d offset %d", c.name, l, o, c.line, c.off)
		}
	}
}

func TestDeleteRight(t *testing.T) {
	for _, c := range []struct {
		name, text string
		before     func(b *Buffer)
		want       string
		line, off  int
	}{
		{"inside a line", "abc", (*Buffer).Right, "ac", 0, 1},
		{"a whole cluster", "e\U00000301a", func(*Buffer) {}, "a", 0, 0},
		{"one invalid byte", "\xff\xfea", func(*Buffer) {}, "\xfea", 0, 0},
		{"a tab", "\ta", func(*Buffer) {}, "a", 0, 0},
		{"joining lines", "ab\ncd", (*Buffer).LineEnd, "abcd", 0, 2},
		{"at the end of the text", "ab", (*Buffer).LineEnd, "ab", 0, 2},
	} {
		b := sized(c.text, 80, 5)
		c.before(b)
		b.DeleteRight()
		if got := b.String(); got != c.want {
			t.Errorf("%s: text = %q, want %q", c.name, got, c.want)
		}
		if l, o := b.Cursor(); l != c.line || o != c.off {
			t.Errorf("%s: cursor at line %d offset %d, want line %d offset %d", c.name, l, o, c.line, c.off)
		}
	}
}

func TestDeleteLine(t *testing.T) {
	for _, c := range []struct {
		name, text string
		before     func(b *Buffer)
		want       string
		line, off  int
	}{
		{"in the middle", "one\ntwo\nthree\n", func(b *Buffer) { b.Down(); b.Right() }, "one\nthree\n", 1, 1},
		{"the last line", "one\ntwo", (*Buffer).Bottom, "one", 0, 3},
		{"the only line", "one\n", (*Buffer).LineEnd, "\n", 0, 0},
		{"the only line, empty", "", func(*Buffer) {}, "", 0, 0},
	} {
		b := sized(c.text, 80, 5)
		c.before(b)
		b.DeleteLine()
		if got := b.String(); got != c.want {
			t.Errorf("%s: text = %q, want %q", c.name, got, c.want)
		}
		if l, o := b.Cursor(); l != c.line || o != c.off {
			t.Errorf("%s: cursor at line %d offset %d, want line %d offset %d", c.name, l, o, c.line, c.off)
		}
	}
}

func TestDeleteLineKeepsTheGoalColumn(t *testing.T) {
	b := sized("abcdef\nab\nabcdef", 80, 5)
	b.LineEnd() // column 6
	b.DeleteLine()
	checkCursor(t, b, 0, 2) // "ab" is too short
	b.DeleteLine()
	checkCursor(t, b, 0, 6) // back at column 6
}

// Review focus: typing a joiner between two emoji makes them one cluster;
// the cursor must not be left inside it.
func TestTheCursorSnapsOutOfAJoinedCluster(t *testing.T) {
	b := sized("👨👩", 80, 1)
	b.Right() // between the two
	b.Insert("\U0000200D")
	checkCursor(t, b, 0, len("👨\U0000200D👩"))
	if got := b.String(); got != "👨\U0000200D👩" {
		t.Errorf("text = %q", got)
	}
}

func TestVersion(t *testing.T) {
	b := sized("ab\ncd", 80, 5)
	if b.Version() != 0 {
		t.Fatalf("a new buffer's version = %d, want 0", b.Version())
	}
	for _, c := range []struct {
		name    string
		do      func()
		changes bool
	}{
		{"a move", b.Right, false},
		{"SetSize", func() { b.SetSize(10, 2) }, false},
		{"insert", func() { b.Insert("x") }, true},
		{"insert nothing", func() { b.Insert("") }, false},
		{"delete left", b.DeleteLeft, true},
		{"delete right", b.DeleteRight, true},
		{"delete line", b.DeleteLine, true},
		{"delete left at the start", func() { b.Top(); b.DeleteLeft() }, false},
		{"delete right at the end", func() { b.Bottom(); b.DeleteRight() }, false},
	} {
		v := b.Version()
		c.do()
		if got := b.Version() != v; got != c.changes {
			t.Errorf("%s: version went from %d to %d", c.name, v, b.Version())
		}
	}
	e := New("", 4)
	e.DeleteLine()
	if e.Version() != 0 {
		t.Error("deleting the only, empty line changes nothing")
	}
}

func TestWindowSizeIsAtLeastOne(t *testing.T) {
	b := New("ab\ncd", 4)
	checkRows(t, b, 0, "a") // 1x1 until SetSize
	for _, w := range []int{0, -3} {
		b.SetSize(w, w)
		checkRows(t, b, 0, "a")
	}
}

func TestTheWindowFollowsTheCursorDownAndUp(t *testing.T) {
	b := sized(numbered(10), 80, 3)
	b.Down()
	b.Down()
	checkRows(t, b, 0, "l0", "l1", "l2")
	b.Down()
	checkRows(t, b, 1, "l1", "l2", "l3")
	b.Bottom() // the end of l9: the cursor's cell is a space
	checkRows(t, b, 7, "l7", "l8", "l9 ")
	b.Up()
	b.Up()
	checkRows(t, b, 7, "l7 ", "l8", "l9")
	b.Up()
	checkRows(t, b, 6, "l6 ", "l7", "l8")
}

func TestNoEmptySpaceBelowTheLastLine(t *testing.T) {
	b := sized(numbered(10), 80, 3)
	b.Bottom()
	b.DeleteLine()
	b.DeleteLine()
	checkRows(t, b, 5, "l5", "l6", "l7 ")
	b = sized(numbered(10), 80, 3)
	b.Bottom()
	b.SetSize(80, 5) // growing near the end pulls the window up
	checkRows(t, b, 5, "l5", "l6", "l7", "l8", "l9 ")
}

func TestTheWindowScrollsSidewaysWhileTyping(t *testing.T) {
	b := sized("", 4, 1)
	for i, want := range []string{"a", "ab", "abc", "bcd", "cde"} {
		b.Insert(string(rune('a' + i)))
		if got := rows(b); len(got) != 1 || got[0] != want+" " {
			t.Errorf("after typing %d letters the window shows %q, want %q and the cursor", i+1, got, want)
		}
	}
	b.LineStart() // fits in the first 4 columns: back to the start
	checkRows(t, b, 0, "abcd")
	b.LineEnd()
	checkRows(t, b, 0, "cde ")
}

func TestScrollingLeftStepsOneColumnAtATime(t *testing.T) {
	b := sized("abcdefghij", 4, 1)
	b.LineEnd() // columns 7 to 10
	for range 4 {
		b.Left() // the cursor reaches column 6, the window's first
	}
	checkRows(t, b, 0, "ghij")
	b.Left() // column 5: one column to the left
	checkRows(t, b, 0, "fghi")
}

func TestScrollingSidewaysAppliesToEveryLine(t *testing.T) {
	b := sized("abcdefgh\n0123456789", 4, 2)
	b.Down()
	b.LineEnd() // column 10: the window shows columns 7 to 10
	checkRows(t, b, 0, "h", "789 ")
}

func TestWideCharactersAndTabsAtTheEdges(t *testing.T) {
	b := sized("日本語", 3, 1)
	checkRows(t, b, 0, "日") // 本 crosses the right edge: not drawn
	b.Right()
	b.Right()                // the cursor on 語, columns 4-5: the window scrolls to columns 3-5
	checkRows(t, b, 0, " 語") // 本's right half is a space
	b = sized("\tab", 3, 1)
	b.LineEnd() // column 6: the window shows columns 4 to 6
	if before, cursor, _ := split(b.Window(nil)[0]); before != "ab" || cursor != " " {
		t.Errorf("window = %q|%q, want the tab out of view", before, cursor)
	}
	b.Left()
	b.Left()
	b.Left() // on the tab, columns 0-3: back to the start
	checkRows(t, b, 0, "   ")
}

func TestTheCursorsCell(t *testing.T) {
	for _, c := range []struct {
		name, text            string
		rights                int
		before, cursor, after string
	}{
		{"inside a line", "abc", 1, "a", "b", "c"},
		{"at the start", "abc", 0, "", "a", "bc"},
		{"at the line's end", "abc", 3, "abc", " ", ""},
		{"an empty line", "", 0, "", " ", ""},
		{"on a tab", "a\tb", 1, "a", " ", "  b"},
		{"on a wide character", "a日b", 1, "a", "日", "b"},
		{"on an invalid byte", "a\xffb", 1, "a", "?", "b"},
	} {
		b := sized(c.text, 80, 1)
		for range c.rights {
			b.Right()
		}
		before, cursor, after := split(b.Window(nil)[0])
		if before != c.before || cursor != c.cursor || after != c.after {
			t.Errorf("%s: %q|%q|%q, want %q|%q|%q", c.name, before, cursor, after, c.before, c.cursor, c.after)
		}
	}
}

func TestOnlyTheCursorsLineHasACursorCell(t *testing.T) {
	b := sized("ab\ncd\nef", 80, 3)
	b.Down()
	for _, l := range b.Window(nil) {
		if (l.Cursor >= 0) != (l.Number == 1) {
			t.Errorf("line %d: cursor run %d", l.Number, l.Cursor)
		}
	}
}

// Review focus: narrowing the window while the cursor is far right, as a
// terminal resize does, keeps the cursor in view.
func TestSetSizeFollowsTheCursor(t *testing.T) {
	b := sized(strings.Repeat("x", 50)+"y", 80, 1)
	b.LineEnd()
	b.SetSize(10, 1)
	if before, cursor, _ := split(b.Window(nil)[0]); before != "xxxxxxxxy" || cursor != " " {
		t.Errorf("after narrowing the window = %q|%q, want the line's end in view", before, cursor)
	}
}

// Review focus: a window narrower than the character under the cursor
// still shows the cursor, as a space standing for the character, and is no
// wider than the window.
func TestAWindowNarrowerThanTheCursorsCell(t *testing.T) {
	b := sized("a日", 1, 1)
	b.Right()
	if got, want := b.Window(nil)[0], (Line{Runs: []Run{{Text: " "}}}); !reflect.DeepEqual(got, want) {
		t.Errorf("window = %+v, want %+v", got, want)
	}
}

// Review focus: the caller sets the size after every key, which must not
// make the cursor forget the column a run of up and down moves aims at.
func TestSetSizeKeepsTheGoalColumn(t *testing.T) {
	b := sized("abcdef\nab\nabcdef", 20, 5)
	b.LineEnd()
	b.Down()
	b.SetSize(30, 5)
	b.Down()
	checkCursor(t, b, 2, 6)
}

// Review focus: a combining mark can join the U+FFFD an invalid byte
// decodes to, but the byte must stay a cell of its own.
func TestAnInvalidByteNeverJoinsACluster(t *testing.T) {
	b := sized("\xff\U00000301x", 80, 1)
	if got := rows(b); got[0] != "??x" {
		t.Errorf("drawn %q, want the byte and the stray mark as one ? each", got)
	}
	moves(t, b, []step{
		{"right over the invalid byte", b.Right, 0, 1},
		{"right over the stray mark", b.Right, 0, 3},
	})
	b.DeleteLeft()
	if got := b.String(); got != "\xffx" {
		t.Errorf("after deleting the mark: %q", got)
	}
}

func TestWindowDoesNotChangeTheBuffer(t *testing.T) {
	b := sized("abc\ndef", 2, 1)
	b.Down()
	b.LineEnd()
	first := b.Window(nil)
	for range 3 {
		if got := b.Window(nil); !reflect.DeepEqual(got, first) {
			t.Fatalf("Window = %+v, then %+v", first, got)
		}
	}
	checkCursor(t, b, 1, 3)
}

// Review focus: a minified file holds one huge line. Drawing it must stop
// at the window's edge rather than walk the whole line, or every frame
// would crawl.
func TestWindowOnAHugeLineIsFast(t *testing.T) {
	b := sized(strings.Repeat("x", 10<<20), 80, 1)
	start := time.Now()
	for range 10000 {
		if got := cells(b.Window(nil)[0]); len(got) != 80 {
			t.Fatalf("drew %d cells, want 80", len(got))
		}
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("10000 draws of a 10 MB line took %v: drawing must stop at the window's edge", d)
	}
}

func TestATabWidthBelowOneCountsAsOne(t *testing.T) {
	if got := rows(sized("\tx", 80, 1)); got[0] != "    x" {
		t.Fatalf("with a tab width of 4: %q", got)
	}
	b := New("\tx", 0)
	b.SetSize(80, 1)
	if got := rows(b); got[0] != " x" {
		t.Errorf("with a tab width of 0: %q, want a 1-column tab", got)
	}
}

// byOffset is a classifier that puts the cell at offset off of any line in
// class classes[off], and 0 when classes has no class for it.
func byOffset(classes map[int]int) func(i, off int) int {
	return func(_, off int) int { return classes[off] }
}

func TestWindowRunsByClass(t *testing.T) {
	classes := byOffset(map[int]int{0: 1, 1: 1, 2: 2, 3: 2, 5: 1, 6: 1}) // "ab" 1, "12" 2, " " 0, "cd" 1
	for _, c := range []struct {
		name   string
		rights int
		want   Line
	}{
		{"the cursor at the start", 0, Line{Cursor: 0, Runs: []Run{{"a", 1}, {"b", 1}, {"12", 2}, {" ", 0}, {"cd", 1}}}},
		{"the cursor inside a run of its class", 1, Line{Cursor: 1, Runs: []Run{{"a", 1}, {"b", 1}, {"12", 2}, {" ", 0}, {"cd", 1}}}},
		{"the cursor past the end", 7, Line{Cursor: 4, Runs: []Run{{"ab", 1}, {"12", 2}, {" ", 0}, {"cd", 1}, {" ", 0}}}},
	} {
		b := sized("ab12 cd", 80, 1)
		for range c.rights {
			b.Right()
		}
		if got := b.Window(classes)[0]; !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// A cell drawn as spaces or as "?" takes the class of the cell it stands
// for.
func TestCellsDrawnAsOthersKeepTheirClass(t *testing.T) {
	b := sized("\tx\xffy", 80, 1)
	b.LineEnd()
	want := Line{Cursor: 4, Runs: []Run{{"    ", 3}, {"x", 1}, {"?", 2}, {"y", 1}, {" ", 0}}}
	if got := b.Window(byOffset(map[int]int{0: 3, 1: 1, 2: 2, 3: 1}))[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("tab and invalid byte: %+v, want %+v", got, want)
	}
	b.LineStart() // on the tab: its first column is the cursor, the rest a run of its class
	want = Line{Cursor: 0, Runs: []Run{{" ", 3}, {"   ", 3}, {"x", 1}, {"?", 2}, {"y", 1}}}
	if got := b.Window(byOffset(map[int]int{0: 3, 1: 1, 2: 2, 3: 1}))[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("on the tab: %+v, want %+v", got, want)
	}
	b = sized("日本語", 3, 1)
	b.Right()
	b.Right() // the window shows columns 3 to 5: 本's right half, then 語
	want = Line{Cursor: 1, Runs: []Run{{" ", 2}, {"語", 3}}}
	if got := b.Window(byOffset(map[int]int{0: 1, 3: 2, 6: 3}))[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("a cell crossing the left edge: %+v, want %+v", got, want)
	}
}

// Window asks for the class of each cell it draws, once, in drawing order,
// and never for one it doesn't draw.
func TestWindowClassifiesTheCellsItDraws(t *testing.T) {
	var calls [][2]int
	classify := func(i, off int) int {
		calls = append(calls, [2]int{i, off})
		return 0
	}
	b := sized("abc\ndef", 80, 2)
	b.Window(classify)
	if want := [][2]int{{0, 0}, {0, 1}, {0, 2}, {1, 0}, {1, 1}, {1, 2}}; !slices.Equal(calls, want) {
		t.Errorf("calls %v, want %v", calls, want)
	}
	calls = nil
	b = sized("abcdefgh", 4, 1)
	b.LineEnd() // the window shows columns 5 to 8: f, g, h and the cursor past the end
	b.Window(classify)
	if want := [][2]int{{0, 5}, {0, 6}, {0, 7}}; !slices.Equal(calls, want) {
		t.Errorf("calls %v, want %v: none for cells left of the window or past the end", calls, want)
	}
}
