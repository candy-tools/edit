package tui

import (
	"cmp"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/candy-tools/edit/libs/treeview"
)

// entry is one file or folder of the workspace.
type entry struct {
	name string // base name, as listed
	path string // slash-separated, relative to the workspace root; "." is the root
	dir  bool   // a folder, or a symlink to one
}

// rootEntry is the workspace root: the hidden root of the sidebar's file
// tree.
var rootEntry = entry{name: ".", path: ".", dir: true}

// workspace lists the workspace's folders for the sidebar's file tree: it is
// the tree's treeview.Source.
type workspace struct{ fsys fs.FS }

var _ treeview.Source[entry] = workspace{}

// Branch reports whether e is a folder: folders expand, files don't.
func (w workspace) Branch(e entry) bool { return e.dir }

// Children lists the folder e: folders first, then files, each in
// case-insensitive name order with ties broken by exact name. Hidden entries
// are included. A symlink is followed to tell whether it is a folder; a
// broken one is listed as a file.
func (w workspace) Children(e entry) ([]entry, error) {
	des, err := fs.ReadDir(w.fsys, e.path)
	if err != nil {
		return nil, err
	}
	out := make([]entry, len(des))
	for i, d := range des {
		p := path.Join(e.path, d.Name())
		out[i] = entry{name: d.Name(), path: p, dir: w.isDir(d, p)}
	}
	slices.SortFunc(out, compareEntries)
	return out, nil
}

// isDir reports whether d, found at p, is a folder, following a symlink to
// see what it points at.
func (w workspace) isDir(d fs.DirEntry, p string) bool {
	if d.Type()&fs.ModeSymlink == 0 {
		return d.IsDir()
	}
	fi, err := fs.Stat(w.fsys, p)
	return err == nil && fi.IsDir()
}

// compareEntries orders folders before files, then names case-insensitively,
// then by exact name, so the order is total.
func compareEntries(a, b entry) int {
	if a.dir != b.dir {
		if a.dir {
			return -1
		}
		return 1
	}
	return cmp.Or(
		strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)),
		strings.Compare(a.name, b.name),
	)
}

// compareDirEntries orders a folder's entries for the workspace search as
// the sidebar lists them: folders first, then by name, as compareEntries
// does. The search leaves symlinks out, so an entry's own type is enough.
func compareDirEntries(a, b fs.DirEntry) int {
	return compareEntries(entry{name: a.Name(), dir: a.IsDir()}, entry{name: b.Name(), dir: b.IsDir()})
}
