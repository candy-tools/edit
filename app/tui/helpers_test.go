package tui

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/candy-tools/edit/libs/keymap"
	tea "github.com/charmbracelet/bubbletea"
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
// keys, sized to a 100x30 terminal.
func newTestModel() model {
	return send(newModel("/work/project", testFS, testKeys), tea.WindowSizeMsg{Width: 100, Height: 30})
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
	return send(newModel("/work/project", testFS, rebound(t, o)), tea.WindowSizeMsg{Width: 100, Height: 30})
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
	"ctrl+c":      tea.KeyCtrlC,
	"ctrl+q":      tea.KeyCtrlQ,
	"ctrl+x":      tea.KeyCtrlX,
	"up":          tea.KeyUp,
	"down":        tea.KeyDown,
	"left":        tea.KeyLeft,
	"right":       tea.KeyRight,
	"enter":       tea.KeyEnter,
	"pgup":        tea.KeyPgUp,
	"pgdown":      tea.KeyPgDown,
	"home":        tea.KeyHome,
	"end":         tea.KeyEnd,
	"ctrl+pgup":   tea.KeyCtrlPgUp,
	"ctrl+pgdown": tea.KeyCtrlPgDown,
	"ctrl+w":      tea.KeyCtrlW,
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
