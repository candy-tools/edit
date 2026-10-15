package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// boxLines splits a rendered box into its rows and checks every row is exactly
// width cells wide.
func boxLines(t *testing.T, box string, width int) []string {
	t.Helper()
	lines := strings.Split(ansi.Strip(box), "\n")
	for i, l := range lines {
		if w := lipgloss.Width(l); w != width {
			t.Errorf("row %d is %d cells wide, want %d: %q", i, w, width, l)
		}
	}
	return lines
}

func TestTitledBox(t *testing.T) {
	lines := boxLines(t, titledBox("Files", "a\nb", 20, 5, true), 20)
	want := []string{
		"╭─ Files " + strings.Repeat("─", 10) + "╮",
		"│a" + strings.Repeat(" ", 17) + "│",
		"│b" + strings.Repeat(" ", 17) + "│",
		"│" + strings.Repeat(" ", 18) + "│",
		"╰" + strings.Repeat("─", 18) + "╯",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("box =\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

func TestFrameClipsOversizedContent(t *testing.T) {
	body := strings.Repeat("x", 30) + "\n2\n3\n4"
	lines := boxLines(t, titledBox(strings.Repeat("t", 30), body, 12, 4, false), 12)
	if len(lines) != 4 {
		t.Fatalf("box is %d rows, want 4", len(lines))
	}
	if want := "│" + strings.Repeat("x", 10) + "│"; lines[1] != want {
		t.Errorf("an overlong line should be clipped to the box: %q, want %q", lines[1], want)
	}
	if !strings.HasPrefix(lines[2], "│2") {
		t.Errorf("rows past the box height should be dropped, got %q", lines[2])
	}
}

func TestFrameMinimumSize(t *testing.T) {
	lines := boxLines(t, titledBox("Files", "", 0, 0, false), 4)
	if len(lines) != 3 {
		t.Errorf("a too-small box should clamp to 3 rows, got %d", len(lines))
	}
}
