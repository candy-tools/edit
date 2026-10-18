package tui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"
)

// listing names es in order, marking folders with a trailing "/".
func listing(es []entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.name
		if e.dir {
			out[i] += "/"
		}
	}
	return out
}

// mustChildren lists the folder e of ws, failing the test on an error.
func mustChildren(t *testing.T, ws workspace, e entry) []entry {
	t.Helper()
	es, err := ws.Children(e)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

// mustSymlink creates link pointing at target, failing the test on an error.
func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceSortsFoldersFirstCaseInsensitive(t *testing.T) {
	fsys := fstest.MapFS{
		"b.txt":     {},
		"a.txt":     {},
		"A.txt":     {},
		".env":      {},
		"README.md": {},
		"Zeta/z":    {},
		"alpha/a":   {},
		".git/HEAD": {},
	}
	got := mustChildren(t, workspace{fsys}, rootEntry)
	want := []string{".git/", "alpha/", "Zeta/", ".env", "A.txt", "a.txt", "b.txt", "README.md"}
	if !slices.Equal(listing(got), want) {
		t.Errorf("listing = %q, want %q", listing(got), want)
	}
}

func TestWorkspaceNestedEntries(t *testing.T) {
	ws := workspace{fstest.MapFS{"app/tui/tui.go": {}}}
	app := mustChildren(t, ws, rootEntry)
	tui := mustChildren(t, ws, app[0])
	files := mustChildren(t, ws, tui[0])
	for _, c := range []struct{ got, want entry }{
		{app[0], entry{name: "app", path: "app", dir: true}},
		{tui[0], entry{name: "tui", path: "app/tui", dir: true}},
		{files[0], entry{name: "tui.go", path: "app/tui/tui.go"}},
	} {
		if c.got != c.want {
			t.Errorf("entry = %+v, want %+v", c.got, c.want)
		}
	}
	if !ws.Branch(app[0]) || ws.Branch(files[0]) {
		t.Error("folders should be branches and files leaves")
	}
}

func TestWorkspaceMissingFolder(t *testing.T) {
	_, err := workspace{fstest.MapFS{}}.Children(entry{name: "gone", path: "gone", dir: true})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("listing a missing folder: err = %v, want fs.ErrNotExist", err)
	}
}

func TestWorkspaceFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "real"), 0o750); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, "real", filepath.Join(dir, "link"))
	mustSymlink(t, "missing", filepath.Join(dir, "broken"))
	got := mustChildren(t, workspace{os.DirFS(dir)}, rootEntry)
	if want := []string{"link/", "real/", "broken"}; !slices.Equal(listing(got), want) {
		t.Errorf("listing = %q, want %q", listing(got), want)
	}
}

// fs.FS paths must be valid UTF-8, so a folder whose name is not is listed
// but cannot be opened: expanding it shows an error row.
func TestWorkspaceNonUTF8FolderCannotBeOpened(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "bad\xff"), 0o750); err != nil {
		t.Skipf("this filesystem rejects non-UTF-8 names: %v", err)
	}
	ws := workspace{os.DirFS(dir)}
	top := mustChildren(t, ws, rootEntry)
	if want := []string{"bad\xff/"}; !slices.Equal(listing(top), want) {
		t.Fatalf("listing = %q, want %q", listing(top), want)
	}
	if _, err := ws.Children(top[0]); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("opening it: err = %v, want fs.ErrInvalid", err)
	}
}

// Review focus: a link to its own folder expands one level per keypress and
// never recurses by itself.
func TestWorkspaceSymlinkLoopExpandsOneLevelAtATime(t *testing.T) {
	dir := t.TempDir()
	mustSymlink(t, ".", filepath.Join(dir, "loop"))
	ws := workspace{os.DirFS(dir)}
	top := mustChildren(t, ws, rootEntry)
	inner := mustChildren(t, ws, top[0])
	if want := []string{"loop/"}; !slices.Equal(listing(top), want) || !slices.Equal(listing(inner), want) {
		t.Errorf("listings = %q then %q, want %q both times", listing(top), listing(inner), want)
	}
	if inner[0].path != "loop/loop" {
		t.Errorf("nested path = %q, want %q", inner[0].path, "loop/loop")
	}
}
