// Package treeview is the navigation state of a lazily loaded tree shown as
// an indented list: which branches are open, the rows that are visible, a
// cursor over them and a scroll window that follows it. It knows nothing
// about what the nodes are or how they are drawn: a Source supplies them and
// the caller draws the rows Window returns.
//
// It is app-agnostic and depends only on the standard library; a depguard
// rule in .golangci.yaml keeps it from importing the app.
package treeview

import "slices"

// Source supplies the nodes of a Tree.
type Source[T any] interface {
	// Branch reports whether n can be expanded, whether or not it turns out
	// to have children. A Tree asks once for each node a Children call
	// returns, right after the call.
	Branch(n T) bool
	// Children returns n's children in display order. A Tree calls it every
	// time n is expanded, since collapsing discards what was loaded below n.
	// When it returns an error, any nodes it also returned are ignored.
	Children(n T) ([]T, error)
}

// Row is one visible line of a Tree.
type Row[T any] struct {
	Node     T    // the node on this row; the zero T on an error row
	Depth    int  // nesting level: 0 for the root's children
	Branch   bool // Node can be expanded
	Expanded bool // Node is open: its children, or an error row, follow
	// Err is set only on an error row, which stands in for the children of
	// the branch above it that failed to load.
	Err error
}

// node is one loaded node of a Tree. An error row is a node too: the only
// child of a branch whose children failed to load, carrying the error.
type node[T any] struct {
	val      T
	parent   *node[T]
	depth    int
	branch   bool
	expanded bool
	children []*node[T]
	err      error
}

// row is the Row that n is shown as.
func (n *node[T]) row() Row[T] {
	return Row[T]{Node: n.val, Depth: n.depth, Branch: n.branch, Expanded: n.expanded, Err: n.err}
}

// Tree is the navigation state over a Source: the open branches, the visible
// rows, the cursor and the scroll window. It is not safe for concurrent use.
type Tree[T any] struct {
	src    Source[T]
	root   *node[T]   // hidden and always open; its children are the depth-0 rows
	rows   []*node[T] // the visible nodes, top to bottom
	cursor int        // index into rows of the cursor row
	offset int        // index into rows of the first row in the window
	height int        // how many rows the window shows, at least 1
}

// New returns a Tree over root's descendants. root itself is never shown: it
// is always open, and its children, loaded now, are the depth-0 rows. The
// cursor starts on the first row, and the window is one row tall until
// SetHeight is called.
func New[T any](src Source[T], root T) *Tree[T] {
	t := &Tree[T]{src: src, root: &node[T]{val: root, depth: -1, branch: true}, height: 1}
	t.load(t.root)
	t.rebuild()
	return t
}

// SetHeight makes the scroll window h rows tall (a height below 1 counts as
// 1) and scrolls it to keep the cursor in view.
func (t *Tree[T]) SetHeight(h int) {
	t.height = max(h, 1)
	t.scroll()
}

// Up moves the cursor one row up, stopping at the first row.
func (t *Tree[T]) Up() { t.moveTo(t.cursor - 1) }

// Down moves the cursor one row down, stopping at the last row.
func (t *Tree[T]) Down() { t.moveTo(t.cursor + 1) }

// PageUp moves the cursor one window height up, stopping at the first row.
func (t *Tree[T]) PageUp() { t.moveTo(t.cursor - t.height) }

// PageDown moves the cursor one window height down, stopping at the last row.
func (t *Tree[T]) PageDown() { t.moveTo(t.cursor + t.height) }

// Top moves the cursor to the first row.
func (t *Tree[T]) Top() { t.moveTo(0) }

// Bottom moves the cursor to the last row.
func (t *Tree[T]) Bottom() { t.moveTo(len(t.rows) - 1) }

// Expand opens the closed branch under the cursor, loading its children, and
// leaves the cursor on it. On an open branch it moves the cursor to the first
// child row, if there is one. On a leaf or an error row it does nothing.
func (t *Tree[T]) Expand() {
	n := t.current()
	if n == nil || !n.branch {
		return
	}
	if !n.expanded {
		t.load(n)
		t.rebuild()
		return
	}
	if len(n.children) > 0 {
		t.moveTo(t.cursor + 1)
	}
}

