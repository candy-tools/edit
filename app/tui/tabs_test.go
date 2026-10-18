package tui

import (
	"slices"
	"testing"

	"github.com/candy-tools/edit/libs/tabset"
	"github.com/charmbracelet/x/ansi"
)

// openPaths returns a tab set measured by measureTabs and w cells wide, with
// a document opened for each path in order, so the last one is active.
func openPaths(w int, paths ...string) *tabset.Set[string, *document] {
	set := tabset.New(measureTabs)
	set.SetWidth(w)
	for _, p := range paths {
		set.Open(p, func() *document { return &document{path: p, note: "test"} })
	}
	return set
}

func TestTabTitles(t *testing.T) {
	tabs, _ := openPaths(100, "README.md", "docs/README.md", "app/main.go", "a\nb.txt", "bad\xff.txt").Tabs()
	want := []string{"README.md", "docs/README.md", "main.go", "a?b.txt", "bad?.txt"}
	if got := tabTitles(tabs); !slices.Equal(got, want) {
		t.Errorf("titles = %q, want %q", got, want)
	}
}

func TestMeasureTabs(t *testing.T) {
	tabs, _ := openPaths(100, "go.mod", "app/日本.go").Tabs()
	if got, want := measureTabs(tabs), []int{9, 10}; !slices.Equal(got, want) {
		t.Errorf("widths = %v, want %v: the title, a space on each side and a dash", got, want)
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
	set := openPaths(100, "main.go", "go.mod", "README.md")
	for _, focused := range []bool{true, false} {
		if got, want := ansi.Strip(tabStrip(set, focused)), " main.go ─ go.mod ─ README.md "; got != want {
			t.Errorf("focused %v: strip = %q, want %q", focused, got, want)
		}
	}
	if got := tabStrip(openPaths(100), true); got != "" {
		t.Errorf("with no tab open the strip should be empty, got %q", got)
	}
}

func TestTabStripMarksHiddenTabs(t *testing.T) {
	// The tabs take 10, 9, 12, 9 and 11 cells: 21 cells hold two or three.
	set := openPaths(21, "main.go", "go.mod", "README.md", "tui.go", "files.go")
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
		if got := ansi.Strip(tabStrip(set, true)); got != step.want {
			t.Errorf("strip = %q, want %q", got, step.want)
		}
	}
}
