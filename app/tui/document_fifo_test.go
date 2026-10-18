//go:build linux || darwin

package tui

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A named pipe is never opened: opening one for reading blocks until
// something writes to it, which would hang the editor.
func TestLoadDocumentNeverOpensANamedPipe(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o600); err != nil {
		t.Skipf("cannot make a named pipe here: %v", err)
	}
	done := make(chan *document, 1)
	go func() { done <- loadDocument(os.DirFS(dir), "pipe") }()
	select {
	case doc := <-done:
		checkNote(t, doc, "pipe", "not a regular file")
	case <-time.After(5 * time.Second):
		t.Fatal("loadDocument blocked on a named pipe")
	}
}
