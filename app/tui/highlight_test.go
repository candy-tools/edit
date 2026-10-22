package tui

import (
	"fmt"
	"io/fs"
	"math"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// candy is the theme the highlighting tests color in.
var candy = func() Theme {
	theme, err := NewTheme("candy")
	if err != nil {
		panic(err)
	}
	return theme
}()

// colored is text as candy draws a run of capture.
func colored(capture, text string) string {
	return candy.draw(candy.class(capture), text)
}

// newColorModel returns a model on fsys in candy, with lipgloss drawing 256
// colors until the test ends, instant ticks and a 100x30 terminal. Its
// tests keep the commands Update returns: a parse that is never run leaves
// its document's Highlighter waiting for it.
func newColorModel(t *testing.T, fsys fs.FS) model {
	t.Helper()
	colors(t)
	m := newModel("/w", fsys, noWrites, testKeys, candy)
	m.tick = instantTick
	return send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
}

// goFS is a workspace of one Go file.
var goFS = fstest.MapFS{"main.go": {Data: []byte("package main\n\nfunc main() {}\n")}}

func TestAFileIsColoredOnceItsParseLands(t *testing.T) {
	m, parse := update(newColorModel(t, goFS), keyMsg("enter"))
	if strings.Contains(m.View(), colored("keyword", "func")) {
		t.Error("main.go is colored before its parse lands")
	}
	m, _ = drain(m, parse)
	if !strings.Contains(m.View(), colored("keyword", "func")) {
		t.Errorf("once its parse lands, func should be drawn as a keyword:\n%q", m.View())
	}
}

// Review focus: an edit shows at once, the colors after it moving along
// with their characters, and the next parse colors what was typed.
func TestColorsFollowAnEditThenItsParse(t *testing.T) {
	m, parse := update(newColorModel(t, goFS), keyMsg("enter"))
	m, _ = drain(m, parse)
	m = press(m, "down", "down") // the start of func main() {}
	m, parse = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("// "), Paste: true})
	m = press(m, "up") // the cursor off the line
	if !strings.Contains(m.View(), "// "+colored("keyword", "func")) {
		t.Errorf("before the parse lands, func should keep its color, three cells on:\n%q", m.View())
	}
	m, _ = drain(m, parse)
	if !strings.Contains(m.View(), colored("comment", "// func main() {}")) {
		t.Errorf("once the parse lands, the line should be a comment:\n%q", m.View())
	}
}

func TestTheCursorIsDrawnWithoutColors(t *testing.T) {
	m, parse := update(newColorModel(t, goFS), keyMsg("enter"))
	m, _ = drain(m, parse)
	m = press(m, "down", "down") // on the f of func
	if want := cursorStyle.Styled("f") + colored("keyword", "unc"); !strings.Contains(m.View(), want) {
		t.Errorf("the cursor should be reverse video without the keyword's color, then the keyword's color:\n%q", m.View())
	}
}

func TestAClosedTabsParseFreesItsTree(t *testing.T) {
	m, parse := update(newColorModel(t, goFS), keyMsg("enter"))
	doc := m.activeDocument()
	m = press(m, "ctrl+w")
	drain(m, parse)
	if spans := doc.syntax.Spans(doc.source, 0, 3, 0, math.MaxInt); spans[2] != nil {
		t.Errorf("a closed tab keeps the tree of its parse: %v", spans)
	}
}

func TestLargeFilesAndLongLinesStayPlain(t *testing.T) {
	big := "package main\n\nfunc main() {}\n" + strings.Repeat("// more\n", 40000) // over 256 KB
	long := "package main\n\nvar s = \"" + strings.Repeat("x", 25000) + "\"\n\nfunc main() {}\n"
	m := newColorModel(t, fstest.MapFS{"big.go": {Data: []byte(big)}, "long.go": {Data: []byte(long)}})
	m, parse := update(m, keyMsg("enter")) // big.go
	m, _ = drain(m, parse)
	if strings.Contains(m.View(), colored("keyword", "func")) {
		t.Error("a file over 256 KB should be drawn without colors")
	}
	m, parse = update(press(m, "shift+tab", "down"), keyMsg("enter")) // long.go
	m, _ = drain(m, parse)
	if view := m.View(); !strings.Contains(view, colored("keyword", "func")) || strings.Contains(view, colored("keyword", "var")) {
		t.Errorf("long.go: func should be colored, and the line over 20,000 bytes plain:\n%q", view)
	}
}

