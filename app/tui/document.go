package tui

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"strconv"

	"github.com/candy-tools/trazo/libs/syntax"
	"github.com/candy-tools/trazo/libs/textedit"
)

const (
	// maxDocumentSize is the largest file a tab shows: 10 MB.
	maxDocumentSize = 10 << 20
	// binarySniffLen is how many leading bytes of a file are searched for a
	// NUL byte, which marks it as binary, as git does.
	binarySniffLen = 8000
	// tooLargeNote is what the tab of a file over maxDocumentSize shows.
	tooLargeNote = "too large to show (over 10 MB)"
	// maxUndoSteps is how many steps of its edits a tab can undo, as vim
	// keeps by default, and maxUndoSize how much text those steps hold at
	// most, the newest step aside: a few pastes of a file of maxDocumentSize.
	maxUndoSteps = 1000
	maxUndoSize  = 64 << 20
)

// document is one open file: what its tab shows and edits, how far its edits
// are saved, and how its text is colored.
type document struct {
	path string           // slash-separated, relative to the workspace root: the tab's key
	text *textedit.Buffer // the file's text, cursor and window; nil when it can't be edited
	note string           // what the tab shows instead, when text is nil

	file    *file // writes text back to the file; nil when text is
	saved   int   // the version of text on disk, as far as trazo knows
	writing bool  // a background write of text is running
	saveErr error // why the last write failed; nil once one works
	failed  int   // the version whose write failed, while saveErr is set

	syntax *syntax.Highlighter // colors text by its syntax; nil when nothing does
	src    []byte              // text, as String writes it, at version srcVer; nil until asked for
	srcVer int
}

// loadDocument reads the file at path, slash-separated, in fsys: into a text
// to edit, which write writes back, or into the note its tab shows instead —
// "⚠ " and the cause when it can't be read, "not a regular file" for a
// folder, pipe, socket or device, tooLargeNote, or "binary file".
func loadDocument(fsys fs.FS, write writeFunc, path string) *document {
	doc := &document{path: path}
	data, note := readText(fsys, path)
	if note != "" {
		doc.note = note
		return doc
	}
	doc.text = textedit.New(string(data), tabWidth)
	doc.text.LimitHistory(maxUndoSteps, maxUndoSize)
	doc.file = &file{path: path, write: write}
	return doc
}

// unsaved reports whether d's text has edits the disk doesn't have.
func (d *document) unsaved() bool {
	return d.text != nil && d.text.Version() != d.saved
}

// flush writes d's newest text now, if it has unsaved edits, waiting for a
// write in progress first, and records how it went.
func (d *document) flush() error {
	if !d.unsaved() {
		return nil
	}
	v := d.text.Version()
	err := d.file.save(v, d.text.String())
	d.record(v, err)
	return err
}

// record keeps what a write of version v of d's text says about the disk:
// that it holds version v, or why it doesn't. A background result can
// arrive after a flush wrote newer text, so only newer news counts: a
// success raises the saved version and clears a failure of a version no
// newer than v; a failure is kept only for a version newer than the saved
// one and no older than the last failure. So a late error never marks a
// saved tab, and a late success never hides a newer failure.
func (d *document) record(v int, err error) {
	switch {
	case err == nil:
		d.saved = max(d.saved, v)
		if v >= d.failed {
			d.saveErr = nil
		}
	case v > d.saved && v >= d.failed:
		d.saveErr, d.failed = err, v
	}
}

// retire stops every later write of d and frees its syntax tree: its tab
// has closed.
func (d *document) retire() {
	if d.file != nil {
		d.file.retire()
	}
	if d.syntax != nil {
		d.syntax.Close()
	}
}

// source returns d's text as bytes, as String writes it. It builds them once
// per version of the text and never changes bytes it returned, so a
// background parse can read them while the text changes on.
func (d *document) source() []byte {
	if v := d.text.Version(); d.src == nil || d.srcVer != v {
		d.src, d.srcVer = []byte(d.text.String()), v
	}
	return d.src
}

// readText reads the file at path in fsys for loadDocument: its bytes, or
// the note that stands in for them. It looks at the file before opening it,
// so it never opens a named pipe, whose open would block until something
// writes to it, and never reads more than maxDocumentSize+1 bytes, in case
// the file grew after the look.
func readText(fsys fs.FS, path string) (data []byte, note string) {
	fi, err := fs.Stat(fsys, path)
	switch {
	case err != nil:
		return nil, "⚠ " + errText(err)
	case !fi.Mode().IsRegular():
		return nil, "not a regular file"
	case fi.Size() > maxDocumentSize:
		return nil, tooLargeNote
	}
	data, err = readAtMost(fsys, path, maxDocumentSize+1)
	switch {
	case err != nil:
		return nil, "⚠ " + errText(err)
	case len(data) > maxDocumentSize:
		return nil, tooLargeNote
	case bytes.IndexByte(data[:min(len(data), binarySniffLen)], 0) >= 0:
		return nil, "binary file"
	}
	return data, ""
}

// readAtMost reads up to n bytes from the start of the file at path in fsys.
func readAtMost(fsys fs.FS, path string, n int64) ([]byte, error) {
	f, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, n))
	return data, errors.Join(err, f.Close())
}

// setSize fits the text window of d to an editor body width cells wide and
// height lines tall, less the gutter's share of the width. A document
// showing a note has no text window, so on one it does nothing.
func (d *document) setSize(width, height int) {
	if d.text != nil {
		d.text.SetSize(width-gutterWidth(d.text.Len()), height)
	}
}

// gutterWidth is how many cells the line numbers of a text of n lines take:
// the digits of n and a space on each side.
func gutterWidth(n int) int {
	return len(strconv.Itoa(n)) + 2
}
