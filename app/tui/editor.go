package tui

import (
	"fmt"
	"strings"

	"github.com/candy-tools/edit/libs/keymap"
	"github.com/candy-tools/edit/libs/textedit"
	"github.com/charmbracelet/x/ansi"
)

const (
	// tabWidth is how many columns apart the tab stops of a file's text are.
	tabWidth = 4
	// emptyHint is what the editor pane shows while no file is open.
	emptyHint = "Select a file and press enter to open it"
)

// cursorStyle draws the cursor's cell in reverse video. It is a plain SGR
// sequence rather than a lipgloss style: lipgloss drops its styles on a
// terminal without colours or with NO_COLOR set, and the cursor must show
// there too.
var cursorStyle = ansi.NewStyle(ansi.AttrReverse)

// editorBody draws the editor pane's body: the text window of doc, the
// active tab's document, with line numbers, its text colored by its syntax
// in theme and, while the editor has focus, the cursor, in reverse video
// without the colors; or doc's note; or, when no tab is open (doc is nil),
// a hint on how to open a file. The cursor's line number is drawn brighter
// than the others, so the line stays easy to find while the sidebar has
// focus. The window already has the body's size: the model fits it on
// every change.
func editorBody(doc *document, focused bool, theme Theme) string {
	switch {
	case doc == nil:
		return " " + helpTextStyle.Render(emptyHint)
	case doc.text == nil:
		return " " + helpTextStyle.Render(doc.note)
	}
	digits := gutterWidth(doc.text.Len()) - 2
	cursorLine, _ := doc.text.Cursor()
	lines := doc.text.Window(doc.classifier(theme))
	out := make([]string, len(lines))
	for i, l := range lines {
		number := lineNumberStyle
		if l.Number == cursorLine {
			number = cursorLineStyle
		}
		var b strings.Builder
		b.WriteString(number.Render(fmt.Sprintf(" %*d ", digits, l.Number+1)))
		for j, r := range l.Runs {
			if j == l.Cursor && focused {
				b.WriteString(cursorStyle.Styled(r.Text))
			} else {
				b.WriteString(theme.draw(r.Class, r.Text))
			}
		}
		out[i] = b.String()
	}
	return strings.Join(out, "\n")
}

// editorEdits are what the editor's actions do to a text: move its cursor,
// or edit it.
var editorEdits = map[keymap.ID]func(*textedit.Buffer){
	editorLeft:        (*textedit.Buffer).Left,
	editorRight:       (*textedit.Buffer).Right,
	editorUp:          (*textedit.Buffer).Up,
	editorDown:        (*textedit.Buffer).Down,
	editorLineStart:   (*textedit.Buffer).LineStart,
	editorLineEnd:     (*textedit.Buffer).LineEnd,
	editorPageUp:      (*textedit.Buffer).PageUp,
	editorPageDown:    (*textedit.Buffer).PageDown,
	editorTop:         (*textedit.Buffer).Top,
	editorBottom:      (*textedit.Buffer).Bottom,
	editorNewline:     func(b *textedit.Buffer) { b.Insert("\n") },
	editorInsertTab:   func(b *textedit.Buffer) { b.Insert("\t") },
	editorDeleteLeft:  (*textedit.Buffer).DeleteLeft,
	editorDeleteRight: (*textedit.Buffer).DeleteRight,
	editorDeleteLine:  (*textedit.Buffer).DeleteLine,
}

// editorAction applies an action of the editor to doc, the active tab's
// document, as editorEdits says. Other actions do nothing, and so does every
// action with no tab open (doc is nil) or on a document showing a note.
func editorAction(doc *document, id keymap.ID) {
	if edit, ok := editorEdits[id]; ok && doc != nil && doc.text != nil {
		edit(doc.text)
	}
}

// typeText types text at the cursor of doc, the active tab's document: what
// a key types, or pasted text. With no tab open (doc is nil), or on a
// document showing a note, it does nothing.
func typeText(doc *document, text string) {
	if doc != nil && doc.text != nil {
		doc.text.Insert(text)
	}
}
