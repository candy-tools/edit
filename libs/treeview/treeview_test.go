package treeview

import (
	"errors"
	"strings"
	"testing"
)

var errLoad = errors.New("load failed")

// fakeSource is an in-memory Source over string nodes. A node is a branch
// when kids has an entry for it, even an empty one; fail makes loading a
// node fail; calls counts the Children calls per node.
type fakeSource struct {
	kids  map[string][]string
	fail  map[string]bool
	calls map[string]int
}

func (s *fakeSource) Branch(n string) bool {
	_, ok := s.kids[n]
	return ok
}

func (s *fakeSource) Children(n string) ([]string, error) {
	s.calls[n]++
	if s.fail[n] {
		return []string{"ignored"}, errLoad // nodes returned with an error must not show
	}
	return s.kids[n], nil
}

// newSource returns a fakeSource rooted at "/" over this tree:
//
//	a        branch
//	  a1     branch
//	    x
//	  a2
//	b        branch, empty
//	c
func newSource() *fakeSource {
	return &fakeSource{
		kids: map[string][]string{
			"/":  {"a", "b", "c"},
			"a":  {"a1", "a2"},
			"a1": {"x"},
			"b":  {},
		},
		fail:  map[string]bool{},
		calls: map[string]int{},
	}
}

// flatSource returns a fakeSource whose root has just the given leaves.
func flatSource(leaves ...string) *fakeSource {
	src := newSource()
	src.kids["/"] = leaves
	return src
}

// tall builds a Tree over src with a window tall enough for every row.
func tall(src *fakeSource) *Tree[string] {
	t := New(src, "/")
	t.SetHeight(100)
	return t
}

// render draws the window one row per line: two spaces per depth, a marker
// ("+" closed branch, "-" open branch, "!" error row, " " leaf), the node or
// the error, and "<" on the cursor row.
func render(t *Tree[string]) string {
	rows, cursor := t.Window()
	out := make([]string, len(rows))
	for i, r := range rows {
		mark, name := " ", r.Node
		switch {
		case r.Err != nil:
			mark, name = "!", r.Err.Error()
		case r.Branch && r.Expanded:
			mark = "-"
		case r.Branch:
			mark = "+"
		}
		out[i] = strings.Repeat("  ", r.Depth) + mark + name
		if i == cursor {
			out[i] += "<"
		}
	}
	return strings.Join(out, "\n")
}

// lines joins rows into the multi-line form render returns.
func lines(rows ...string) string { return strings.Join(rows, "\n") }

// check fails the test when tr does not render as want.
func check(t *testing.T, tr *Tree[string], want string) {
	t.Helper()
	if got := render(tr); got != want {
		t.Errorf("tree =\n%s\nwant\n%s", got, want)
	}
}

func TestNewShowsTheRootsChildren(t *testing.T) {
	src := newSource()
	tr := tall(src)
	check(t, tr, lines("+a<", "+b", " c"))
	if src.calls["/"] != 1 {
		t.Errorf("New loaded the root %d times, want once", src.calls["/"])
	}
}

func TestExpand(t *testing.T) {
	tr := tall(newSource())
	tr.Expand() // a is closed: open it, the cursor stays
	check(t, tr, lines("-a<", "  +a1", "   a2", "+b", " c"))
	tr.Expand() // a is open: move to its first child
	check(t, tr, lines("-a", "  +a1<", "   a2", "+b", " c"))
	tr.Down()
	tr.Expand() // a2 is a leaf: nothing
	check(t, tr, lines("-a", "  +a1", "   a2<", "+b", " c"))
	tr.Down()
	tr.Expand() // b is closed and empty: open it
	tr.Expand() // b is open with no children: nothing
	check(t, tr, lines("-a", "  +a1", "   a2", "-b<", " c"))
}

func TestCollapse(t *testing.T) {
	tr := tall(newSource())
	tr.Expand()
	tr.Down()
	tr.Expand()
	tr.Down()
	check(t, tr, lines("-a", "  -a1", "     x<", "   a2", "+b", " c"))
	tr.Collapse() // x is a leaf: move to its parent
	check(t, tr, lines("-a", "  -a1<", "     x", "   a2", "+b", " c"))
	tr.Collapse() // a1 is open: close it
	check(t, tr, lines("-a", "  +a1<", "   a2", "+b", " c"))
	tr.Collapse() // a1 is closed: move to its parent
	check(t, tr, lines("-a<", "  +a1", "   a2", "+b", " c"))
	tr.Collapse() // a is open: close it
	check(t, tr, lines("+a<", "+b", " c"))
	tr.Collapse() // closed, at depth 0: nothing
	check(t, tr, lines("+a<", "+b", " c"))
}

func TestCollapseForgetsWhatWasOpenInside(t *testing.T) {
	src := newSource()
	tr := tall(src)
	tr.Expand()
	tr.Down()
	tr.Expand() // a and a1 open
	tr.Up()
	tr.Collapse()
	tr.Expand() // reopen a
	check(t, tr, lines("-a<", "  +a1", "   a2", "+b", " c"))
	if src.calls["a"] != 2 {
		t.Errorf("a was loaded %d times, want 2: reopening must reload it", src.calls["a"])
	}
}

func TestToggle(t *testing.T) {
	tr := tall(newSource())
	tr.Toggle() // opens a, the cursor stays
	check(t, tr, lines("-a<", "  +a1", "   a2", "+b", " c"))
	tr.Toggle() // closes a, the cursor stays
	check(t, tr, lines("+a<", "+b", " c"))
	tr.Bottom()
	tr.Toggle() // c is a leaf: nothing
	check(t, tr, lines("+a", "+b", " c<"))
}

