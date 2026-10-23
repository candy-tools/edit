package textedit

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The history's limits until LimitHistory changes them: as many steps as vim
// keeps by default, and text enough for a few pastes of a large file.
const (
	defaultSteps = 1000
	defaultSize  = 64 << 20
)

// A stepKind says what edits a step is made of, and so which can extend it.
type stepKind int

const (
	typing        stepKind = iota // characters typed in a row, a word at a time
	deletingLeft                  // DeleteLeft in a row
	deletingRight                 // DeleteRight in a row
	alone                         // a paste or a deleted line, which nothing extends
)

// A cursor is a place of the cursor: a line, and a byte offset in it.
type cursor struct{ line, offset int }

// An undoStep is what one undo takes back: at offset col of line, the text
// removed was replaced with inserted, both with "\n" for a line break; the
// cursor was at before when the step began, and at after when it ended.
type undoStep struct {
	kind              stepKind
	line, col         int
	removed, inserted string
	before, after     cursor
}

// size is how many bytes of text s holds.
func (s undoStep) size() int { return len(s.removed) + len(s.inserted) }

// history is a Buffer's steps: those to undo, oldest first, and those Undo
// took back, to redo, the last one undone last.
type history struct {
	undo, redo  []undoStep
	open        bool // whether the newest undo step can still grow
	held        int  // how many bytes of text the steps of both lists hold
	steps, size int  // the most steps kept, and the most text, but for the newest step
}

// Undo takes back the last step of edits: it puts the text back as it was
// before the step, and the cursor where it was then, and moves the window
// to show it. A step is the characters typed in a row, a word with the
// white space typed after it at a time; DeleteLeft in a row, or DeleteRight
// in a row; or a paste or a DeleteLine. A move ends a step, and so does an
// edit of another kind. With nothing to undo, Undo does nothing.
func (b *Buffer) Undo() {
	h := &b.history
	if len(h.undo) == 0 {
		return
	}
	s := pop(&h.undo)
	h.redo, h.open = append(h.redo, s), false
	b.splice(s.line, s.col, s.inserted, s.removed)
	b.line, b.offset = s.before.line, s.before.offset
	b.changed()
	b.report(s.line, s.col, s.inserted, s.removed)
}

// Redo applies again the last step Undo took back, and puts the cursor
// where it was after it. With nothing to redo, or after an edit that
// changed the text since that undo, it does nothing.
func (b *Buffer) Redo() {
	h := &b.history
	if len(h.redo) == 0 {
		return
	}
	s := pop(&h.redo)
	h.undo, h.open = append(h.undo, s), false
	b.splice(s.line, s.col, s.removed, s.inserted)
	b.line, b.offset = s.after.line, s.after.offset
	b.changed()
	b.report(s.line, s.col, s.removed, s.inserted)
}

// LimitHistory makes b keep at most steps undo steps, and drop its oldest
// steps while the text they hold is over size bytes, but for the newest,
// whatever its size. A limit below 1 counts as 1. The limits apply from the
// next edit on; New's Buffer keeps 1000 steps and 64 MiB.
func (b *Buffer) LimitHistory(steps, size int) {
	b.history.steps, b.history.size = max(steps, 1), max(size, 1)
}

// edited ends an edit of kind k that replaced from, the text at offset col
// of line i on, with to, the cursor having been at before: it tells the
// OnChange function, and records the edit, with the cursor where the edit
// left it.
func (b *Buffer) edited(k stepKind, i, col int, from, to string, before cursor) {
	b.report(i, col, from, to)
	b.history.record(k, i, col, from, to, before, cursor{b.line, b.offset})
}

// record adds an edit of kind k to the history: it replaced from, at
// offset col of line i, with to, and moved the cursor from before to
// after. It extends the open step when the edit continues it; otherwise the
// edit starts a step of its own, which drops the steps there were to redo.
// Either way it then drops the oldest steps the limits don't keep. The
// removed text is cloned: it can be a part of a long line, which it would
// otherwise keep whole.
func (h *history) record(k stepKind, i, col int, from, to string, before, after cursor) {
	if !h.extend(k, i, col, from, to, after) {
		h.dropRedo()
		h.undo = append(h.undo, undoStep{kind: k, line: i, col: col, removed: strings.Clone(from), inserted: to, before: before, after: after})
		h.held += len(from) + len(to)
		h.open = k != alone
	}
	h.trim()
}

// extend grows the open step with an edit of kind k that continues it, and
// reports whether it did: typing where the step's typed text ends, unless
// it starts a word; DeleteLeft where the step's deleted text starts; and
// DeleteRight at the step's own place.
func (h *history) extend(k stepKind, i, col int, from, to string, after cursor) bool {
	if !h.open {
		return false
	}
	s := &h.undo[len(h.undo)-1]
	switch {
	case k != s.kind:
		return false
	case k == typing:
		if line, offset := span(s.line, s.col, s.inserted); line != i || offset != col || startsWord(s.inserted, to) {
			return false
		}
		s.inserted += to
	case k == deletingLeft:
		if line, offset := span(i, col, from); line != s.line || offset != s.col {
			return false
		}
		s.line, s.col, s.removed = i, col, from+s.removed
	default: // deletingRight: a step alone is never open
		if i != s.line || col != s.col {
			return false
		}
		s.removed += from
	}
	s.after = after
	h.held += len(from) + len(to)
	return true
}

// startsWord reports whether typing text after typed starts a word: its
// first character is not white space, and typed's last one is.
func startsWord(typed, text string) bool {
	last, _ := utf8.DecodeLastRuneInString(typed)
	first, _ := utf8.DecodeRuneInString(text)
	return unicode.IsSpace(last) && !unicode.IsSpace(first)
}

// dropRedo drops the steps there were to redo, clearing their slots so
// they don't stay reachable from there.
func (h *history) dropRedo() {
	for _, s := range h.redo {
		h.held -= s.size()
	}
	clear(h.redo)
	h.redo = h.redo[:0]
}

// trim drops the oldest steps while there are more than the limit, or while
// the text the steps hold is over the limit and more than one step is left:
// the newest step is kept, however large.
func (h *history) trim() {
	n := 0
	for n < len(h.undo)-1 && (len(h.undo)-n > h.steps || h.held > h.size) {
		h.held -= h.undo[n].size()
		n++
	}
	h.undo = slices.Delete(h.undo, 0, n)
}

// pop takes the last step off steps, zeroing its slot so it doesn't stay
// reachable from there.
func pop(steps *[]undoStep) undoStep {
	last := len(*steps) - 1
	s := (*steps)[last]
	(*steps)[last] = undoStep{}
	*steps = (*steps)[:last]
	return s
}
