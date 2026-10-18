package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Palette (256-colour), shared with the todo and dibs TUIs so the candy-tools
// apps feel of a piece.
var (
	colAccent = lipgloss.Color("205") // hot pink: focus border + key hints + active tab + tree cursor
	colDim    = lipgloss.Color("240") // gray: unfocused borders, inactive tabs, hints
	colTitle  = lipgloss.Color("141") // purple: app name in the header + folder names
)

var (
	borderActiveStyle = lipgloss.NewStyle().Foreground(colAccent)
	borderDimStyle    = lipgloss.NewStyle().Foreground(colDim)
	titleActiveStyle  = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	titleDimStyle     = lipgloss.NewStyle().Foreground(colDim)
	// activeTabStyle marks the shown tab while the editor pane is unfocused:
	// bold rather than accented, so it still stands out from the dim inactive
	// tabs without competing with the focused pane's accent.
	activeTabStyle = lipgloss.NewStyle().Bold(true)

	// folderStyle marks a folder's name in the file tree, and
	// selectedRowStyle draws the tree's cursor row while the sidebar has
	// focus, both as in todo's tree. plainStyle is the identity style, for
	// text drawn unstyled.
	folderStyle      = lipgloss.NewStyle().Foreground(colTitle).Bold(true)
	selectedRowStyle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	plainStyle       = lipgloss.NewStyle()

	helpKeyStyle   = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	helpTextStyle  = lipgloss.NewStyle().Foreground(colDim)
	headerAppStyle = lipgloss.NewStyle().Foreground(colTitle).Bold(true)
	headerHintSty  = lipgloss.NewStyle().Foreground(colDim)
)

// borderStyle colours a pane's border: accent when the pane has focus, dim
// otherwise.
func borderStyle(focused bool) lipgloss.Style {
	if focused {
		return borderActiveStyle
	}
	return borderDimStyle
}

// hint renders one "key: label" help entry — key accented, ": label" dim.
func hint(k, label string) string {
	return helpKeyStyle.Render(k) + helpTextStyle.Render(": "+label)
}

// titledBox renders a rounded-border box of exactly width x height cells with
// title embedded in the top border and body clipped/padded to fit. Border and
// title use the accent colour when focused, dim otherwise. Adapted from the
// todo and dibs TUIs.
func titledBox(title, body string, width, height int, focused bool) string {
	ts := titleDimStyle
	if focused {
		ts = titleActiveStyle
	}
	return frame(ts.Render(" "+title+" "), body, width, height, focused)
}

// frame draws the rounded border of titledBox around body, with label — already
// styled, so it can be a single title or a whole tab strip — embedded in the
// top border and truncated to fit.
func frame(label, body string, width, height int, focused bool) string {
	width, height = max(width, 4), max(height, 3)
	iw, ih := width-2, height-2
	border := borderStyle(focused)

	label = ansi.Truncate(label, max(iw-3, 0), "")
	fill := max(iw-1-lipgloss.Width(label), 0)
	rows := make([]string, 0, height)
	rows = append(rows, border.Render("╭─")+label+border.Render(strings.Repeat("─", fill)+"╮"))

	lines := strings.Split(body, "\n")
	for i := range ih {
		content := ""
		if i < len(lines) {
			content = ansi.Truncate(lines[i], iw, "")
		}
		pad := strings.Repeat(" ", max(iw-lipgloss.Width(content), 0))
		rows = append(rows, border.Render("│")+content+pad+border.Render("│"))
	}

	rows = append(rows, border.Render("╰"+strings.Repeat("─", iw)+"╯"))
	return strings.Join(rows, "\n")
}
