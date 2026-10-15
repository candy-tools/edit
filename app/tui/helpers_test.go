package tui

import tea "github.com/charmbracelet/bubbletea"

// newTestModel returns a fresh model on a fixed workspace, sized to a 100x30
// terminal.
func newTestModel() model {
	return send(newModel("/work/project"), tea.WindowSizeMsg{Width: 100, Height: 30})
}

// send applies msg and returns the updated model.
func send(m model, msg tea.Msg) model {
	nm, _ := m.Update(msg)
	return nm.(model)
}

// press sends keystrokes by name ("tab", "ctrl+q", …); anything else is typed
// as runes.
func press(m model, keys ...string) model {
	for _, k := range keys {
		m = send(m, keyMsg(k))
	}
	return m
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+q":
		return tea.KeyMsg{Type: tea.KeyCtrlQ}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}
