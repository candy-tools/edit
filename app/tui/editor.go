package tui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/candy-tools/edit/libs/keymap"
	"github.com/charmbracelet/x/ansi"
)

const (
	// tabWidth is how many columns apart the tab stops of a file's text are.
	tabWidth = 4
	// emptyHint is what the editor pane shows while no file is open.
	emptyHint = "Select a file and press enter to open it"
)

// editorBody draws the editor pane's body, width cells wide: the text window
// of doc, the active tab's document, with line numbers; or doc's note; or,
// when no tab is open (doc is nil), a hint on how to open a file.
func editorBody(doc *document, width int) string {
	switch {
	case doc == nil:
		return " " + helpTextStyle.Render(emptyHint)
	case doc.text == nil:
		return " " + helpTextStyle.Render(doc.note)
	}
	lines, first := doc.text.Window()
	digits := len(strconv.Itoa(doc.text.Len()))
	room := width - digits - 2 // the gutter: a space, the number, a space
	out := make([]string, len(lines))
	for i, l := range lines {
		gutter := fmt.Sprintf(" %*d ", digits, first+i+1)
		out[i] = lineNumberStyle.Render(gutter) + textLine(l, room)
	}
	return strings.Join(out, "\n")
}

// editorAction applies an action of the editor to doc, the active tab's
// document: up, down, page-up, page-down, top and bottom scroll its text.
// Other actions do nothing, and so does every action with no tab open (doc
// is nil) or on a document showing a note.
func editorAction(doc *document, id keymap.ID) {
	if doc == nil || doc.text == nil {
		return
	}
	switch id {
	case editorUp:
		doc.text.Up()
	case editorDown:
		doc.text.Down()
	case editorPageUp:
		doc.text.PageUp()
	case editorPageDown:
		doc.text.PageDown()
	case editorTop:
		doc.text.Top()
	case editorBottom:
		doc.text.Bottom()
	}
}

// textLine draws a line of a file's text into at most width cells. A tab
// becomes spaces up to the next multiple of tabWidth; each run of invalid
// UTF-8 bytes and each other control character becomes "?", as in
// displayName; any other character is drawn as it is. It stops before the
// first character that would not fit, so however long the line, only what
// fits is processed.
func textLine(line string, width int) string {
	var b strings.Builder
	col := 0 // cells drawn so far
	for i := 0; i < len(line); {
		text, w, size := textCell(line[i:], col)
		if col+w > width {
			break
		}
		b.WriteString(text)
		col += w
		i += size
	}
	return b.String()
}

// textCell is how textLine draws the character at the start of s, col cells
// into the line: the text to draw, how many cells it takes, and how many
// bytes of s it stands for.
func textCell(s string, col int) (text string, width, size int) {
	r, size := utf8.DecodeRuneInString(s)
	switch {
	case r == utf8.RuneError && size == 1:
		for size < len(s) { // the rest of the run of invalid bytes
			if r, n := utf8.DecodeRuneInString(s[size:]); r != utf8.RuneError || n != 1 {
				break
			}
			size++
		}
		return "?", 1, size
	case r == '\t':
		n := tabWidth - col%tabWidth
		return strings.Repeat(" ", n), n, size
	case unicode.IsControl(r):
		return "?", 1, size
	}
	return s[:size], ansi.StringWidth(s[:size]), size
}
