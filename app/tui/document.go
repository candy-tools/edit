package tui

import (
	"bytes"
	"errors"
	"io"
	"io/fs"

	"github.com/candy-tools/edit/libs/textview"
)

const (
	// maxDocumentSize is the largest file a tab shows: 10 MB.
	maxDocumentSize = 10 << 20
	// binarySniffLen is how many leading bytes of a file are searched for a
	// NUL byte, which marks it as binary, as git does.
	binarySniffLen = 8000
	// tooLargeNote is what the tab of a file over maxDocumentSize shows.
	tooLargeNote = "too large to show (over 10 MB)"
)

// document is one open file: what its tab shows.
type document struct {
	path string         // slash-separated, relative to the workspace root: the tab's key
	text *textview.View // the file's lines and scroll window; nil when it can't be shown
	note string         // what the tab shows instead, when text is nil
}

// loadDocument reads the file at path, slash-separated, in fsys: into a text
// view, or into the note its tab shows instead — "⚠ " and the cause when it
// can't be read, "not a regular file" for a folder, pipe, socket or device,
// tooLargeNote, or "binary file".
func loadDocument(fsys fs.FS, path string) *document {
	doc := &document{path: path}
	data, note := readText(fsys, path)
	if note != "" {
		doc.note = note
		return doc
	}
	doc.text = textview.New(string(data))
	return doc
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

// setHeight makes the text window of d h lines tall. A document showing a
// note has no text window, so on one it does nothing.
func (d *document) setHeight(h int) {
	if d.text != nil {
		d.text.SetHeight(h)
	}
}
