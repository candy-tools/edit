package textview

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// numbered returns a text of n lines, "l0" to "l<n-1>", each ended by "\n".
func numbered(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "l%d\n", i)
	}
	return b.String()
}

// check fails the test unless v's window shows want, starting at line first.
func check(t *testing.T, v *View, first int, want ...string) {
	t.Helper()
	lines, f := v.Window()
	if f != first || !slices.Equal(lines, want) {
		t.Errorf("window = %q from line %d, want %q from line %d", lines, f, want, first)
	}
}

func TestNewSplitsLines(t *testing.T) {
	for _, c := range []struct {
		text string
		want []string
	}{
		{"", []string{""}},
		{"a", []string{"a"}},
		{"a\n", []string{"a"}},
		{"a\n\n", []string{"a", ""}},
		{"\n", []string{""}},
		{"a\nb", []string{"a", "b"}},
		{"a\r\nb\r\n", []string{"a", "b"}},
		{"a\rb", []string{"a\rb"}},
		{"a\r", []string{"a\r"}},
		{"\tx\x1b[0m\xff", []string{"\tx\x1b[0m\xff"}},
	} {
		v := New(c.text)
		v.SetHeight(10)
		if lines, first := v.Window(); first != 0 || !slices.Equal(lines, c.want) || v.Len() != len(c.want) {
			t.Errorf("New(%q): window %q from line %d, Len %d; want %q from line 0", c.text, lines, first, v.Len(), c.want)
		}
	}
}

func TestHeightIsOneUntilSet(t *testing.T) {
	v := New("a\nb\nc")
	check(t, v, 0, "a")
	v.Down()
	check(t, v, 1, "b")
}

func TestHeightBelowOneCountsAsOne(t *testing.T) {
	v := New("a\nb\nc")
	for _, h := range []int{0, -5} {
		v.SetHeight(h)
		check(t, v, 0, "a")
	}
}

func TestShortTextNeverScrolls(t *testing.T) {
	v := New("a\nb")
	v.SetHeight(5)
	for _, move := range []func(){v.Down, v.PageDown, v.Bottom, v.Up, v.PageUp, v.Top} {
		move()
		check(t, v, 0, "a", "b")
	}
}

func TestMovesStopAtTheEnds(t *testing.T) {
	v := New(numbered(10))
	v.SetHeight(4) // the last page starts at line 6
	for i, step := range []struct {
		move  func()
		first int
	}{
		{v.Up, 0},
		{v.Down, 1},
		{v.PageDown, 5},
		{v.PageDown, 6}, // stops with the last line at the bottom
		{v.Down, 6},
		{v.PageUp, 2},
		{v.PageUp, 0},
		{v.Bottom, 6},
		{v.Top, 0},
	} {
		step.move()
		if _, first := v.Window(); first != step.first {
			t.Errorf("step %d: window starts at line %d, want %d", i, first, step.first)
		}
	}
}

func TestWindowShowsTheLinesFromItsFirst(t *testing.T) {
	v := New(numbered(10))
	v.SetHeight(3)
	v.Bottom()
	check(t, v, 7, "l7", "l8", "l9")
}

func TestSetHeightKeepsTheFirstLine(t *testing.T) {
	v := New(numbered(10))
	v.SetHeight(3)
	v.Down()
	v.Down() // lines 2 to 4
	v.SetHeight(2)
	check(t, v, 2, "l2", "l3")
	v.SetHeight(5)
	check(t, v, 2, "l2", "l3", "l4", "l5", "l6")
}

func TestGrowingNearTheEndPullsTheWindowUp(t *testing.T) {
	v := New(numbered(10))
	v.SetHeight(3)
	v.Bottom() // lines 7 to 9
	v.SetHeight(5)
	check(t, v, 5, "l5", "l6", "l7", "l8", "l9")
}

func TestWindowReturnsACopy(t *testing.T) {
	v := New("a\nb")
	v.SetHeight(2)
	lines, _ := v.Window()
	lines[0] = "changed"
	check(t, v, 0, "a", "b")
}
