package tui

import (
	"errors"
	"io/fs"
	"strings"
	"unicode"

	"github.com/candy-tools/edit/libs/keymap"
	"github.com/candy-tools/edit/libs/treeview"
)

// sidebarBody draws the file tree's scroll window for the sidebar box.
func (m model) sidebarBody() string {
	rows, cursor := m.files.Window()
	return treeLines(rows, cursor, m.focused(paneSidebar))
}

// sidebarAction applies an action of the sidebar to the file tree: up, down,
// page-up, page-down, top and bottom move the cursor, expand and collapse
// open and close folders, and open, which reaches here on anything but a
// file, opens or closes a folder. Other actions do nothing.
func sidebarAction(files *treeview.Tree[entry], id keymap.ID) {
	switch id {
	case sidebarUp:
		files.Up()
	case sidebarDown:
		files.Down()
	case sidebarPageUp:
		files.PageUp()
	case sidebarPageDown:
		files.PageDown()
	case sidebarTop:
		files.Top()
	case sidebarBottom:
		files.Bottom()
	case sidebarExpand:
		files.Expand()
	case sidebarCollapse:
		files.Collapse()
	case sidebarOpen:
		files.Toggle()
	}
}

// treeLines draws the file tree's rows for the sidebar box, one per line.
// cursor is the cursor row's index among rows (-1 for none), and focused
// whether the sidebar has focus.
func treeLines(rows []treeview.Row[entry], cursor int, focused bool) string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = treeRow(r, i == cursor, focused)
	}
	return strings.Join(out, "\n")
}

// treeRow draws one row of the file tree: a 2-column gutter ("❯ " on the
// cursor row), two columns of indent per level, a dim 2-column arrow column
// (▸ closed folder, ▾ open folder, ⚠ error row) and the name — purple for a
// folder, or the error's cause, dim, on an error row. With the sidebar
// focused the cursor row is drawn wholly in the accent colour; unfocused it
// keeps only a dim "❯", so you don't lose your place.
func treeRow(r treeview.Row[entry], cursor, focused bool) string {
	arrow, text, textStyle := "  ", r.Node.name, plainStyle
	switch {
	case r.Err != nil:
		arrow, text, textStyle = "⚠ ", errText(r.Err), helpTextStyle
	case r.Branch && r.Expanded:
		arrow, textStyle = "▾ ", folderStyle
	case r.Branch:
		arrow, textStyle = "▸ ", folderStyle
	}
	text = displayName(text)
	indent := strings.Repeat("  ", r.Depth)
	if cursor && focused {
		return selectedRowStyle.Render("❯ " + indent + arrow + text)
	}
	gutter := "  "
	if cursor {
		gutter = helpTextStyle.Render("❯ ")
	}
	return gutter + indent + helpTextStyle.Render(arrow) + textStyle.Render(text)
}

// displayName makes text safe to draw in the tree: each run of invalid UTF-8
// bytes and each control character becomes "?", so a file name can't break
// the layout or send escape codes to the terminal.
func displayName(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, strings.ToValidUTF8(text, "?"))
}

// errText is what an error row shows: just the cause of a path error, such
// as "permission denied", since the full "open <path>: …" message would not
// fit the sidebar.
func errText(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}
