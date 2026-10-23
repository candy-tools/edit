package picker

import (
	"slices"
	"strings"
	"testing"
)

// drawn returns p's query line as drawn, its cursor's cell between "[" and
// "]".
func drawn(p *Picker[int]) string {
	l := p.QueryLine()
	var b strings.Builder
	for i, r := range l.Runs {
		if i == l.Cursor {
			b.WriteString("[" + r.Text + "]")
		} else {
			b.WriteString(r.Text)
		}
	}
	return b.String()
}

// sized returns a new Picker, its query window w cells wide and its list h
// items tall.
func sized(w, h int) *Picker[int] {
	p := New[int]()
	p.SetSize(w, h)
	return p
}

func TestTypingMovingAndDeletingInTheQuery(t *testing.T) {
	p := sized(20, 3)
	p.Type("ab")
	p.Type("👍🏽")
	p.Left() // over the whole cluster
	p.DeleteLeft()
	if got, want := drawn(p), "a[👍🏽]"; got != want {
		t.Errorf("drawn %q, want %q", got, want)
	}
	p.Start()
	p.DeleteRight()
	p.End()
	if got, want := drawn(p), "👍🏽[ ]"; got != want || p.Query() != "👍🏽" {
		t.Errorf("drawn %q, query %q; want %q", got, p.Query(), want)
	}
	p.Right() // at the end: nothing
	if p.Query() != "👍🏽" {
		t.Errorf("query %q after a move", p.Query())
	}
}

func TestLineBreaksTypeAsSpaces(t *testing.T) {
	p := sized(20, 3)
	p.Type("a\r\nb\nc\rd")
	if got := p.Query(); got != "a b c d" {
		t.Errorf("query %q, want the line breaks as spaces", got)
	}
}

func TestAPrefilledQuery(t *testing.T) {
	for _, c := range []struct {
		name string
		do   func(p *Picker[int])
		want string
	}{
		{"typing replaces it", func(p *Picker[int]) { p.Type("n") }, "n"},
		{"backspace clears it", (*Picker[int]).DeleteLeft, ""},
		{"delete clears it", (*Picker[int]).DeleteRight, ""},
		{"a move keeps it", func(p *Picker[int]) { p.Left(); p.Type("x") }, "olxd"},
		{"typing nothing keeps it", func(p *Picker[int]) { p.Type("") }, "old"},
	} {
		p := sized(20, 3)
		p.Prefill("old")
		if !p.Prefilled() || drawn(p) != "old[ ]" {
			t.Fatalf("Prefill: prefilled %t, drawn %q; want true, old with the cursor after it", p.Prefilled(), drawn(p))
		}
		c.do(p)
		if p.Query() != c.want {
			t.Errorf("%s: query %q, want %q", c.name, p.Query(), c.want)
		}
	}
	p := sized(20, 3)
	p.Prefill("old")
	p.Left()
	if p.Prefilled() {
		t.Error("a move should end the selection")
	}
	p.Prefill("")
	if p.Prefilled() {
		t.Error("an empty query should not be selected")
	}
	p.Prefill("a\nb")
	if p.Query() != "a b" {
		t.Errorf("Prefill: query %q, want one line", p.Query())
	}
}

func TestALongQueryScrollsSideways(t *testing.T) {
	p := sized(5, 1)
	p.Type("abcdefgh")
	if got, want := drawn(p), "efgh[ ]"; got != want {
		t.Errorf("drawn %q, want %q", got, want)
	}
	p.Start()
	if got, want := drawn(p), "[a]bcde"; got != want {
		t.Errorf("drawn %q, want %q", got, want)
	}
}

// checkWindow fails the test unless p's window shows want, its cursor on
// want's item cursor.
func checkWindow(t *testing.T, p *Picker[int], cursor int, want ...int) {
	t.Helper()
	if items, c := p.Window(); !slices.Equal(items, want) || c != cursor {
		t.Errorf("window %v, cursor %d; want %v, %d", items, c, want, cursor)
	}
}

// numbers returns the items 0 to n-1.
func numbers(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func TestAnEmptyList(t *testing.T) {
	p := sized(10, 3)
	p.Up()
	p.PageDown()
	p.Select(4)
	if _, ok := p.Current(); ok || p.Len() != 0 {
		t.Error("an empty list has no current item")
	}
	if items, cursor := p.Window(); items != nil || cursor != -1 {
		t.Errorf("window %v, %d; want nil, -1", items, cursor)
	}
}

func TestMovingThroughTheList(t *testing.T) {
	p := sized(10, 3)
	p.SetItems(numbers(10))
	checkWindow(t, p, 0, 0, 1, 2)
	p.Down()
	p.Down()
	p.Down()
	checkWindow(t, p, 2, 1, 2, 3)
	p.PageDown()
	checkWindow(t, p, 2, 4, 5, 6)
	p.PageDown()
	p.PageDown()
	checkWindow(t, p, 2, 7, 8, 9)
	p.Up()
	checkWindow(t, p, 1, 7, 8, 9)
	for range 4 {
		p.PageUp()
	}
	checkWindow(t, p, 0, 0, 1, 2)
	if item, ok := p.Current(); !ok || item != 0 {
		t.Errorf("current %d, %t; want 0", item, ok)
	}
}

func TestAppendKeepsTheCursorAndTheWindow(t *testing.T) {
	p := sized(10, 3)
	p.Append(0, 1) // to an empty list: the cursor on the first
	if item, _ := p.Current(); item != 0 {
		t.Errorf("current %d, want 0", item)
	}
	p.Down()
	p.Append(2, 3, 4)
	checkWindow(t, p, 1, 0, 1, 2)
}

func TestSelectCentersAnItemOutOfView(t *testing.T) {
	p := sized(10, 5)
	p.SetItems(numbers(100))
	p.Select(50)
	checkWindow(t, p, 2, 48, 49, 50, 51, 52)
	p.Select(51) // in view: the window stays
	checkWindow(t, p, 3, 48, 49, 50, 51, 52)
	p.Select(120)
	checkWindow(t, p, 4, 95, 96, 97, 98, 99)
	p.Select(-5)
	checkWindow(t, p, 0, 0, 1, 2, 3, 4)
}

func TestSetItemsCopiesAndPutsTheCursorFirst(t *testing.T) {
	p := sized(10, 3)
	items := []int{1, 2, 3}
	p.SetItems(items)
	p.Down()
	items[0] = 9
	checkWindow(t, p, 1, 1, 2, 3)
	p.SetItems([]int{4, 5})
	checkWindow(t, p, 0, 4, 5)
}

func TestASmallerHeightKeepsTheCursorInView(t *testing.T) {
	p := sized(10, 5)
	p.SetItems(numbers(10))
	p.Select(4)
	p.SetSize(10, 2)
	checkWindow(t, p, 1, 3, 4)
}
