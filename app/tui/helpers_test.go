package tui

import (
	"strings"
	"testing"
	"testing/fstest"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/candy-tools/edit/libs/keymap"
)

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

// press sends keystrokes by name ("tab", "ctrl+q", "alt+right", "G", …);
// anything else is typed, one key press per character, as Bubble Tea sends
// typed text.
func press(m model, keys ...string) model {
	for _, k := range keys {
		if isKeyName(k) {
			m = send(m, keyMsg(k))
			continue
		}
		for _, r := range k {
			m = send(m, keyMsg(string(r)))
		}
	}
	return m
}

// isKeyName reports whether s names one key: one of namedKeys, a single
// character, or either after modifiers.
func isKeyName(s string) bool {
	for _, m := range keyModifiers {
		if rest, ok := strings.CutPrefix(s, m.prefix); ok && rest != "" {
			return isKeyName(rest)
		}
	}
	_, named := namedKeys[s]
	return named || utf8.RuneCountInString(s) == 1
}

// namedKeys are the keys keyMsg knows by name, by their Bubble Tea codes.
var namedKeys = map[string]rune{
	"tab": tea.KeyTab, "space": tea.KeySpace, "enter": tea.KeyEnter, "esc": tea.KeyEscape,
	"backspace": tea.KeyBackspace, "delete": tea.KeyDelete,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "home": tea.KeyHome, "end": tea.KeyEnd,
	"f6": tea.KeyF6,
}

// keyModifiers are the modifiers a key's name in a test can start with.
var keyModifiers = []struct {
	prefix string
	mod    tea.KeyMod
}{{"ctrl+", tea.ModCtrl}, {"alt+", tea.ModAlt}, {"shift+", tea.ModShift}}

// keyMsg is the message Bubble Tea sends for the key named s: modifiers in
// front ("ctrl+", "alt+", "shift+"), then one of namedKeys or a character.
// A character typed with no modifier carries its text, a capital as shift
// and the small letter, as Bubble Tea decodes them; so does the space bar.
func keyMsg(s string) tea.KeyPressMsg {
	var mod tea.KeyMod
	for cut := true; cut; {
		cut = false
		for _, m := range keyModifiers {
			if rest, ok := strings.CutPrefix(s, m.prefix); ok && rest != "" {
				s, mod, cut = rest, mod|m.mod, true
			}
		}
	}
	if code, ok := namedKeys[s]; ok {
		k := tea.KeyPressMsg{Code: code, Mod: mod}
		if code == tea.KeySpace && mod == 0 {
			k.Text = " "
		}
		return k
	}
	r, _ := utf8.DecodeRuneInString(s)
	k := tea.KeyPressMsg{Code: r, Mod: mod}
	if mod == 0 {
		k.Text = s
		if unicode.IsUpper(r) {
			k.Code, k.ShiftedCode, k.Mod = unicode.ToLower(r), r, tea.ModShift
		}
	}
	return k
}

// paste sends text as Bubble Tea sends pasted text: a message of its own.
func paste(m model, text string) model {
	return send(m, tea.PasteMsg{Content: text})
}

// cursor returns the cursor of m's active tab: its line and byte offset.
func cursor(m model) (line, offset int) {
	return m.activeDocument().text.Cursor()
}
