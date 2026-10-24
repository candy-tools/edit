package tui

import (
	"fmt"
	"strings"

	"github.com/candy-tools/trazo/libs/syntax"
	"charm.land/lipgloss/v2"
)

// themeNames are the built-in themes, in the order an error lists them.
var themeNames = []string{"candy", "candy-light", "dracula", "gruvbox", "none"}

// palette is the built-in themes' styles: for each row's captures, the style
// in candy, candy-light, dracula and gruvbox, as style reads it; "" is
// unstyled. A capture not listed takes the style of its nearest listed
// parent (function.method takes function's), or none.
var palette = []struct {
	captures []string
	styles   [4]string
}{
	{[]string{"comment", "markup.quote"}, [4]string{"244 italic", "244 italic", "#6272a4 italic", "#928374 italic"}},
	{[]string{"keyword", "markup.list"}, [4]string{"205", "162", "#ff79c6", "#fb4934"}},
	{[]string{"string", "character", "markup.raw"}, [4]string{"114", "28", "#f1fa8c", "#b8bb26"}},
	{[]string{"string.escape", "string.special"}, [4]string{"213", "127", "#ff79c6", "#fe8019"}},
	{[]string{"number", "boolean", "constant"}, [4]string{"215", "130", "#bd93f9", "#d3869b"}},
	{[]string{"type", "constructor"}, [4]string{"81", "31", "#8be9fd italic", "#fabd2f"}},
	{[]string{"function"}, [4]string{"141", "91", "#50fa7b", "#b8bb26 bold"}},
	{[]string{"function.builtin"}, [4]string{"141 italic", "91 italic", "#8be9fd", "#fe8019"}},
	{[]string{"variable.builtin", "constant.builtin"}, [4]string{"215 italic", "130 italic", "#bd93f9 italic", "#fe8019"}},
	{[]string{"variable.parameter"}, [4]string{"", "", "#ffb86c italic", ""}},
	{[]string{"property"}, [4]string{"153", "25", "", "#83a598"}},
	{[]string{"attribute", "label"}, [4]string{"180", "94", "#50fa7b italic", "#8ec07c"}},
	{[]string{"operator"}, [4]string{"", "", "#ff79c6", ""}},
	{[]string{"markup.heading"}, [4]string{"205 bold", "162 bold", "#bd93f9 bold", "#fabd2f bold"}},
	{[]string{"markup.strong"}, [4]string{"bold", "bold", "#ffb86c bold", "bold"}},
	{[]string{"markup.italic"}, [4]string{"italic", "italic", "#f1fa8c italic", "italic"}},
	{[]string{"markup.link.label"}, [4]string{"141", "91", "#8be9fd", "#83a598"}},
	{[]string{"markup.link.url"}, [4]string{"81 underline", "31 underline", "#8be9fd underline", "#83a598 underline"}},
}

// Theme is a built-in color theme: the style each capture of the syntax
// vocabulary is drawn in. A capture's class is the index of its style; class
// 0 is plain text, drawn unstyled.
type Theme struct {
	name    string
	styles  []lipgloss.Style // by class
	classes map[string]int   // by capture; a capture missing is plain
}

// NewTheme returns the built-in theme called name: "" means candy. Its error
// lists the themes there are.
func NewTheme(name string) (Theme, error) {
	if name == "" {
		name = "candy"
	}
	column := -1
	for i, n := range themeNames {
		if n == name {
			column = i
		}
	}
	if column < 0 {
		last := len(themeNames) - 1
		return Theme{}, fmt.Errorf("unknown theme %q: want %s or %s", name, strings.Join(themeNames[:last], ", "), themeNames[last])
	}
	table := map[string]string{} // capture → style; none, the last theme, has no column in palette
	for _, row := range palette {
		for _, c := range row.captures {
			if column < len(row.styles) && row.styles[column] != "" {
				table[c] = row.styles[column]
			}
		}
	}
	t := Theme{name: name, styles: []lipgloss.Style{plainStyle}, classes: map[string]int{}}
	for _, c := range syntax.Captures() {
		if spec, ok := lookup(table, c); ok {
			t.classes[c] = len(t.styles)
			t.styles = append(t.styles, style(spec))
		}
	}
	return t, nil
}

// lookup returns the style table gives capture c: its own or, failing that,
// its nearest parent's, found by dropping the last dotted part of its name
// until one is in table.
func lookup(table map[string]string, c string) (string, bool) {
	for {
		if spec, ok := table[c]; ok {
			return spec, true
		}
		i := strings.LastIndexByte(c, '.')
		if i < 0 {
			return "", false
		}
		c = c[:i]
	}
}

// style is the lipgloss style a palette entry describes: a foreground color,
// a 256-color number or a hex color, and any of bold, italic and underline,
// separated by spaces.
func style(spec string) lipgloss.Style {
	s := lipgloss.NewStyle()
	for _, f := range strings.Fields(spec) {
		switch f {
		case "bold":
			s = s.Bold(true)
		case "italic":
			s = s.Italic(true)
		case "underline":
			s = s.Underline(true)
		default:
			s = s.Foreground(lipgloss.Color(f))
		}
	}
	return s
}

// Name returns the theme's name.
func (t Theme) Name() string { return t.name }

// colors reports whether t colors text at all, as every theme but none does.
func (t Theme) colors() bool { return len(t.styles) > 1 }

// class returns the class of capture c.
func (t Theme) class(c string) int { return t.classes[c] }

// draw draws text, cells of class c, in the class's style.
func (t Theme) draw(c int, text string) string {
	if c <= 0 || c >= len(t.styles) {
		return text
	}
	return t.styles[c].Render(text)
}
