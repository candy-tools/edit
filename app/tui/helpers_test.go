package tui

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/candy-tools/edit/libs/keymap"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// colors makes lipgloss draw 256 colors until the test ends, as it does on
// most terminals: a test's output is no terminal, so it draws none there.
func colors(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
}

// testFS is the workspace the model tests open: a folder and two files.
var testFS = fstest.MapFS{
	"app/main.go": {Data: []byte("package main\n\nfunc main() {}\n")},
	"go.mod":      {Data: []byte("module example.com/project\n\ngo 1.26\n")},
	"README.md":   {Data: []byte("# project\n")},
}

// overrides is the shape of the config's shortcuts: scope → action name →
// keys.
type overrides = map[string]map[string][]string

// testKeys is edit's default keymap, which the model tests run on unless
// they rebind keys.
var testKeys = func() *keymap.Map {
	keys, err := NewKeymap(nil)
	if err != nil {
		panic(err)
	}
	return keys
}()

// newTestModel returns a fresh model on a fixed workspace, with the default
// keys, sized to a 100x30 terminal. It must not write: a test that saves
// edits uses newEditModel.
func newTestModel() model {
	return send(newModel("/work/project", testFS, noWrites, testKeys, noColors), tea.WindowSizeMsg{Width: 100, Height: 30})
}

// noColors is the theme none, for the model tests, where colors don't
// matter.
var noColors = func() Theme {
	theme, err := NewTheme("none")
	if err != nil {
		panic(err)
	}
	return theme
}()

// noWrites is the writer of a model that must not write: a write there is a
// bug, so it panics.
func noWrites(path string, _ []byte) error {
	panic("unexpected write of " + path)
}

// rebound is edit's keymap with the keys o rebinds.
func rebound(t *testing.T, o overrides) *keymap.Map {
	t.Helper()
	keys, err := NewKeymap(o)
	if err != nil {
		t.Fatalf("NewKeymap: %v", err)
	}
	return keys
}

// newReboundModel is newTestModel with the keys o rebinds.
func newReboundModel(t *testing.T, o overrides) model {
	t.Helper()
	return send(newModel("/work/project", testFS, noWrites, rebound(t, o), noColors), tea.WindowSizeMsg{Width: 100, Height: 30})
}

// send applies msg and returns the updated model.
func send(m model, msg tea.Msg) model {
	nm, _ := m.Update(msg)
	return nm.(model)
}

// press sends keystrokes by name ("tab", "ctrl+q", "alt+right", …); anything
// else is typed as runes.
func press(m model, keys ...string) model {
	for _, k := range keys {
		m = send(m, keyMsg(k))
	}
	return m
}

// namedKeys are the keys keyMsg knows by name.
var namedKeys = map[string]tea.KeyType{
	"tab":         tea.KeyTab,
	"shift+tab":   tea.KeyShiftTab,
	"space":       tea.KeySpace,
	"ctrl+c":      tea.KeyCtrlC,
	"ctrl+q":      tea.KeyCtrlQ,
	"ctrl+x":      tea.KeyCtrlX,
	"ctrl+h":      tea.KeyCtrlH,
	"ctrl+k":      tea.KeyCtrlK,
	"up":          tea.KeyUp,
	"down":        tea.KeyDown,
	"left":        tea.KeyLeft,
	"right":       tea.KeyRight,
	"enter":       tea.KeyEnter,
	"backspace":   tea.KeyBackspace,
	"delete":      tea.KeyDelete,
	"pgup":        tea.KeyPgUp,
	"pgdown":      tea.KeyPgDown,
	"home":        tea.KeyHome,
	"end":         tea.KeyEnd,
	"ctrl+home":   tea.KeyCtrlHome,
	"ctrl+end":    tea.KeyCtrlEnd,
	"ctrl+pgup":   tea.KeyCtrlPgUp,
	"ctrl+pgdown": tea.KeyCtrlPgDown,
	"ctrl+w":      tea.KeyCtrlW,
}

// paste sends text as Bubble Tea sends pasted text: one message, marked as
// a paste.
func paste(m model, text string) model {
	return send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
}

// cursor returns the cursor of m's active tab: its line and byte offset.
func cursor(m model) (line, offset int) {
	return m.activeDocument().text.Cursor()
}

// keyMsg is the message Bubble Tea sends for the key named s: one of
// namedKeys, or s typed as runes, with alt held down when s starts with
// "alt+".
func keyMsg(s string) tea.KeyMsg {
	if rest, ok := strings.CutPrefix(s, "alt+"); ok && rest != "" {
		msg := keyMsg(rest)
		msg.Alt = true
		return msg
	}
	if k, ok := namedKeys[s]; ok {
		return tea.KeyMsg{Type: k}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