// Collapse closes the open branch under the cursor, discarding everything
// loaded below it, and leaves the cursor on it. On any other row it moves the
// cursor to the parent row; at depth 0 it does nothing.
func (t *Tree[T]) Collapse() {
	n := t.current()
	if n == nil {
		return
	}
	if n.expanded {
		t.unload(n)
		t.rebuild()
		return
	}
	if n.parent != t.root {
		t.moveTo(slices.Index(t.rows, n.parent))
	}
}

// Toggle opens the closed branch under the cursor or closes the open one, as
// Expand and Collapse do, but never moves the cursor. On a leaf or an error
// row it does nothing.
func (t *Tree[T]) Toggle() {
	n := t.current()
	if n == nil || !n.branch {
		return
	}
	if n.expanded {
		t.unload(n)
	} else {
		t.load(n)
	}
	t.rebuild()
}

// Selected returns the cursor row, or false when the tree has no rows.
func (t *Tree[T]) Selected() (Row[T], bool) {
	n := t.current()
	if n == nil {
		return Row[T]{}, false
	}
	return n.row(), true
}

// Window returns the rows inside the scroll window, top to bottom, and the
// cursor's index among them (-1 when the tree has no rows). It does not
// change the tree, so it is safe to call from a Bubble Tea View.
func (t *Tree[T]) Window() (rows []Row[T], cursor int) {
	if len(t.rows) == 0 {
		return nil, -1
	}
	end := min(t.offset+t.height, len(t.rows))
	rows = make([]Row[T], 0, end-t.offset)
	for _, n := range t.rows[t.offset:end] {
		rows = append(rows, n.row())
	}
	return rows, t.cursor - t.offset
}

// current returns the node under the cursor, or nil when the tree has no
// rows.
func (t *Tree[T]) current() *node[T] {
	if len(t.rows) == 0 {
		return nil
	}
	return t.rows[t.cursor]
}

// load opens n and loads its children from the Source. If that fails, a
// single error row becomes n's only child.
func (t *Tree[T]) load(n *node[T]) {
	n.expanded = true
	vals, err := t.src.Children(n.val)
	if err != nil {
		n.children = []*node[T]{{parent: n, depth: n.depth + 1, err: err}}
		return
	}
	n.children = make([]*node[T], len(vals))
	for i, v := range vals {
		n.children[i] = &node[T]{val: v, parent: n, depth: n.depth + 1, branch: t.src.Branch(v)}
	}
}

// unload closes n and discards everything loaded below it, so the next load
// starts afresh: folders that were open inside n come back closed.
func (t *Tree[T]) unload(n *node[T]) {
	n.expanded = false
	n.children = nil
}

// rebuild recomputes the visible rows after a branch opened or closed,
// keeping the cursor on the node it was on.
func (t *Tree[T]) rebuild() {
	cur := t.current()
	t.rows = t.rows[:0]
	t.appendOpen(t.root.children)
	t.cursor = max(slices.Index(t.rows, cur), 0)
	t.scroll()
}

// appendOpen appends nodes to the visible rows, each followed by its own
// visible rows when it is open.
func (t *Tree[T]) appendOpen(nodes []*node[T]) {
	for _, n := range nodes {
		t.rows = append(t.rows, n)
		if n.expanded {
			t.appendOpen(n.children)
		}
	}
}

// moveTo puts the cursor on row i, clamped to the rows, and scrolls the
// window to keep it in view. On a tree with no rows it does nothing.
func (t *Tree[T]) moveTo(i int) {
	if len(t.rows) == 0 {
		return
	}
	t.cursor = min(max(i, 0), len(t.rows)-1)
	t.scroll()
}

// scroll moves the window only as far as needed to keep the cursor inside
// it, then clamps it so it never starts before the first row, nor shows empty
// space below the last row while earlier rows are out of view.
func (t *Tree[T]) scroll() {
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+t.height {
		t.offset = t.cursor - t.height + 1
	}
	t.offset = max(min(t.offset, len(t.rows)-t.height), 0)
}
