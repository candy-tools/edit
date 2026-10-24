package tui

import (
	"path"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/candy-tools/trazo/libs/grep"
	"github.com/charmbracelet/x/ansi"
)

const (
	// searchMaxWidth and searchMaxHeight are the popup's largest size, and
	// searchMinWidth and searchMinHeight its smallest but on a smaller
	// screen, which it fills.
	searchMaxWidth, searchMaxHeight = 100, 30
	searchMinWidth, searchMinHeight = 20, 3
	// findPrompt starts the popup's first row, before the query.
	findPrompt = " Find: "
	// countRoom is the cells the first row keeps for the count, on its
	// right.
	countRoom = 28
)

// searchBox returns where the popup is drawn on a screen w by h: its left
// column and top row, and its width and height. It is 4 columns narrower
// and 4 rows shorter than the screen, so the panes show around it, but at
// most searchMaxWidth by searchMaxHeight, and at least searchMinWidth by
// searchMinHeight, or the whole screen when that is smaller.
func searchBox(w, h int) (x, y, width, height int) {
	width = min(max(min(w-4, searchMaxWidth), searchMinWidth), w)
	height = min(max(min(h-4, searchMaxHeight), searchMinHeight), h)
	return (w - width) / 2, (h - height) / 2, width, height
}

// queryWidth is the width of the query's window in a popup width cells
// wide: what its first row leaves between the prompt and the count's room
// or, in a popup too narrow to leave the query 10 cells that way, all the
// row leaves after the prompt; the count then gets what the query leaves.
func queryWidth(width int) int {
	inner := width - 2 - len(findPrompt) - 1
	if room := inner - countRoom; room >= 10 {
		return room
	}
	return max(inner, 1)
}

// overlaySearch lays the search popup over frame, a screen w by h, where
// searchBox puts it, with lipgloss's compositor, which measures characters
// as grapheme clusters, as trazo does.
func (m model) overlaySearch(frame string, w, h int) string {
	x, y, width, height := searchBox(w, h)
	popup := lipgloss.NewLayer(m.searchView(width, height)).X(x).Y(y).Z(1)
	return lipgloss.NewCompositor(lipgloss.NewLayer(frame), popup).Render()
}

// searchView draws the popup, width by height: its box, titled, around its
// first row, the query and the count, and the rows of the matches in view.
func (m model) searchView(width, height int) string {
	p := m.search
	inner := width - 2
	rows := append([]string{p.queryRow(inner)}, p.matchRows(inner)...)
	return titledBox(p.title(), strings.Join(rows, "\n"), width, height, true)
}

// title is the popup's title: "Search", and the name of the file it
// searches.
func (p *searchPopup) title() string {
	if p.doc == nil {
		return "Search"
	}
	return "Search " + displayName(path.Base(p.doc.path))
}

// queryRow draws the popup's first row, inner cells wide: the prompt, the
// query — its cursor's cell in reverse video, or all of it while it is the
// selected query the popup opened with — and the count on the right, cut to
// the room left.
func (p *searchPopup) queryRow(inner int) string {
	var b strings.Builder
	b.WriteString(findPrompt)
	line := p.picker.QueryLine()
	for i, r := range line.Runs {
		if i == line.Cursor || p.picker.Prefilled() {
			b.WriteString(cursorStyle.Styled(r.Text))
		} else {
			b.WriteString(r.Text)
		}
	}
	left := b.String()
	count := helpTextStyle.Render(ansi.Truncate(p.count(), max(inner-lipgloss.Width(left)-2, 0), "…"))
	pad := max(inner-lipgloss.Width(left)-lipgloss.Width(count)-1, 1)
	return left + strings.Repeat(" ", pad) + count + " "
}

// count is what the popup's first row says of the search: nothing with no
// query; "searching…" while it runs; why the workspace couldn't be read; or
// how many matches it found, a "+" after the number when it stopped at its
// limit, and in how many files, for the workspace.
func (p *searchPopup) count() string {
	switch {
	case p.picker.Query() == "":
		return ""
	case p.running:
		return "searching…"
	case p.err != nil:
		return "⚠ " + errText(p.err)
	case p.matches == 0:
		return "no matches"
	}
	s := strconv.Itoa(p.matches)
	if p.more {
		s += "+"
	}
	s += " " + plural(p.matches, "match", "matches")
	if p.doc == nil {
		s += " in " + strconv.Itoa(p.files) + " " + plural(p.files, "file", "files")
	}
	return s
}

// plural returns one when n is 1, and many otherwise.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// matchRows draws the rows of the matches in the list's window, inner cells
// wide: the gutter, "❯" on the list's cursor row; where the match is — the
// path and the line for the workspace, the line for a file — in a column as
// wide as the widest in view, at most a third of the row; then the line.
func (p *searchPopup) matchRows(inner int) []string {
	matches, cursor := p.picker.Window()
	places := make([]string, len(matches))
	col := 0
	for i, mt := range matches {
		places[i] = strconv.Itoa(mt.Line + 1)
		if p.doc == nil {
			places[i] = displayName(mt.Path) + ":" + places[i]
		}
		col = max(col, ansi.StringWidth(places[i]))
	}
	col = min(col, max(inner/3, 1))
	rows := make([]string, len(matches))
	for i, mt := range matches {
		gutter, style := "  ", helpTextStyle
		if i == cursor {
			gutter, style = selectedRowStyle.Render("❯ "), selectedRowStyle
		}
		place := style.Render(fitPlace(places[i], col, p.doc == nil))
		rows[i] = gutter + place + "  " + matchLine(mt, inner-2-col-2)
	}
	return rows
}

// fitPlace fits place in col cells: cut at its start, "…" in front, when it
// is wider; then padded, after a path, before a line number, so the paths
// line up on the left and the numbers on the right.
func fitPlace(place string, col int, isPath bool) string {
	if w := ansi.StringWidth(place); w > col {
		cut := place
		for n := w - col + 1; ansi.StringWidth(cut) > col-1 && n <= w; n++ {
			cut = ansi.TruncateLeft(place, n, "")
		}
		place = "…" + cut
	}
	pad := strings.Repeat(" ", max(col-ansi.StringWidth(place), 0))
	if isPath {
		return place + pad
	}
	return pad + place
}

// matchLine draws the line of m in room cells: its indentation dropped, but
// never part of the match; its tabs as spaces and its control characters
// and invalid bytes as "?"; the match highlighted. A line too long for room
// is cut at its end and, when the match wouldn't show whole, at its start
// too, so that the match starts a quarter of the way into room; "…" marks
// each cut.
func matchLine(m grep.Match, room int) string {
	if room < 1 {
		return ""
	}
	before := m.Text[:m.Start]
	before = safeText(strings.TrimLeft(before, " \t"))
	match, after := safeText(m.Text[m.Start:m.End]), safeText(m.Text[m.End:])
	if bw := ansi.StringWidth(before); bw+ansi.StringWidth(match) > room {
		cut := ""
		if keep := room / 4; keep > 0 {
			cut = "…" + ansi.TruncateLeft(before, bw-keep+1, "")
		}
		before = cut
	}
	return ansi.Truncate(before+matchStyle.Render(match)+after, room, "…")
}

// safeText makes a part of a line safe to draw: each tab a space, and each
// control character and run of invalid bytes "?", as displayName does.
func safeText(s string) string { return displayName(strings.ReplaceAll(s, "\t", " ")) }
