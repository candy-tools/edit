package tui

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"
)

// docLines returns all of doc's lines, failing the test when doc holds a note
// instead of text.
func docLines(t *testing.T, doc *document) []string {
	t.Helper()
	if doc.text == nil {
		t.Fatalf("document %q has no text, only the note %q", doc.path, doc.note)
	}
	doc.setHeight(doc.text.Len())
	lines, _ := doc.text.Window()
	return lines
}

// checkNote fails the test unless doc, loaded from path, holds just the note
// want.
func checkNote(t *testing.T, doc *document, path, want string) {
	t.Helper()
	if doc.path != path || doc.text != nil || doc.note != want {
		t.Errorf("document = {path: %q, text: %v, note: %q}, want the note %q for %q",
			doc.path, doc.text != nil, doc.note, want, path)
	}
}

// withNULAt returns n bytes of text with a NUL byte at index i.
func withNULAt(n, i int) []byte {
	b := bytes.Repeat([]byte("a"), n)
	b[i] = 0
	return b
}

func TestLoadDocumentReadsText(t *testing.T) {
	fsys := fstest.MapFS{
		"app/main.go": {Data: []byte("package main\n\nfunc main() {}\n")},
		"empty":       {},
		"dos.txt":     {Data: []byte("one\r\ntwo\r\n")},
		"late-nul":    {Data: withNULAt(binarySniffLen+1, binarySniffLen)},
	}
	for _, c := range []struct {
		path string
		want []string
	}{
		{"app/main.go", []string{"package main", "", "func main() {}"}},
		{"empty", []string{""}},
		{"dos.txt", []string{"one", "two"}},
		{"late-nul", []string{string(withNULAt(binarySniffLen+1, binarySniffLen))}},
	} {
		doc := loadDocument(fsys, c.path)
		if doc.path != c.path || doc.note != "" {
			t.Errorf("%s: path %q, note %q; want its text", c.path, doc.path, doc.note)
			continue
		}
		if got := docLines(t, doc); !slices.Equal(got, c.want) {
			t.Errorf("%s: lines = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestLoadDocumentNotes(t *testing.T) {
	fsys := fstest.MapFS{
		"logo.png": {Data: withNULAt(binarySniffLen, binarySniffLen-1)},
		"pipe":     {Mode: fs.ModeNamedPipe},
		"app/x":    {},
	}
	for _, c := range []struct{ path, want string }{
		{"logo.png", "binary file"},
		{"pipe", "not a regular file"},
		{"app", "not a regular file"},
		{"gone", "⚠ file does not exist"},
	} {
		checkNote(t, loadDocument(fsys, c.path), c.path, c.want)
	}
	checkNote(t, loadDocument(deniedFS{}, "secret"), "secret", "⚠ permission denied")
}

func TestLoadDocumentSizeLimit(t *testing.T) {
	fsys := fstest.MapFS{
		"limit":   {Data: bytes.Repeat([]byte("a"), maxDocumentSize)},
		"too-big": {Data: bytes.Repeat([]byte("a"), maxDocumentSize+1)},
	}
	if doc := loadDocument(fsys, "limit"); doc.text == nil {
		t.Errorf("a file of exactly 10 MB should be shown, got the note %q", doc.note)
	}
	checkNote(t, loadDocument(fsys, "too-big"), "too-big", "too large to show (over 10 MB)")
}

// grownFS is a filesystem whose Stat reports every file as 1 byte long, as if
// each file grew between being looked at and being read.
type grownFS struct{ fstest.MapFS }

func (f grownFS) Stat(name string) (fs.FileInfo, error) {
	fi, err := f.MapFS.Stat(name)
	if err != nil {
		return nil, err
	}
	return oneByteInfo{fi}, nil
}

// oneByteInfo is a FileInfo that reports a size of 1 byte.
type oneByteInfo struct{ fs.FileInfo }

func (oneByteInfo) Size() int64 { return 1 }

func TestLoadDocumentCapsTheRead(t *testing.T) {
	fsys := grownFS{fstest.MapFS{"log": {Data: bytes.Repeat([]byte("a"), maxDocumentSize+1)}}}
	checkNote(t, loadDocument(fsys, "log"), "log", "too large to show (over 10 MB)")
}

// lockedFS is a filesystem that lets every file be looked at but not opened,
// as when a file's permissions change between the two.
type lockedFS struct{ fstest.MapFS }

func (lockedFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
}

func TestLoadDocumentWhenOpeningFails(t *testing.T) {
	fsys := lockedFS{fstest.MapFS{"notes.txt": {Data: []byte("hi")}}}
	checkNote(t, loadDocument(fsys, "notes.txt"), "notes.txt", "⚠ permission denied")
}

func TestLoadDocumentBrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	mustSymlink(t, "missing", filepath.Join(dir, "broken"))
	checkNote(t, loadDocument(os.DirFS(dir), "broken"), "broken", "⚠ no such file or directory")
}

// Review focus: a file you can't read on a real disk can still be looked at,
// so it is the open that fails, and the tab shows the short cause, not
// "open /abs/path: permission denied".
func TestLoadDocumentOnARealUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read any file")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "secret"), []byte("hidden"), 0o000); err != nil {
		t.Fatal(err)
	}
	checkNote(t, loadDocument(os.DirFS(dir), "secret"), "secret", "⚠ permission denied")
}

// Review focus: fs.FS paths must be valid UTF-8, so a file whose name is not
// is listed in the tree but can't be read: its tab shows the error.
func TestLoadDocumentNonUTF8Name(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad\xff.txt"), []byte("text"), 0o600); err != nil {
		t.Skipf("this filesystem rejects non-UTF-8 names: %v", err)
	}
	checkNote(t, loadDocument(os.DirFS(dir), "bad\xff.txt"), "bad\xff.txt", "⚠ invalid argument")
}

func TestSetHeightOnANoteDoesNothing(t *testing.T) {
	doc := &document{path: "logo.png", note: "binary file"}
	doc.setHeight(10)
	if doc.text != nil {
		t.Error("a document showing a note must not get a text window")
	}
}
