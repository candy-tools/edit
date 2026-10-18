package tui

import (
	"testing/fstest"

	tea "github.com/charmbracelet/bubbletea"
)

// testFS is the workspace the model tests open: a folder and two files.
var testFS = fstest.MapFS{
	"app/main.go": {Data: []byte("package main\n\nfunc main() {}\n")},
	"go.mod":      {Data: []byte("module example.com/project\n\ngo 1.26\n")},
	"README.md":   {Data: []byte("# project\n")},
}

// newTestModel returns a fresh model on a fixed workspace, sized to a 100x30
// terminal.
func newTestModel() model {
	return send(newModel("/work/project", testFS), tea.WindowSizeMsg{Width: 100, Height: 30})
}

// send applies msg and returns the updated model.
func send(m model, msg tea.Msg) model {
	nm, _ := m.Update(msg)
	return nm.(model)
}

// press sends keystrokes by name ("tab", "ctrl+q", "up", …); anything else is
// typed as runes.
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

func keyMsg(s string) tea.KeyMsg {
	if k, ok := namedKeys[s]; ok {
		return tea.KeyMsg{Type: k}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
