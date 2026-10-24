package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/candy-tools/trazo/libs/keymap"
	"github.com/candy-tools/trazo/libs/textedit"
	"github.com/charmbracelet/x/ansi"
)

// numberedLines returns a text of n lines, "line 1" to "line <n>", each ended
// by "\n".
func numberedLines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

// textDoc returns a document at path holding text, fitted to an editor body
// w cells wide, the gutter included, and h lines tall.
func textDoc(path, text string, w, h int) *document {
	doc := &document{path: path, text: textedit.New(text, tabWidth)}
	doc.setSize(w, h)
	return doc
}

func TestEditorBodyNumbersTheLines(t *testing.T) {
	for _, c := range []struct {
		lines int
		want  string
	}{
		{9, " 1 line 1\n 2 line 2"},
		{10, "  1 line 1\n  2 line 2"},
		{100, "   1 line 1\n   2 line 2"},
	} {
		doc := textDoc("f", numberedLines(c.lines), 40, 2)
		if got := ansi.Strip(editorBody(doc, false, noColors)); got != c.want {
			t.Errorf("%d lines: body =\n%s\nwant\n%s", c.lines, got, c.want)
		}
	}
}

func TestEditorBodyNumbersFromTheWindowsFirstLine(t *testing.T) {
	doc := textDoc("f", numberedLines(10), 40, 2)
	doc.text.Bottom()
	doc.text.LineStart()
	if got, want := ansi.Strip(editorBody(doc, false, noColors)), "  9 line 9\n 10 line 10"; got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestEditorBodyCutsLinesAfterTheGutter(t *testing.T) {
	doc := textDoc("f", "\tabcdefgh", 8, 1)
	if got, want := ansi.Strip(editorBody(doc, false, noColors)), " 1     a"; got != want {
		t.Errorf("body = %q, want %q: the gutter takes 3 of the 8 cells", got, want)
	}
}

func TestEditorBodyWithoutText(t *testing.T) {
	if got, want := ansi.Strip(editorBody(nil, true, noColors)), " Select a file and press enter to open it"; got != want {
		t.Errorf("with no tab open, body = %q, want %q", got, want)
	}
	doc := &document{path: "logo.png", note: "binary file"}
	if got, want := ansi.Strip(editorBody(doc, true, noColors)), " binary file"; got != want {
		t.Errorf("with a note, body = %q, want %q", got, want)
	}
}

// The cursor is drawn in reverse video with a plain SGR sequence, which
// shows whatever the terminal's colours, but only while the editor has
// focus.
func TestTheCursorIsDrawnWhileTheEditorHasFocus(t *testing.T) {
	doc := textDoc("f", "abc\ndef", 40, 2)
	doc.text.Right()
	if got, want := editorBody(doc, true, noColors), cursorStyle.Styled("b"); !strings.Contains(got, want) {
		t.Errorf("focused body %q should draw the cursor on b: %q", got, want)
	}
	if got := editorBody(doc, false, noColors); strings.Contains(got, "\x1b[7m") {
		t.Errorf("unfocused body %q should not draw the cursor", got)
	}
	doc.text.LineEnd()
	if got, want := editorBody(doc, true, noColors), cursorStyle.Styled(" "); !strings.Contains(got, want) {
		t.Errorf("at the end of a line the cursor is a reversed space: body %q", got)
	}
}

// step is an editor action and what the text and its cursor are after it.
type step struct {
	id           keymap.ID
	text         string
	line, offset int
}

func TestEditorActions(t *testing.T) {
	doc := textDoc("f", "ab\ncd", 40, 5)
	for _, s := range []step{
		{editorRight, "ab\ncd", 0, 1},
		{editorDown, "ab\ncd", 1, 1},
		{editorLineEnd, "ab\ncd", 1, 2},
		{editorNewline, "ab\ncd\n", 2, 0},
		{editorInsertTab, "ab\ncd\n\t", 2, 1},
		{editorDeleteLeft, "ab\ncd\n", 2, 0},
		{editorDeleteLeft, "ab\ncd", 1, 2},
		{editorLineStart, "ab\ncd", 1, 0},
		{editorDeleteRight, "ab\nd", 1, 0},
		{editorUp, "ab\nd", 0, 0},
		{editorLeft, "ab\nd", 0, 0},
		{editorBottom, "ab\nd", 1, 1},
		{editorTop, "ab\nd", 0, 0},
		{editorPageDown, "ab\nd", 1, 0},
		{editorPageUp, "ab\nd", 0, 0},
		{editorDeleteLine, "d", 0, 0},
		{sidebarDown, "d", 0, 0}, // not an editor action: nothing
		{actQuit, "d", 0, 0},
	} {
		editorAction(doc, s.id)
		line, offset := doc.text.Cursor()
		if got := doc.text.String(); got != s.text || line != s.line || offset != s.offset {
			t.Errorf("after %s: text %q, cursor %d:%d; want %q, %d:%d", s.id, got, line, offset, s.text, s.line, s.offset)
		}
	}
	editorAction(nil, editorDown)                                  // no tab open: nothing
	editorAction(&document{note: "binary file"}, editorDeleteLine) // a note: nothing
}

func TestTypeText(t *testing.T) {
	doc := textDoc("f", "ac", 40, 1)
	doc.text.Right()
	typeText(doc, "b")
	if got := doc.text.String(); got != "abc" {
		t.Errorf("text = %q, want abc", got)
	}
	typeText(nil, "x")                            // no tab open: nothing
	typeText(&document{note: "binary file"}, "x") // a note: nothing
}

func TestSetSizeLeavesRoomForTheGutter(t *testing.T) {
	doc := textDoc("f", strings.Repeat("x", 20), 10, 1) // a 3-cell gutter leaves 7
	if got := ansi.Strip(editorBody(doc, false, noColors)); got != " 1 xxxxxxx" {
		t.Errorf("body = %q, want 7 cells of text after the gutter", got)
	}
	for _, w := range []int{4, 2} { // one cell of text left, then none: still one
		doc.setSize(w, 1)
		if got := ansi.Strip(editorBody(doc, false, noColors)); got != " 1 x" {
			t.Errorf("width %d: body = %q, want 1 cell of text after the gutter", w, got)
		}
	}
}
