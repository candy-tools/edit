package tui

import (
	"path"
	"strings"

	"github.com/candy-tools/edit/libs/tabset"
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

// measureTabs is the tab set's measure: how many cells each tab takes in the
// strip, which is its title with a space on each side, and the "─" drawn
// after it.
func measureTabs(tabs []openTab) []int {
	titles := tabTitles(tabs)
	widths := make([]int, len(titles))
	for i, title := range titles {
		widths[i] = lipgloss.Width(title) + 3
	}
	return widths
}

// stripWidth is the width the tab set gets for the strip at terminal width w:
// the room frame leaves for the editor box's label, less 2 cells at each end
// for the "…" markers, plus 1 because no "─" is drawn after the last tab.
func stripWidth(w int) int {
	return max(w-sidebarWidth(w)-5, 0) - 3
}

// tabStrip draws the tabs inside the window of set for the editor pane's top
// border, left to right: the active tab accented while the editor has focus
// (bold otherwise), the rest dim, separated by border dashes. A dim "…" marks
// each end beyond which tabs are hidden.
func tabStrip(set *tabset.Set[string, *document], focused bool) string {
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
		parts = append(parts, st.Render(" "+titles[i]+" "))
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