// Review focus: in a real program, parses run on goroutines of their own
// while keys keep coming; typing into a colored file loses nothing (run
// with -race).
func TestTypingWhileParsesRun(t *testing.T) {
	colors(t)
	d := newDisk()
	m := send(newModel("/w", goFS, d.write, testKeys, candy), tea.WindowSizeMsg{Width: 100, Height: 30})
	if err := run(m, make(chan os.Signal), runOpts(strings.NewReader("\rx\ry\rz\x11"))...); err != nil { // enter opens main.go, then x, y, z on lines of their own, then ctrl+q
		t.Fatal(err)
	}
	if got, want := d.log(), []string{"main.go=x\ny\nzpackage main\n\nfunc main() {}\n"}; !slices.Equal(got, want) {
		t.Errorf("writes %q, want %q", got, want)
	}
}

// Review fix: a JSON-lines file holds records of a few KB, with a capture
// every two or three bytes, and every keystroke draws a frame. The spans of
// a line must cost what the window shows of it, not its whole length.
func TestAFrameOfLongLinesIsFast(t *testing.T) {
	var record strings.Builder
	last := 0
	for record.WriteString("{"); record.Len() < 19000; last++ {
		if last > 0 {
			record.WriteString(",")
		}
		fmt.Fprintf(&record, `"k%d":{"a":%d,"b":"v","c":true}`, last, last)
	}
	record.WriteString("}")
	text := strings.Repeat(record.String()+"\n", 10)
	m, parse := update(newColorModel(t, fstest.MapFS{"a.json": {Data: []byte(text)}}), keyMsg("enter"))
	m, _ = drain(m, parse)
	start := time.Now()
	for range 10 {
		m.View()
	}
	m = press(m, "end") // the window at the lines' ends
	for range 10 {
		m.View()
	}
	if d := time.Since(start) / 20; d > 40*time.Millisecond {
		t.Errorf("a frame of 10 lines of 19 KB took %v: it must cost what the window shows of them", d)
	}
	if key := fmt.Sprintf(`"k%d"`, last-1); !strings.Contains(m.View(), colored("property", key)) {
		t.Errorf("at the lines' ends, %s should be colored as a property:\n%q", key, m.View())
	}
}

func TestTheThemeNoneParsesNothing(t *testing.T) {
	m := send(newModel("/w", goFS, noWrites, testKeys, noColors), tea.WindowSizeMsg{Width: 100, Height: 30})
	if m, cmd := update(m, keyMsg("enter")); cmd != nil || m.activeDocument().syntax != nil {
		t.Error("with the theme none, opening a file starts no parse")
	}
}

// Review focus: colored text holding wide characters, tabs, escape codes
// and invalid bytes still gives lines exactly as wide as the screen.
func TestColoredFilesKeepTheLayout(t *testing.T) {
	goText := "package main\n\n// 日本 \x1b[2J \xff\nfunc main() {\n\ts := \"日本\\t語\"\n\t_ = s\n}\n"
	mdText := "# 日本 *é*\n\n```go\nfunc 日() {}\n```\n\n| a | **日本** |\n|---|---|\n"
	m := newColorModel(t, fstest.MapFS{"a.go": {Data: []byte(goText)}, "b.md": {Data: []byte(mdText)}})
	m, parse := update(m, keyMsg("enter"))
	m, _ = drain(m, parse)
	m, parse = update(press(m, "shift+tab", "down"), keyMsg("enter"))
	m, _ = drain(m, parse)
	if !strings.Contains(m.View(), colored("markup.heading", " 日本 ")) || !strings.Contains(m.View(), colored("keyword", "func")) {
		t.Fatalf("b.md should be colored, its Go block as Go:\n%q", m.View())
	}
	for _, w := range []int{100, 41, 12} {
		m = send(m, tea.WindowSizeMsg{Width: w, Height: 12})
		for _, key := range []string{"ctrl+pgup", "ctrl+pgdown"} {
			m = press(m, key)
			lines := strings.Split(m.View(), "\n")
			for i, l := range lines[1 : len(lines)-1] {
				if got := lipgloss.Width(l); got != w {
					t.Errorf("%s at width %d: body line %d is %d cells wide", activeKey(m), w, i+1, got)
				}
			}
		}
	}
}
