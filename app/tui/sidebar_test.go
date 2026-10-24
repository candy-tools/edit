package tui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/candy-tools/trazo/libs/treeview"
	"github.com/charmbracelet/x/ansi"
)

var (
	appDir = entry{name: "app", path: "app", dir: true}
	mainGo = entry{name: "main.go", path: "app/main.go"}
)

// Colours are not asserted — tests render without a colour profile, as in
// the other candy-tools apps — only the layout.
func TestTreeRowLayout(t *testing.T) {
	denied := &fs.PathError{Op: "open", Path: "/w/secret", Err: fs.ErrPermission}
	for _, c := range []struct {
		name            string
		row             treeview.Row[entry]
		cursor, focused bool
		want            string
	}{
		{"closed folder", treeview.Row[entry]{Node: appDir, Branch: true}, false, true, "  ▸ app"},
		{"open folder", treeview.Row[entry]{Node: appDir, Branch: true, Expanded: true}, false, true, "  ▾ app"},
		{"file one level down", treeview.Row[entry]{Node: mainGo, Depth: 1}, false, true, "      main.go"},
		{"cursor, sidebar focused", treeview.Row[entry]{Node: mainGo, Depth: 1}, true, true, "❯     main.go"},
		{"cursor, sidebar unfocused", treeview.Row[entry]{Node: mainGo, Depth: 1}, true, false, "❯     main.go"},
		{"path error", treeview.Row[entry]{Depth: 1, Err: denied}, false, true, "    ⚠ permission denied"},
		{"other error", treeview.Row[entry]{Err: errors.New("boom")}, false, true, "  ⚠ boom"},
		{"unsafe name", treeview.Row[entry]{Node: entry{name: "a\nb", path: "a\nb"}}, false, true, "    a?b"},
	} {
		if got := ansi.Strip(treeRow(c.row, c.cursor, c.focused)); got != c.want {
			t.Errorf("%s: row = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTreeLinesMarksOnlyTheCursorRow(t *testing.T) {
	rows := []treeview.Row[entry]{
		{Node: appDir, Branch: true, Expanded: true},
		{Node: mainGo, Depth: 1},
		{Node: entry{name: "go.mod", path: "go.mod"}},
	}
	if got, want := ansi.Strip(treeLines(rows, 1, true)), "  ▾ app\n❯     main.go\n    go.mod"; got != want {
		t.Errorf("tree =\n%s\nwant\n%s", got, want)
	}
	if got := treeLines(nil, -1, true); got != "" {
		t.Errorf("an empty tree should draw nothing, got %q", got)
	}
}

func TestDisplayName(t *testing.T) {
	for in, want := range map[string]string{
		"main.go":          "main.go",
		"two\nlines":       "two?lines",
		"\x1b[31mred":      "?[31mred",
		"tab\there":        "tab?here",
		"del\x7f":          "del?",
		"bad\xff\xfebytes": "bad?bytes",
		"héllo 日本":         "héllo 日本",
	} {
		if got := displayName(in); got != want {
			t.Errorf("displayName(%q) = %q, want %q", in, got, want)
		}
	}
}

// Review focus: a real unreadable folder shows "permission denied", not the
// full "open <path>: …" message.
func TestErrTextOnARealUnreadableFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read any folder")
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "secret"), 0o000); err != nil {
		t.Fatal(err)
	}
	_, err := workspace{os.DirFS(dir)}.Children(entry{name: "secret", path: "secret", dir: true})
	if err == nil {
		t.Fatal("listing an unreadable folder should fail")
	}
	if got := errText(err); got != "permission denied" {
		t.Errorf("errText = %q, want %q", got, "permission denied")
	}
}