func TestLoadErrorShowsAnErrorRow(t *testing.T) {
	src := newSource()
	src.fail["a"] = true
	tr := tall(src)
	tr.Expand()
	check(t, tr, lines("-a<", "  !load failed", "+b", " c"))

	tr.Expand() // a is open: move to its first child, the error row
	row, ok := tr.Selected()
	if !ok || !errors.Is(row.Err, errLoad) || row.Node != "" || row.Branch || row.Expanded || row.Depth != 1 {
		t.Errorf("error row = %+v, want a depth-1 row carrying errLoad and nothing else", row)
	}
	tr.Expand()
	tr.Toggle() // nothing on an error row
	check(t, tr, lines("-a", "  !load failed<", "+b", " c"))
	tr.Collapse() // on an error row: move to the branch that failed
	check(t, tr, lines("-a<", "  !load failed", "+b", " c"))

	src.fail["a"] = false
	tr.Collapse()
	tr.Expand() // reopening retries
	check(t, tr, lines("-a<", "  +a1", "   a2", "+b", " c"))
}

func TestRootLoadError(t *testing.T) {
	src := newSource()
	src.fail["/"] = true
	tr := tall(src)
	tr.Expand()
	tr.Collapse()
	tr.Toggle()
	check(t, tr, lines("!load failed<"))
	if row, ok := tr.Selected(); !ok || row.Depth != 0 || !errors.Is(row.Err, errLoad) {
		t.Errorf("Selected() = %+v, %v; want the depth-0 error row", row, ok)
	}
}

func TestEmptyTree(t *testing.T) {
	tr := tall(flatSource())
	tr.Up()
	tr.Down()
	tr.PageUp()
	tr.PageDown()
	tr.Top()
	tr.Bottom()
	tr.Expand()
	tr.Collapse()
	tr.Toggle()
	if rows, cursor := tr.Window(); len(rows) != 0 || cursor != -1 {
		t.Errorf("Window() = %v, %d; want no rows and cursor -1", rows, cursor)
	}
	if _, ok := tr.Selected(); ok {
		t.Error("Selected() should report no row on an empty tree")
	}
}

func TestCursorStopsAtTheEnds(t *testing.T) {
	tr := tall(newSource()) // rows a, b, c; a page is 100 rows
	for i, step := range []struct {
		move func()
		want string
	}{
		{tr.Up, "a"},
		{tr.Down, "b"},
		{tr.Down, "c"},
		{tr.Down, "c"},
		{tr.PageUp, "a"},
		{tr.PageDown, "c"},
		{tr.Top, "a"},
		{tr.Bottom, "c"},
	} {
		step.move()
		if row, _ := tr.Selected(); row.Node != step.want {
			t.Errorf("step %d: cursor on %q, want %q", i, row.Node, step.want)
		}
	}
}

func TestWindowScrollsOnlyToFollowTheCursor(t *testing.T) {
	tr := New(flatSource("n0", "n1", "n2", "n3", "n4", "n5", "n6", "n7", "n8", "n9"), "/")
	tr.SetHeight(4)
	check(t, tr, lines(" n0<", " n1", " n2", " n3"))
	tr.Down()
	tr.Down()
	tr.Down() // to n3: still inside the window
	check(t, tr, lines(" n0", " n1", " n2", " n3<"))
	tr.Down() // to n4: scroll one row
	check(t, tr, lines(" n1", " n2", " n3", " n4<"))
	tr.Up()
	tr.Up()
	tr.Up() // to n1: inside the window, so no scrolling back
	check(t, tr, lines(" n1<", " n2", " n3", " n4"))
	tr.PageDown() // a page is the window height: n1 to n5
	check(t, tr, lines(" n2", " n3", " n4", " n5<"))
	tr.Bottom()
	check(t, tr, lines(" n6", " n7", " n8", " n9<"))
	tr.PageUp() // n9 to n5
	check(t, tr, lines(" n5<", " n6", " n7", " n8"))
	tr.Top()
	check(t, tr, lines(" n0<", " n1", " n2", " n3"))
}

func TestCollapseNearTheBottomLeavesNoEmptySpace(t *testing.T) {
	src := flatSource("x0", "x1", "x2", "x3", "x4", "x5", "b")
	src.kids["b"] = []string{"b0", "b1", "b2", "b3"}
	tr := New(src, "/")
	tr.SetHeight(4)
	tr.Bottom()
	tr.Expand() // open b, the last row
	for range 4 {
		tr.Down()
	}
	for range 4 {
		tr.Up()
	}
	check(t, tr, lines("-b<", "   b0", "   b1", "   b2"))
	tr.Collapse() // 4 rows fewer: the window slides back up to stay full
	check(t, tr, lines(" x3", " x4", " x5", "+b<"))
}

func TestSetHeight(t *testing.T) {
	src := flatSource("n0", "n1", "n2", "n3", "n4", "n5", "n6", "n7", "n8", "n9")
	if got := render(New(src, "/")); got != " n0<" {
		t.Errorf("before SetHeight the window should be one row, got\n%s", got)
	}
	tr := tall(src)
	tr.Bottom()
	tr.SetHeight(3) // shrinking keeps the cursor in view
	check(t, tr, lines(" n7", " n8", " n9<"))
	tr.SetHeight(0) // below 1 counts as 1
	check(t, tr, lines(" n9<"))
	tr.SetHeight(-5)
	check(t, tr, lines(" n9<"))
}
