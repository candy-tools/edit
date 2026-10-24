package tui

import (
	"errors"
	"slices"
	"testing"

	"github.com/candy-tools/trazo/libs/tabset"
	"github.com/charmbracelet/x/ansi"
)

// openPaths returns a tab set measured by measureTabs, with the mouse on or
// off, and w cells wide, with a document opened for each path in order, so
// the last one is active.
func openPaths(w int, mouse bool, paths ...string) *tabset.Set[string, *document] {
	set := tabset.New(func(tabs []openTab) []int { return measureTabs(tabs, mouse) })
	set.SetWidth(w)
	for _, p := range paths {
		set.Open(p, func() *document { return &document{path: p, note: "test"} })
	}
	return set
}

func TestTabTitles(t *testing.T) {
	tabs, _ := openPaths(100, false, "README.md", "docs/README.md", "app/main.go", "a\nb.txt", "bad\xff.txt").Tabs()
	want := []string{"README.md", "docs/README.md", "main.go", "a?b.txt", "bad?.txt"}
	if got := tabTitles(tabs); !slices.Equal(got, want) {
		t.Errorf("titles = %q, want %q", got, want)
	}
}

func TestTabTitlesMarkAFailedWrite(t *testing.T) {
	set := openPaths(100, false, "go.mod", "main.go")
	tabs, _ := set.Tabs()
	tabs[0].Value.saveErr = errors.New("permission denied")
	if got, want := tabTitles(tabs), []string{"⚠ go.mod", "main.go"}; !slices.Equal(got, want) {
		t.Errorf("titles = %q, want %q", got, want)
	}
	if got := measureTabs(tabs, false); got[0] != len("go.mod")+5 {
		t.Errorf("widths = %v: the mark takes 2 more cells", got)
	}
}

func TestMeasureTabs(t *testing.T) {
	tabs, _ := openPaths(100, false, "go.mod", "app/日本.go").Tabs()
	if got, want := measureTabs(tabs, false), []int{9, 10}; !slices.Equal(got, want) {
		t.Errorf("widths = %v, want %v: the title, a space on each side and a dash", got, want)
	}
	if got, want := measureTabs(tabs, true), []int{11, 12}; !slices.Equal(got, want) {
		t.Errorf("with the mouse on: widths = %v, want %v: the × and a space more", got, want)
	}
}

func TestStripWidth(t *testing.T) {
	for _, c := range []struct{ w, want int }{{100, 62}, {80, 46}, {12, 0}, {3, -3}} {
		if got := stripWidth(c.w); got != c.want {
			t.Errorf("stripWidth(%d) = %d, want %d", c.w, got, c.want)
		}
	}
}

func TestTabStrip(t *testing.T) {
	set := openPaths(100, false, "main.go", "go.mod", "README.md")
	for _, focused := range []bool{true, false} {
		if got, want := ansi.Strip(tabStrip(set, focused, false)), " main.go ─ go.mod ─ README.md "; got != want {
			t.Errorf("focused %v: strip = %q, want %q", focused, got, want)
		}
	}
	if got := tabStrip(openPaths(100, false), true, false); got != "" {
		t.Errorf("with no tab open the strip should be empty, got %q", got)
	}
}

func TestTabStripWithTheMouseOn(t *testing.T) {
	set := openPaths(100, true, "main.go", "go.mod", "README.md")
	if got, want := ansi.Strip(tabStrip(set, true, true)), " main.go × ─ go.mod × ─ README.md × "; got != want {
		t.Errorf("strip = %q, want %q: each tab's ×", got, want)
	}
}

func TestStripTabAt(t *testing.T) {
	// " main.go × ─ go.mod × ─ README.md × ": main.go is cells 0 to 10, its
	// × the last two; a dash; go.mod is 12 to 21; a dash; README.md 23 to 35.
	set := openPaths(100, true, "main.go", "go.mod", "README.md")
	for _, c := range []struct {
		mouse   bool
		x       int
		tab     int // -1 for none
		onClose bool
	}{
		{true, 0, 0, false}, {true, 8, 0, false}, {true, 9, 0, true}, {true, 10, 0, true},
		{true, 11, -1, false}, {true, 12, 1, false}, {true, 20, 1, true}, {true, 35, 2, true},
		{true, 36, -1, false}, {true, -1, -1, false},
		{false, 8, 0, false}, {false, 9, -1, false}, // " main.go ─ go.mod ─ README.md ", no ×
	} {
		tab, onClose, ok := stripTabAt(set, c.mouse, c.x, 100)
		if !ok {
			tab = -1
		}
		if tab != c.tab || onClose != c.onClose {
			t.Errorf("mouse %t, cell %d: tab %d, on its × %t; want %d, %t", c.mouse, c.x, tab, onClose, c.tab, c.onClose)
		}
	}
}

func TestStripTabAtSkipsTheMarkAndWhatTheBorderCutsOff(t *testing.T) {
	set := openPaths(10, true, "aa.go", "bb.go") // "…─ bb.go × ": 10 cells hold one tab
	for _, c := range []struct {
		x, room, tab int // tab is -1 for none
	}{
		{0, 100, -1}, {1, 100, -1}, {2, 100, 1}, {10, 100, 1},
		{9, 9, -1}, // the border has room for 9 cells: the × is cut off
	} {
		tab, _, ok := stripTabAt(set, true, c.x, c.room)
		if !ok {
			tab = -1
		}
		if tab != c.tab {
			t.Errorf("cell %d of %d: tab %d, want %d", c.x, c.room, tab, c.tab)
		}
	}
}

func TestTabStripMarksHiddenTabs(t *testing.T) {
	// The tabs take 10, 9, 12, 9 and 11 cells: 21 cells hold two or three.
	set := openPaths(21, false, "main.go", "go.mod", "README.md", "tui.go", "files.go")
	for _, step := range []struct {
		move func()
		want string
	}{
		{func() {}, "…─ tui.go ─ files.go "},
		{set.Prev, "…─ tui.go ─ files.go "},
		{set.Prev, "…─ README.md ─ tui.go ─…"},
		{set.Next, "…─ README.md ─ tui.go ─…"},
		{set.Next, "…─ tui.go ─ files.go "},
		{set.Next, " main.go ─ go.mod ─…"},
	} {
		step.move()
		if got := ansi.Strip(tabStrip(set, true, false)); got != step.want {
			t.Errorf("strip = %q, want %q", got, step.want)
		}
	}
}
