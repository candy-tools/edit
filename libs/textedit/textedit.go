// Package textedit is the editing state of one text shown a few lines at a
// time: its lines, a cursor, the moves and edits a basic text editor makes,
// and undoing and redoing them, and a window over the lines that follows
// the cursor up, down and sideways. It reads the text the way a terminal draws it, as grapheme
// clusters, and hands the caller the window's cells ready to style; it knows
// nothing about keys, styles or files. It reports what each edit changed, so
// a caller can keep something in step with the text, such as a syntax tree.
//
// It is app-agnostic and depends only on the standard library and
// charmbracelet/x/ansi, which measures characters the way lipgloss does; a
// depguard rule in .golangci.yaml keeps it from importing the app.
package textedit

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Buffer is one text being edited: its lines, a cursor, and a window over
// the lines that follows the cursor. The cursor is a line and a byte offset
// in it, always at a cell boundary: before a cell, or at the line's end. It
// is not safe for concurrent use.
type Buffer struct {
	lines    []string // the text's lines, without their line breaks: at least one
	eol      string   // the line break String writes: "\r\n" or "\n"
	final    bool     // whether the text ends with a line break
	tabWidth int      // how many columns apart tab stops are, at least 1

	line   int // the cursor's line, an index into lines
	offset int // the cursor's byte offset in its line
	goal   int // the column a run of vertical moves aims at; -1 outside one

	top, left     int // the window's first line and first column
	width, height int // the window's size in columns and lines, each at least 1

	version  int          // how many calls changed the text
	onChange func(Change) // told what each edit changed; nil for nobody
	history  history      // the edits to undo and redo
}

// A Position is a place in the text, counted in bytes of the text as String
// writes it: Offset from its start, and Line, from 0, and Column, the bytes
// before it on its line.
type Position struct{ Offset, Line, Column int }

// A Change is what one edit did to the text: it replaced the bytes from Start
// to OldEnd with bytes that now run from Start to NewEnd.
type Change struct{ Start, OldEnd, NewEnd Position }

// A Run is a stretch of a window line's cells that the classifier put in one
// class.
type Run struct {
	Text  string // the cells, as drawn
	Class int
}

// Line is one line of the window, cut to the window's columns and made
// printable, in runs ready for the caller to style.
type Line struct {
	Number int   // the line's index in the text, from 0
	Runs   []Run // the line's cells, left to right, in runs of one class
	Cursor int   // the index in Runs of the cursor's cell, a run of its own; -1 on any other line
}

