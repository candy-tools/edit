package tabset

import (
	"slices"
	"strings"
	"testing"
)

// keyWidths is a fake measure: a tab takes one cell per byte of its key, and
// calls records the keys of every row of tabs it was asked to measure.
type keyWidths struct{ calls [][]string }

func (m *keyWidths) measure(tabs []Tab[string, int]) []int {
	keys := make([]string, len(tabs))
	widths := make([]int, len(tabs))
	for i, t := range tabs {
		keys[i] = t.Key
		widths[i] = len(t.Key)
	}
	m.calls = append(m.calls, keys)
	return widths
}

// newSet returns a set measured by keyWidths and w cells wide, with a tab
// opened for each key in order, so the last one is active. Each tab holds its
// index in keys.
func newSet(w int, keys ...string) (*Set[string, int], *keyWidths) {
	m := &keyWidths{}
	s := New(m.measure)
	s.SetWidth(w)
	for i, k := range keys {
		s.Open(k, func() int { return i })
	}
	return s, m
}

// row draws s's tabs by key, left to right: the active one in brackets and
// the window between bars, e.g. "a | b [c] | d".
func row(s *Set[string, int]) string {
	tabs, active := s.Tabs()
	first, end := s.Window()
	var parts []string
	for i, t := range tabs {
		if i == first {
			parts = append(parts, "|")
		}
		k := t.Key
		if i == active {
			k = "[" + k + "]"
		}
		parts = append(parts, k)
		if i == end-1 {
			parts = append(parts, "|")
		}
	}
	return strings.Join(parts, " ")
}

// checkRow fails the test unless row(s) is want.
func checkRow(t *testing.T, s *Set[string, int], want string) {
	t.Helper()
	if got := row(s); got != want {
		t.Errorf("row = %q, want %q", got, want)
	}
}

func TestEmptySet(t *testing.T) {
	s, _ := newSet(10)
	s.Close()
	s.Next()
	s.Prev()
	if s.Len() != 0 {
		t.Errorf("Len() = %d, want 0", s.Len())
	}
	if _, ok := s.Active(); ok {
		t.Error("Active() should report no tab")
	}
	if tabs, active := s.Tabs(); len(tabs) != 0 || active != -1 {
		t.Errorf("Tabs() = %v, %d; want no tabs and -1", tabs, active)
	}
	if first, end := s.Window(); first != 0 || end != 0 {
		t.Errorf("Window() = %d, %d; want 0, 0", first, end)
	}
}

func TestOpenAddsTabsAtTheRightEnd(t *testing.T) {
	s, _ := newSet(100, "a", "b", "c")
	checkRow(t, s, "| a b [c] |")
	if tab, ok := s.Active(); !ok || tab.Key != "c" || tab.Value != 2 {
		t.Errorf("Active() = %+v, %v; want c holding 2", tab, ok)
	}
	if s.Len() != 3 {
		t.Errorf("Len() = %d, want 3", s.Len())
	}
}

func TestOpenAnOpenKeyOnlyActivatesIt(t *testing.T) {
	s, _ := newSet(100, "a", "b", "c")
	s.Open("a", func() int {
		t.Error("value must not be called for a tab that is already open")
		return -1
	})
	checkRow(t, s, "| [a] b c |")
	if tab, _ := s.Active(); tab.Value != 0 {
		t.Errorf("a holds %d, want the 0 it was opened with", tab.Value)
	}
}

func TestCloseActivatesTheRightNeighbour(t *testing.T) {
	s, _ := newSet(100, "a", "b", "c")
	s.Prev() // b
	for _, want := range []string{
		"| a [c] |", // b's right neighbour
		"| [a] |",   // c was the last: its left neighbour
		"",          // the only tab
	} {
		s.Close()
		checkRow(t, s, want)
	}
}

func TestNextAndPrevWrapAround(t *testing.T) {
	s, _ := newSet(100, "a", "b", "c")
	for i, step := range []struct {
		move func()
		want string
	}{
		{s.Next, "| [a] b c |"},
		{s.Next, "| a [b] c |"},
		{s.Prev, "| [a] b c |"},
		{s.Prev, "| a b [c] |"},
	} {
		step.move()
		if got := row(s); got != step.want {
			t.Errorf("step %d: row = %q, want %q", i, got, step.want)
		}
	}
}

