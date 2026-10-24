package tui

import (
	"path"
	"strings"

	"github.com/candy-tools/trazo/libs/tabset"
	"charm.land/lipgloss/v2"
)

// openTab is one of the editor's tabs: an open document, keyed by its path.
type openTab = tabset.Tab[string, *document]

// tabTitles returns the titles of tabs, left to right: each file's name or,
// when two or more of the tabs share that name, its path relative to the
// workspace, made safe to draw by displayName, after a "⚠ " while the tab's
// last write failed.
func tabTitles(tabs []openTab) []string {
	names := make(map[string]int, len(tabs))
	for _, t := range tabs {
		names[path.Base(t.Key)]++
	}
	titles := make([]string, len(tabs))
	for i, t := range tabs {
		title := path.Base(t.Key)
		if names[title] > 1 {
			title = t.Key
		}
		titles[i] = displayName(title)
		if t.Value.saveErr != nil {
			titles[i] = "⚠ " + titles[i]
		}
	}
	return titles
}

// tabLabel is what the strip draws for the tab titled title: the title with
// a space on each side and, with the mouse on, the tab's × and a space,
// which a click closes the tab on.
func tabLabel(title string, mouse bool) string {
	if mouse {
		return " " + title + " × "
	}
	return " " + title + " "
}

// measureTabs is the tab set's measure, with the mouse on or off: how many
// cells each tab takes in the strip, which is its label and the "─" drawn
// after it.
func measureTabs(tabs []openTab, mouse bool) []int {
	titles := tabTitles(tabs)
	widths := make([]int, len(titles))
	for i, title := range titles {
		widths[i] = lipgloss.Width(tabLabel(title, mouse)) + 1
	}
	return widths
}

// stripRow is the screen row the strip is drawn on: the editor box's top
// border, under the header.
const stripRow = 1

// stripLeft is the screen column the strip starts on at terminal width w:
// after the sidebar and the editor box's "╭─".
func stripLeft(w int) int {
	return sidebarWidth(w) + 2
}

// stripRoom is how many cells the editor box's top border has for the
// strip at terminal width w: frame cuts the strip off there.
func stripRoom(w int) int {
	return max(w-sidebarWidth(w)-5, 0)
}

// stripWidth is the width the tab set gets for the strip at terminal width w:
// the room frame leaves for it, less 2 cells at each end for the "…"
// markers, plus 1 because no "─" is drawn after the last tab.
func stripWidth(w int) int {
	return stripRoom(w) - 3
}

// tabStrip draws the tabs inside the window of set for the editor pane's top
// border, left to right, each with its × while the mouse is on: the active
// tab accented while the editor has focus (bold otherwise), the rest dim,
// separated by border dashes. A dim "…" marks each end beyond which tabs are
// hidden.
func tabStrip(set *tabset.Set[string, *document], focused, mouse bool) string {
	tabs, active := set.Tabs()
	first, end := set.Window()
	titles := tabTitles(tabs)
	dash := borderStyle(focused).Render("─")
	parts := make([]string, 0, end-first)
	for i := first; i < end; i++ {
		st := titleDimStyle
		if i == active {
			st = activeTabStyle
			if focused {
				st = titleActiveStyle
			}
		}
		parts = append(parts, st.Render(tabLabel(titles[i], mouse)))
	}
	strip := strings.Join(parts, dash)
	if first > 0 {
		strip = titleDimStyle.Render("…") + dash + strip
	}
	if end < len(tabs) {
		strip += dash + titleDimStyle.Render("…")
	}
	return strip
}

// stripTabAt returns the index into set's Tabs of the tab whose label
// tabStrip draws on cell x of the strip, counted from its left end, which
// the border shows room cells of; with the mouse on, onX says whether the
// cell is the tab's × or the space after it. It returns false for a cell
// with no tab: a marker, a dash, or one past what the border shows.
func stripTabAt(set *tabset.Set[string, *document], mouse bool, x, room int) (i int, onX, ok bool) {
	if x < 0 || x >= room {
		return 0, false, false
	}
	tabs, _ := set.Tabs()
	first, end := set.Window()
	widths := measureTabs(tabs, mouse)
	left := 0
	if first > 0 {
		left = 2 // the "…" and a dash
	}
	for i := first; i < end; i++ {
		label := widths[i] - 1 // without the dash after it
		if x >= left && x < left+label {
			return i, mouse && x >= left+label-2, true
		}
		left += widths[i]
	}
	return 0, false, false
}
