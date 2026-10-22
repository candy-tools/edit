// Package syntax colors a text by its syntax. Engines plug in behind
// Language and Tree; Highlighter keeps the tree of one text being edited up
// to date, parsing it in the background. It is app-agnostic and depends only
// on the standard library.
package syntax

// Point is a place in a text: a line, from 0, and a byte column in it.
type Point struct{ Line, Column int }

// Edit is one change to a text: the bytes from StartByte to OldEndByte, which
// ran from Start to OldEnd, became bytes that run to NewEndByte, at NewEnd.
type Edit struct {
	StartByte, OldEndByte, NewEndByte int
	Start, OldEnd, NewEnd             Point
}

// Span colors the bytes Start to End of a line as Capture, one of the names
// Captures lists, such as "keyword" or "string.escape". End can lie past the
// line's end, for a capture that carries on to the next line.
type Span struct {
	Start, End int
	Capture    string
}

// A Language parses texts of one kind. Parse may be called from any
// goroutine, and concurrently. It returns a tree, or an error and no tree.
type Language interface {
	Name() string
	Parse(text []byte) (Tree, error)
}

// A Tree is the syntax of one text. Its methods must be called from one
// goroutine at a time.
type Tree interface {
	// Edit makes the tree follow an edit made to the text after it was
	// parsed, so that its spans stay on the characters they color.
	Edit(Edit)
	// Spans returns the spans of lines from to to-1 of text, the text as it
	// is now, after the edits the tree has followed: one slice per line, each
	// sorted and without overlaps. They are right on bytes lo to hi-1 of each
	// line, and may leave out spans elsewhere on it, so that a caller that
	// draws part of a long line pays for that part only.
	Spans(text []byte, from, to, lo, hi int) [][]Span
	// Close frees the tree. It must not be used afterwards.
	Close()
}