func TestASingleTabStaysActive(t *testing.T) {
	s, _ := newSet(100, "a")
	s.Next()
	s.Prev()
	checkRow(t, s, "| [a] |")
}

func TestMeasureSeesEveryTabAfterEachChange(t *testing.T) {
	s, m := newSet(100, "a", "b")
	s.Next()
	s.Close()
	s.SetWidth(50)
	want := [][]string{{}, {"a"}, {"a", "b"}, {"a", "b"}, {"b"}, {"b"}}
	if !slices.EqualFunc(m.calls, want, slices.Equal[[]string]) {
		t.Errorf("measure calls = %q, want %q", m.calls, want)
	}
}

func TestWindowMovesOnlyAsFarAsNeeded(t *testing.T) {
	s, _ := newSet(6, "aa", "bb", "cc", "dd", "ee") // 2 cells each, so 3 fit
	checkRow(t, s, "aa bb | cc dd [ee] |")
	for i, step := range []struct {
		move func()
		want string
	}{
		{s.Prev, "aa bb | cc [dd] ee |"},
		{s.Prev, "aa bb | [cc] dd ee |"},
		{s.Prev, "aa | [bb] cc dd | ee"},
		{s.Next, "aa | bb [cc] dd | ee"},
		{s.Next, "aa | bb cc [dd] | ee"},
		{s.Next, "aa bb | cc dd [ee] |"},
		{s.Next, "| [aa] bb cc | dd ee"},
	} {
		step.move()
		if got := row(s); got != step.want {
			t.Errorf("step %d: row = %q, want %q", i, got, step.want)
		}
	}
}

func TestWindowPullsBackAfterAClose(t *testing.T) {
	s, _ := newSet(6, "aa", "bb", "cc", "dd", "ee")
	s.Close() // ee: without the pull-back the strip would show just cc and dd
	checkRow(t, s, "aa | bb cc [dd] |")
}

func TestWindowFollowsTheWidth(t *testing.T) {
	s, _ := newSet(6, "aa", "bb", "cc", "dd", "ee")
	for _, step := range []struct {
		width int
		want  string
	}{
		{8, "aa | bb cc dd [ee] |"},
		{100, "| aa bb cc dd [ee] |"},
		{4, "aa bb cc | dd [ee] |"},
	} {
		s.SetWidth(step.width)
		if got := row(s); got != step.want {
			t.Errorf("width %d: row = %q, want %q", step.width, got, step.want)
		}
	}
}

func TestATabWiderThanTheStripIsShownAlone(t *testing.T) {
	s, _ := newSet(3, "a", "wide-tab", "b")
	s.Prev()
	checkRow(t, s, "a | [wide-tab] | b")
}

func TestAZeroWidthStripShowsOnlyTheActiveTab(t *testing.T) {
	for _, w := range []int{0, -3} {
		s, _ := newSet(w, "a", "b", "c")
		s.Prev()
		if got, want := row(s), "a | [b] | c"; got != want {
			t.Errorf("width %d: row = %q, want %q", w, got, want)
		}
	}
}

func TestWidthIsZeroUntilSet(t *testing.T) {
	s := New((&keyWidths{}).measure)
	s.Open("a", func() int { return 0 })
	s.Open("b", func() int { return 1 })
	checkRow(t, s, "a | [b] |")
}

func TestNegativeWidthsCountAsZero(t *testing.T) {
	s := New(func(tabs []Tab[string, int]) []int {
		widths := make([]int, len(tabs))
		for i := range widths {
			widths[i] = -5
		}
		return widths
	})
	s.Open("a", func() int { return 0 })
	s.Open("b", func() int { return 1 })
	checkRow(t, s, "| a [b] |") // both fit in the 0-cell strip
}

func TestMeasureMustReturnOneWidthPerTab(t *testing.T) {
	s := New(func([]Tab[string, int]) []int { return []int{1, 2} })
	defer func() {
		if recover() == nil {
			t.Error("a measure that returns the wrong number of widths should panic")
		}
	}()
	s.Open("a", func() int { return 0 })
}

func TestTabsReturnsACopy(t *testing.T) {
	s, _ := newSet(100, "a", "b")
	tabs, _ := s.Tabs()
	tabs[0].Key = "changed"
	checkRow(t, s, "| a [b] |")
}
