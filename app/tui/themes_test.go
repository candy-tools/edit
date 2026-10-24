package tui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/candy-tools/trazo/libs/syntax"
	"github.com/charmbracelet/x/ansi"
)

// describe says how s draws, as palette writes it: its foreground color,
// then bold, italic and underline; "" for unstyled.
func describe(s lipgloss.Style) string {
	var parts []string
	switch c := s.GetForeground().(type) {
	case ansi.BasicColor:
		parts = append(parts, strconv.Itoa(int(c)))
	case ansi.IndexedColor:
		parts = append(parts, strconv.Itoa(int(c)))
	case color.RGBA:
		parts = append(parts, fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B))
	}
	for _, a := range []struct {
		on   bool
		name string
	}{{s.GetBold(), "bold"}, {s.GetItalic(), "italic"}, {s.GetUnderline(), "underline"}} {
		if a.on {
			parts = append(parts, a.name)
		}
	}
	return strings.Join(parts, " ")
}

// The spec's table: in each theme, every capture of the vocabulary is drawn
// as the table says, or as the nearest parent it lists, or unstyled.
func TestThemesDrawEachCaptureAsTheTableSays(t *testing.T) {
	want := map[string]map[string]string{
		"candy": {
			"comment": "244 italic", "markup.quote": "244 italic", "keyword": "205", "markup.list": "205",
			"string": "114", "character": "114", "markup.raw": "114", "string.escape": "213", "string.special": "213",
			"number": "215", "boolean": "215", "constant": "215", "type": "81", "type.builtin": "81", "constructor": "81",
			"function": "141", "function.method": "141", "function.builtin": "141 italic",
			"variable.builtin": "215 italic", "constant.builtin": "215 italic", "property": "153",
			"attribute": "180", "label": "180", "markup.heading": "205 bold", "markup.strong": "bold",
			"markup.italic": "italic", "markup.link.label": "141", "markup.link.url": "81 underline",
		},
		"candy-light": {
			"comment": "244 italic", "markup.quote": "244 italic", "keyword": "162", "markup.list": "162",
			"string": "28", "character": "28", "markup.raw": "28", "string.escape": "127", "string.special": "127",
			"number": "130", "boolean": "130", "constant": "130", "type": "31", "type.builtin": "31", "constructor": "31",
			"function": "91", "function.method": "91", "function.builtin": "91 italic",
			"variable.builtin": "130 italic", "constant.builtin": "130 italic", "property": "25",
			"attribute": "94", "label": "94", "markup.heading": "162 bold", "markup.strong": "bold",
			"markup.italic": "italic", "markup.link.label": "91", "markup.link.url": "31 underline",
		},
		"dracula": {
			"comment": "#6272a4 italic", "markup.quote": "#6272a4 italic", "keyword": "#ff79c6", "markup.list": "#ff79c6",
			"string": "#f1fa8c", "character": "#f1fa8c", "markup.raw": "#f1fa8c", "string.escape": "#ff79c6",
			"string.special": "#ff79c6", "number": "#bd93f9", "boolean": "#bd93f9", "constant": "#bd93f9",
			"type": "#8be9fd italic", "type.builtin": "#8be9fd italic", "constructor": "#8be9fd italic",
			"function": "#50fa7b", "function.method": "#50fa7b", "function.builtin": "#8be9fd",
			"variable.builtin": "#bd93f9 italic", "constant.builtin": "#bd93f9 italic", "variable.parameter": "#ffb86c italic",
			"attribute": "#50fa7b italic", "label": "#50fa7b italic", "operator": "#ff79c6",
			"markup.heading": "#bd93f9 bold", "markup.strong": "#ffb86c bold", "markup.italic": "#f1fa8c italic",
			"markup.link.label": "#8be9fd", "markup.link.url": "#8be9fd underline",
		},
		"gruvbox": {
			"comment": "#928374 italic", "markup.quote": "#928374 italic", "keyword": "#fb4934", "markup.list": "#fb4934",
			"string": "#b8bb26", "character": "#b8bb26", "markup.raw": "#b8bb26", "string.escape": "#fe8019",
			"string.special": "#fe8019", "number": "#d3869b", "boolean": "#d3869b", "constant": "#d3869b",
			"type": "#fabd2f", "type.builtin": "#fabd2f", "constructor": "#fabd2f",
			"function": "#b8bb26 bold", "function.method": "#b8bb26 bold", "function.builtin": "#fe8019",
			"variable.builtin": "#fe8019", "constant.builtin": "#fe8019", "property": "#83a598",
			"attribute": "#8ec07c", "label": "#8ec07c", "markup.heading": "#fabd2f bold", "markup.strong": "bold",
			"markup.italic": "italic", "markup.link.label": "#83a598", "markup.link.url": "#83a598 underline",
		},
		"none": {},
	}
	for name, styled := range want {
		theme, err := NewTheme(name)
		if err != nil {
			t.Fatalf("NewTheme(%q): %v", name, err)
		}
		for _, c := range syntax.Captures() {
			if got := describe(theme.styles[theme.class(c)]); got != styled[c] {
				t.Errorf("%s draws %s as %q, want %q", name, c, got, styled[c])
			}
		}
	}
}

func TestNewTheme(t *testing.T) {
	if theme, err := NewTheme(""); err != nil || theme.Name() != "candy" || !theme.colors() {
		t.Errorf(`NewTheme("") = %q, %v; want candy`, theme.Name(), err)
	}
	if theme, err := NewTheme("none"); err != nil || theme.colors() {
		t.Errorf("none: %v; it colors nothing", err)
	}
	want := `unknown theme "monokai": want candy, candy-light, dracula, gruvbox or none`
	if _, err := NewTheme("monokai"); err == nil || err.Error() != want {
		t.Errorf("NewTheme(monokai) = %v, want %q", err, want)
	}
}

func TestThemeDrawsARunInItsClassStyle(t *testing.T) {
	theme, _ := NewTheme("candy")
	if got := theme.draw(theme.class("keyword"), "func"); !strings.Contains(got, "\x1b[38;5;205m") || !strings.Contains(got, "func") {
		t.Errorf("a keyword in candy: %q, want it in color 205", got)
	}
	if got := theme.draw(0, "x"); got != "x" {
		t.Errorf("plain text: %q, want it unstyled", got)
	}
}
