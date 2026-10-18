// Package textview is the navigation state of a read-only text shown a few
// lines at a time: its lines and a scroll window over them. It knows nothing
// about how the lines are drawn: the caller draws the lines Window returns.
//
// It is app-agnostic and depends only on the standard library; a depguard
// rule in .golangci.yaml keeps it from importing the app.
package textview

import (
	"slices"
	"strings"
)

// View is the navigation state over one text: its lines and a scroll window
// over them. It is not safe for concurrent use.
type View struct {
	lines  []string // the text's lines, at least one
	offset int      // index into lines of the first line in the window
	height int      // how many lines the window shows, at least 1
}

// New returns a View over text, scrolled to the top, whose window is one line
// tall until SetHeight is called. The text is split at every "\n", and a "\r"
// just before one is dropped, so CRLF text reads as LF text. A final line
// ending ends the last line rather than starting an empty one, and empty text
// is a single empty line. Nothing else in the lines changes.
func New(text string) *View {
	lines := strings.Split(text, "\n")
	last := len(lines) - 1
	for i := range last {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	if last > 0 && lines[last] == "" {
		lines = lines[:last]
	}
	return &View{lines: lines, height: 1}
}

// SetHeight makes the window h lines tall (a height below 1 counts as 1). The
// window keeps its first line, then is clamped, so growing it near the end of
// the text pulls it up.
func (v *View) SetHeight(h int) {
	v.height = max(h, 1)
	v.scrollTo(v.offset)
}

// Up scrolls the window one line up, stopping at the first line.
func (v *View) Up() { v.scrollTo(v.offset - 1) }

// Down scrolls the window one line down, stopping once the last line is in
// view.
func (v *View) Down() { v.scrollTo(v.offset + 1) }

// PageUp scrolls the window one window height up, stopping at the first line.
func (v *View) PageUp() { v.scrollTo(v.offset - v.height) }

// PageDown scrolls the window one window height down, stopping once the last
// line is in view.
func (v *View) PageDown() { v.scrollTo(v.offset + v.height) }

// Top scrolls the window to the first line.
func (v *View) Top() { v.scrollTo(0) }

// Bottom scrolls the window to the last page, with the last line at its
// bottom.
func (v *View) Bottom() { v.scrollTo(len(v.lines)) }

// Len returns how many lines the text has, at least 1.
func (v *View) Len() int { return len(v.lines) }

// Window returns the lines inside the scroll window, top to bottom, in a new
// slice, and the index of the first of them, so the caller can number them.
// It does not change the view, so it is safe to call from a Bubble Tea View.
func (v *View) Window() (lines []string, first int) {
	end := min(v.offset+v.height, len(v.lines))
	return slices.Clone(v.lines[v.offset:end]), v.offset
}

// scrollTo starts the window at line i, clamped so that it never starts
// before the first line, nor shows empty space below the last line while
// earlier lines are out of view.
func (v *View) scrollTo(i int) {
	v.offset = max(min(i, len(v.lines)-v.height), 0)
}
