package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/candy-tools/edit/libs/textview"
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

// textDoc returns a document at path holding text, its window h lines tall.
func textDoc(path, text string, h int) *document {
	doc := &document{path: path, text: textview.New(text)}
	doc.setHeight(h)
	return doc
}

func TestTextLine(t *testing.T) {
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
		{"invalid UTF-8 runs", "bad\xff\xfebytes\xff", 80, "bad?bytes?"},
		{"an encoded U+FFFD is kept", "a�b", 80, "a�b"},
		{"cut at the width", "abcdef", 4, "abcd"},
		{"tab crossing the edge", "ab\tc", 3, "ab"},
		{"wide character crossing the edge", "ab日", 3, "ab"},
		{"no room", "abc", 0, ""},
		{"negative room", "abc", -2, ""},
	} {
		if got := textLine(c.line, c.width); got != c.want {
			t.Errorf("%s: textLine(%q, %d) = %q, want %q", c.name, c.line, c.width, got, c.want)
		}
	}
}

// Review focus: a minified file holds one huge line. textLine must stop at
// the width rather than walk the whole line, or every frame would crawl.
func TestTextLineOnAHugeLineIsFast(t *testing.T) {
	line := strings.Repeat("x", maxDocumentSize)
	start := time.Now()
	for range 10000 {
		if got := textLine(line, 80); len(got) != 80 {
			t.Fatalf("textLine drew %d cells, want 80", len(got))
		}
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("10000 draws of a 10 MB line took %v: textLine must stop at the width", d)
	}
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
		doc := textDoc("f", numberedLines(c.lines), 2)
		if got := ansi.Strip(editorBody(doc, 40)); got != c.want {
			t.Errorf("%d lines: body =\n%s\nwant\n%s", c.lines, got, c.want)
		}
	}
}

func TestEditorBodyNumbersFromTheWindowsFirstLine(t *testing.T) {
	doc := textDoc("f", numberedLines(10), 2)
	doc.text.Bottom()
	if got, want := ansi.Strip(editorBody(doc, 40)), "  9 line 9\n 10 line 10"; got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestEditorBodyCutsLinesAfterTheGutter(t *testing.T) {
	doc := textDoc("f", "\tabcdefgh", 1)
	if got, want := ansi.Strip(editorBody(doc, 8)), " 1     a"; got != want {
		t.Errorf("body = %q, want %q: the gutter takes 3 of the 8 cells", got, want)
	}
}

func TestEditorBodyWithoutText(t *testing.T) {
	if got, want := ansi.Strip(editorBody(nil, 60)), " Select a file and press enter to open it"; got != want {
		t.Errorf("with no tab open, body = %q, want %q", got, want)
	}
	doc := &document{path: "logo.png", note: "binary file"}
	if got, want := ansi.Strip(editorBody(doc, 60)), " binary file"; got != want {
		t.Errorf("with a note, body = %q, want %q", got, want)
	}
}

func TestEditorKeysScrollTheText(t *testing.T) {
	doc := textDoc("f", numberedLines(10), 3) // the last page starts at line 7
	for _, step := range []struct {
		key   string
		first int
	}{
		{"down", 1},
		{"pgdown", 4},
		{"end", 7},
		{"up", 6},
		{"pgup", 3},
		{"home", 0},
		{"j", 0},
		{"enter", 0},
	} {
		editorKey(doc, step.key)
		if _, first := doc.text.Window(); first != step.first {
			t.Errorf("after %s the window starts at line %d, want %d", step.key, first, step.first)
		}
	}
	editorKey(nil, "down")                            // no tab open: nothing
	editorKey(&document{note: "binary file"}, "down") // a note: nothing
}