// New returns a Buffer holding text, with the cursor at its start and a
// window one column wide and one line tall until SetSize is called. The
// text is split at every "\n". When it has at least one and every one of
// them follows a "\r", its line break is "\r\n" and those "\r"s are not part
// of the lines; otherwise it is "\n" and every "\r" stays in its line. A
// final line break ends the last line rather than starting an empty one,
// and empty text is a single empty line. Tab stops are tabWidth columns
// apart; a tabWidth below 1 counts as 1.
func New(text string, tabWidth int) *Buffer {
	b := &Buffer{eol: "\n", tabWidth: max(tabWidth, 1), goal: -1, width: 1, height: 1,
		history: history{steps: defaultSteps, size: defaultSize}}
	if n := strings.Count(text, "\n"); n > 0 && strings.Count(text, "\r\n") == n {
		b.eol = "\r\n"
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	b.final = strings.HasSuffix(text, "\n")
	b.lines = strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	return b
}

// String returns the whole text: the lines joined by the line break, and a
// final line break when the text had one. Until an edit, it is the text New
// was given, byte for byte.
func (b *Buffer) String() string {
	s := strings.Join(b.lines, b.eol)
	if b.final {
		s += b.eol
	}
	return s
}

// Len returns how many lines the text has, at least 1.
func (b *Buffer) Len() int { return len(b.lines) }

// Version returns how many calls changed the text: 0 for a new Buffer, and
// one more after each edit that changed it. Moves, SetSize and edits that
// change nothing leave it alone, so a caller can tell whether the text
// changed since it last looked.
func (b *Buffer) Version() int { return b.version }

// Cursor returns the cursor's line and its byte offset in that line, both
// from 0.
func (b *Buffer) Cursor() (line, offset int) { return b.line, b.offset }

// OnChange makes b call f after every edit that changes the text, with what
// it changed. A nil f stops the calls.
func (b *Buffer) OnChange(f func(Change)) { b.onChange = f }

// Visible returns the lines the window shows: from, to-1.
func (b *Buffer) Visible() (from, to int) {
	return b.top, min(b.top+b.height, len(b.lines))
}

// LineText returns line i of the text, without its line break.
func (b *Buffer) LineText(i int) string { return b.lines[i] }

// Size returns how many bytes String would return, without building it.
func (b *Buffer) Size() int {
	n := len(b.eol) * (len(b.lines) - 1)
	if b.final {
		n += len(b.eol)
	}
	for _, l := range b.lines {
		n += len(l)
	}
	return n
}

// SetSize makes the window width columns wide and height lines tall (a size
// below 1 counts as 1), then moves it to show the cursor. It doesn't end a
// run of vertical moves, and with the size unchanged it does nothing, so a
// caller can call it after every key.
func (b *Buffer) SetSize(width, height int) {
	width, height = max(width, 1), max(height, 1)
	if width == b.width && height == b.height {
		return
	}
	b.width, b.height = width, height
	b.follow()
}

// Left moves the cursor to the previous cell. At the start of a line it
// does nothing.
func (b *Buffer) Left() {
	if b.offset > 0 {
		b.offset = b.before(b.lines[b.line], b.offset)
		b.moved()
	}
}

// Right moves the cursor to the next cell. At the end of a line it does
// nothing.
func (b *Buffer) Right() {
	if s := b.lines[b.line]; b.offset < len(s) {
		size, _, _ := b.cell(s[b.offset:], 0)
		b.offset += size
		b.moved()
	}
}

// Up moves the cursor to the line above, at the goal column. On the first
// line it does nothing.
func (b *Buffer) Up() {
	if b.line > 0 {
		b.vertical(b.line - 1)
	}
}

// Down moves the cursor to the line below, at the goal column. On the last
// line it does nothing.
func (b *Buffer) Down() {
	if b.line < len(b.lines)-1 {
		b.vertical(b.line + 1)
	}
}

// PageUp scrolls the window up by its height, stopping at the first line,
// and moves the cursor up as many lines, at the goal column.
func (b *Buffer) PageUp() {
	b.top = max(b.top-b.height, 0)
	b.vertical(max(b.line-b.height, 0))
}

// PageDown scrolls the window down by its height, stopping at the last
// page, and moves the cursor down as many lines, stopping at the last line,
// at the goal column.
func (b *Buffer) PageDown() {
	b.top = max(min(b.top+b.height, len(b.lines)-b.height), 0)
	b.vertical(min(b.line+b.height, len(b.lines)-1))
}

// LineStart moves the cursor to the start of its line.
func (b *Buffer) LineStart() {
	b.offset = 0
	b.moved()
}

// LineEnd moves the cursor to the end of its line.
func (b *Buffer) LineEnd() {
	b.offset = len(b.lines[b.line])
	b.moved()
}

// Top moves the cursor to the start of the text.
func (b *Buffer) Top() {
	b.line, b.offset = 0, 0
	b.moved()
}

// Bottom moves the cursor to the end of the text.
func (b *Buffer) Bottom() {
	b.line = len(b.lines) - 1
	b.offset = len(b.lines[b.line])
	b.moved()
}

// moved ends every move but the vertical ones: it ends a run of vertical
// moves and the open undo step, and moves the window to show the cursor.
func (b *Buffer) moved() {
	b.goal = -1
	b.history.open = false
	b.follow()
}

// vertical moves the cursor to line i, onto the cell that covers the goal
// column, or to the end of a line too short to reach it. The first vertical
// move of a run sets the goal: the cursor's column.
func (b *Buffer) vertical(i int) {
	if b.goal < 0 {
		b.goal = b.column(b.lines[b.line], b.offset)
	}
	b.line = i
	b.offset = b.offsetAt(b.lines[i], b.goal)
	b.history.open = false
	b.follow()
}

// lineBreaks turns each line break Insert accepts into "\n".
var lineBreaks = strings.NewReplacer("\r\n", "\n", "\r", "\n")

// Insert puts text in at the cursor and moves the cursor after it. Each
// "\r\n", "\r" and "\n" in text breaks the line there; every other byte goes
// in as it is. Inserting "" does nothing.
func (b *Buffer) Insert(text string) {
	if text == "" {
		return
	}
	text = lineBreaks.Replace(text)
	line, col := b.line, b.offset
	b.line, b.offset = b.splice(line, col, "", text)
	b.changed()
	k := alone // pasted text, which Undo takes back whole
	if size, _, _ := b.cell(text, 0); size == len(text) {
		k = typing // one cell: a key's character, a tab, a line break, a cluster an input method typed
	}
	b.edited(k, line, col, "", text, cursor{line, col})
}

// DeleteLeft deletes the cell before the cursor or, at the start of a line,
// joins the line to the end of the line above, the cursor at the join. At
// the start of the text it does nothing.
func (b *Buffer) DeleteLeft() {
	var line, col int
	var from string
	switch s := b.lines[b.line]; {
	case b.offset > 0:
		line, col = b.line, b.before(s, b.offset)
		from = s[col:b.offset]
	case b.line > 0:
		line, col, from = b.line-1, len(b.lines[b.line-1]), "\n"
	default:
		return
	}
	before := cursor{b.line, b.offset}
	b.line, b.offset = b.splice(line, col, from, "")
	b.changed()
	b.edited(deletingLeft, line, col, from, "", before)
}

// DeleteRight deletes the cell under the cursor or, at the end of a line,
// joins the line below to this one. At the end of the text it does nothing.
func (b *Buffer) DeleteRight() {
	var from string
	switch s := b.lines[b.line]; {
	case b.offset < len(s):
		size, _, _ := b.cell(s[b.offset:], 0)
		from = s[b.offset : b.offset+size]
	case b.line < len(b.lines)-1:
		from = "\n"
	default:
		return
	}
	line, col := b.line, b.offset
	b.splice(line, col, from, "")
	b.changed()
	b.edited(deletingRight, line, col, from, "", cursor{line, col})
}

// DeleteLine deletes the cursor's line and moves the cursor to the line that
// takes its place, or to the new last line, at the goal column, as a
// vertical move does. A text of one line keeps it, emptied; when it is
// empty already, DeleteLine does nothing.
func (b *Buffer) DeleteLine() {
	i, s, n := b.line, b.lines[b.line], len(b.lines)
	var line, col int
	var from string
	switch {
	case n > 1 && i < n-1: // the line and the break after it
		line, col, from = i, 0, s+"\n"
	case n > 1: // the last of several lines: the break before it and the line
		line, col, from = i-1, len(b.lines[i-1]), "\n"+s
	case s != "": // the only line: its bytes
		line, col, from = 0, 0, s
	default:
		return
	}
	before := cursor{b.line, b.offset}
	if n > 1 && b.goal < 0 {
		b.goal = b.column(s, b.offset)
	}
	b.splice(line, col, from, "")
	if n > 1 {
		b.line = min(i, len(b.lines)-1)
		b.offset = b.offsetAt(b.lines[b.line], b.goal)
	} else {
		b.offset = 0
	}
	b.version++
	b.follow()
	b.edited(alone, line, col, from, "", before)
}

// splice replaces from, the text at offset col of line i on, with to, in
// the lines. Both texts have "\n" for a line break and every other byte as
// it is, so splice changes no byte but those: it never turns a "\r" into a
// line break. It returns where to ends: its line, and its offset in that
// line.
func (b *Buffer) splice(i, col int, from, to string) (line, offset int) {
	endLine, endCol := span(i, col, from)
	line, offset = span(i, col, to)
	parts := strings.Split(to, "\n")
	last := len(parts) - 1
	parts[0] = b.lines[i][:col] + parts[0]
	parts[last] += b.lines[endLine][endCol:]
	b.lines = slices.Replace(b.lines, i, endLine+1, parts...)
	return line, offset
}

// span returns where text, from offset col of line i on, ends: its line,
// and its offset in that line.
func span(i, col int, text string) (line, offset int) {
	n := strings.Count(text, "\n")
	if n == 0 {
		return i, col + len(text)
	}
	return i + n, len(text) - strings.LastIndexByte(text, '\n') - 1
}

// report tells the OnChange function, if there is one, that an edit
// replaced from, the text at offset col of line i on, with to. The lines
// above line i are as they were.
func (b *Buffer) report(i, col int, from, to string) {
	if b.onChange != nil {
		start := b.position(i, col)
		b.onChange(Change{Start: start, OldEnd: b.end(start, from), NewEnd: b.end(start, to)})
	}
}

// end returns the Position where text, put in at start, ends, counted in
// the bytes of the text as String writes it, with the line break for each
// "\n".
func (b *Buffer) end(start Position, text string) Position {
	line, col := span(start.Line, start.Column, text)
	return Position{Offset: start.Offset + len(text) + (line-start.Line)*(len(b.eol)-1), Line: line, Column: col}
}

// position returns the Position of offset col of line i.
func (b *Buffer) position(i, col int) Position {
	off := col
	for _, l := range b.lines[:i] {
		off += len(l) + len(b.eol)
	}
	return Position{Offset: off, Line: i, Column: col}
}

// changed ends every edit but DeleteLine: it ends a run of vertical moves,
// moves a cursor the edit left inside a cluster to the cluster's end, counts
// the change and moves the window to show the cursor.
func (b *Buffer) changed() {
	b.goal = -1
	b.snap()
	b.version++
	b.follow()
}

// snap moves the cursor to the first cell boundary at or after it. An edit
// can join characters into one cluster across the cursor, as typing a ZWJ
// between two emoji does.
func (b *Buffer) snap() {
	s, i := b.lines[b.line], 0
	for i < b.offset && i < len(s) {
		size, _, _ := b.cell(s[i:], 0)
		i += size
	}
	b.offset = i
}

// follow moves the window as little as needed to show the cursor's cell.
// Vertically, it never shows empty space below the last line while earlier
// lines are out of view. Sideways, the cursor's cell is one column past the
// end of the line, a tab's first column, or all the columns of any other
// cell; when it is out of view, the window goes back to the line start if
// the cell fits in the first width columns, else only as far as needed to
// show it, and to its first column when it is wider than the window.
func (b *Buffer) follow() {
	b.top = max(min(b.top, b.line), b.line-b.height+1)
	b.top = max(min(b.top, len(b.lines)-b.height), 0)

	s := b.lines[b.line]
	col, w := b.column(s, b.offset), 1
	if b.offset < len(s) {
		if _, cw, k := b.cell(s[b.offset:], col); k != tab {
			w = cw
		}
	}
	if col >= b.left && col+w <= b.left+b.width {
		return
	}
	switch {
	case col+w <= b.width:
		b.left = 0
	case w > b.width, col < b.left:
		b.left = col
	default:
		b.left = col + w - b.width
	}
}

// Window returns the lines in the window, top to bottom, each cut to the
// window's columns, so that none is wider than the window: a cell crossing
// the left edge is drawn as spaces for its visible columns, a tab crossing
// the right edge as spaces up to it, and drawing stops before any other
// cell crossing the right edge. On the cursor's line, the cursor's cell is
// a run of its own — a space at the line's end, a space for a tab's first
// column, whose other visible columns start the next run, a space for a
// cell wider than the whole window, and the cell itself otherwise — so it
// is never empty.
//
// classify gives the class of the cell at byte offset off of line i, and
// Window calls it once for each cell it draws any column of, in drawing
// order; a nil classify puts every cell in class 0. A cell drawn as spaces
// or as "?" takes the class of the cell it stands for, and the cursor past
// the end of a line is a space of class 0. Window doesn't change the
// buffer, so it is safe to call from a Bubble Tea View.
func (b *Buffer) Window(classify func(i, off int) int) []Line {
	from, to := b.Visible()
	lines := make([]Line, 0, to-from)
	for i := from; i < to; i++ {
		cursor := -1
		if i == b.line {
			cursor = b.offset
		}
		lines = append(lines, b.draw(i, cursor, classify))
	}
	return lines
}

// draw draws line i of the text within the window's columns, with the
// cursor's cell a run of its own when cursor, the cursor's offset in the
// line, is not -1.
func (b *Buffer) draw(i, cursor int, classify func(i, off int) int) Line {
	s, right := b.lines[i], b.left+b.width
	d := drawing{line: Line{Number: i, Cursor: -1}}
	col, off := 0, 0
	for off < len(s) && col < right {
		size, w, k := b.cell(s[off:], col)
		if col+w > right && off != cursor {
			break
		}
		if col+w > b.left || off == cursor { // drawn
			class := 0
			if classify != nil {
				class = classify(i, off)
			}
			switch {
			case off == cursor && k == tab:
				d.cursor(" ", class)
				d.add(strings.Repeat(" ", min(w, right-col)-1), class)
			case off == cursor && w > b.width: // a space stands for a cell wider than the whole window
				d.cursor(" ", class)
			case off == cursor:
				d.cursor(face(s[off:off+size], k), class)
			case col < b.left || k == tab: // crossing the left edge, or a tab
				d.add(strings.Repeat(" ", col+w-max(col, b.left)), class)
			default:
				d.add(face(s[off:off+size], k), class)
			}
		}
		off, col = off+size, col+w
	}
	if off == cursor { // the cursor past the end of the line
		d.cursor(" ", 0)
	}
	d.end()
	return d.line
}

// drawing is a window line being drawn: its runs so far, and the cells of
// the run still open.
type drawing struct {
	line  Line
	run   strings.Builder // the open run's cells
	class int             // the open run's class
}

// add draws text, cells of class: on the open run when it has that class,
// otherwise on a new one.
func (d *drawing) add(text string, class int) {
	if d.run.Len() > 0 && class != d.class {
		d.end()
	}
	d.class = class
	d.run.WriteString(text)
}

// cursor draws text, the cursor's cell, of class, as a run of its own.
func (d *drawing) cursor(text string, class int) {
	d.end()
	d.line.Cursor = len(d.line.Runs)
	d.line.Runs = append(d.line.Runs, Run{Text: text, Class: class})
}

// end closes the open run, if it has any cells.
func (d *drawing) end() {
	if d.run.Len() > 0 {
		d.line.Runs = append(d.line.Runs, Run{Text: d.run.String(), Class: d.class})
		d.run.Reset()
	}
}

// A kind says how a cell is drawn.
type kind int

const (
	shown  kind = iota // as the bytes it stands for
	tab                // as spaces up to the next tab stop
	hidden             // as "?": invalid UTF-8, a control character, or a cluster with no width
)

// face is what a cell of kind k standing for text is drawn as: one column of
// it, for a tab.
func face(text string, k kind) string {
	switch k {
	case tab:
		return " "
	case hidden:
		return "?"
	}
	return text
}

// cell returns the cell at the start of s, a non-empty part of a line that
// starts col columns into it: how many bytes of s the cell stands for, how
// many columns it takes, and how it is drawn. A cell is one grapheme
// cluster, measured by x/ansi: a tab takes the columns up to the next tab
// stop, each byte that is not valid UTF-8 is a cell of its own, and a
// cluster that holds a control character, has no width or can't stand on
// its own (see standalone) is drawn as "?", one column wide. An ASCII byte
// followed by another, or by nothing, is a cluster of its own, so most text
// never reaches x/ansi.
func (b *Buffer) cell(s string, col int) (size, width int, k kind) {
	switch c := s[0]; {
	case c == '\t':
		return 1, b.tabWidth - col%b.tabWidth, tab
	case c < utf8.RuneSelf && (len(s) == 1 || s[1] < utf8.RuneSelf):
		if c < ' ' || c == 0x7f {
			return 1, 1, hidden
		}
		return 1, 1, shown
	}
	if r, n := utf8.DecodeRuneInString(s); r == utf8.RuneError && n == 1 {
		return 1, 1, hidden
	}
	cluster, w := ansi.FirstGraphemeCluster(s, ansi.GraphemeWidth)
	if !utf8.ValidString(cluster) {
		// A combining mark can join the U+FFFD an invalid byte decodes to,
		// but a cluster never takes in an invalid byte: segment the valid
		// bytes before it instead.
		cluster, w = ansi.FirstGraphemeCluster(cluster[:validLen(cluster)], ansi.GraphemeWidth)
	}
	switch {
	case cluster == "": // x/ansi never returns one; never loop on it
		return 1, 1, hidden
	case w == 0 || strings.ContainsFunc(cluster, unicode.IsControl) || !standalone(cluster):
		return len(cluster), 1, hidden
	}
	return len(cluster), w, shown
}

// standalone reports whether cluster stays a cluster of its own between the
// characters it is drawn next to. One that starts with a mark that extends
// the character before it, as a skin tone modifier or a vowel sign alone at
// a line's start or after a tab does, or that ends with one that prepends
// itself to the next, would join the space, "?" or text drawn beside it,
// and the terminal would draw the two in fewer columns than they were given.
func standalone(cluster string) bool {
	var buf [64]byte
	before := append(append(buf[:0], 'x'), cluster...)
	if c, _ := ansi.FirstGraphemeCluster(before, ansi.GraphemeWidth); len(c) != 1 {
		return false
	}
	after := append(append(buf[:0], cluster...), 'x')
	c, _ := ansi.FirstGraphemeCluster(after, ansi.GraphemeWidth)
	return len(c) == len(cluster)
}

// validLen returns how many bytes at the start of s are valid UTF-8.
func validLen(s string) int {
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			return i
		}
		i += n
	}
	return len(s)
}

// column returns the column at which offset off of line s starts: the width
// of the cells before it.
func (b *Buffer) column(s string, off int) int {
	col := 0
	for i := 0; i < off && i < len(s); {
		size, w, _ := b.cell(s[i:], col)
		i, col = i+size, col+w
	}
	return col
}

// offsetAt returns the offset in line s of the cell that covers column col,
// or the end of s when s is too short to reach it.
func (b *Buffer) offsetAt(s string, col int) int {
	c := 0
	for i := 0; i < len(s); {
		size, w, _ := b.cell(s[i:], c)
		if c+w > col {
			return i
		}
		i, c = i+size, c+w
	}
	return len(s)
}

// before returns the offset in line s of the cell that ends at offset off.
func (b *Buffer) before(s string, off int) int {
	start := 0
	for i := 0; i < off && i < len(s); {
		size, _, _ := b.cell(s[i:], 0)
		start, i = i, i+size
	}
	return start
}
