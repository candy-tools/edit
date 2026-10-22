package tui

import (
	"slices"
	"testing"
)

// defaultKeys are edit's actions, as the config file names them, and their
// default keys.
var defaultKeys = map[string][]string{
	"global.quit":         {"ctrl+q", "ctrl+c"},
	"global.switch-focus": {"shift+tab"},
	"global.next-tab":     {"ctrl+pgdown"},
	"global.previous-tab": {"ctrl+pgup"},
	"global.close-tab":    {"ctrl+w"},
	"sidebar.up":          {"up"},
	"sidebar.down":        {"down"},
	"sidebar.page-up":     {"pgup"},
	"sidebar.page-down":   {"pgdown"},
	"sidebar.top":         {"home"},
	"sidebar.bottom":      {"end"},
	"sidebar.expand":      {"right"},
	"sidebar.collapse":    {"left"},
	"sidebar.open":        {"enter"},
	"editor.left":         {"left"},
	"editor.right":        {"right"},
	"editor.up":           {"up"},
	"editor.down":         {"down"},
	"editor.line-start":   {"home"},
	"editor.line-end":     {"end"},
	"editor.page-up":      {"pgup"},
	"editor.page-down":    {"pgdown"},
	"editor.top":          {"ctrl+home"},
	"editor.bottom":       {"ctrl+end"},
	"editor.newline":      {"enter"},
	"editor.insert-tab":   {"tab"},
	"editor.delete-left":  {"backspace", "ctrl+h"},
	"editor.delete-right": {"delete"},
	"editor.delete-line":  {"ctrl+k"},
}

func TestTheDefaultKeys(t *testing.T) {
	keys, err := NewKeymap(nil)
	if err != nil {
		t.Fatalf("the default keys should make a valid keymap: %v", err)
	}
	if len(actions) != len(defaultKeys) {
		t.Errorf("edit has %d actions, want %d", len(actions), len(defaultKeys))
	}
	for _, a := range actions {
		want, ok := defaultKeys[a.String()]
		if !ok {
			t.Errorf("%s is not one of edit's actions", a)
			continue
		}
		if got := keys.Keys(a.ID); !slices.Equal(got, want) {
			t.Errorf("%s: keys %q, want %q", a, got, want)
		}
	}
}

func TestNewKeymapRebindsActions(t *testing.T) {
	keys, err := NewKeymap(map[string]map[string][]string{"sidebar": {"down": {"j", "down"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := keys.Keys(sidebarDown); !slices.Equal(got, []string{"j", "down"}) {
		t.Errorf("sidebar.down: keys %q, want j, down", got)
	}
}

func TestQuitMustKeepAKey(t *testing.T) {
	_, err := NewKeymap(map[string]map[string][]string{"global": {"quit": {}}})
	if want := "global.quit: needs at least one key"; err == nil || err.Error() != want {
		t.Errorf("NewKeymap = %v, want %q", err, want)
	}
}

// Keys type text in the editor, so a letter can't be bound there, nor in
// the global scope, whose actions work in the editor too.
func TestLettersCantBeBoundWhereTheyType(t *testing.T) {
	for _, c := range []struct {
		o    map[string]map[string][]string
		want string
	}{
		{map[string]map[string][]string{"editor": {"down": {"j"}}}, `editor.down: key "j" types text`},
		{map[string]map[string][]string{"global": {"quit": {"q"}}}, `global.quit: key "q" types text`},
		{map[string]map[string][]string{"editor": {"delete-line": {"space"}}}, `editor.delete-line: key "space" types text`},
	} {
		if _, err := NewKeymap(c.o); err == nil || err.Error() != c.want {
			t.Errorf("NewKeymap(%v) = %v, want %q", c.o, err, c.want)
		}
	}
	if _, err := NewKeymap(map[string]map[string][]string{"sidebar": {"down": {"j"}}, "editor": {"down": {"alt+j"}}}); err != nil {
		t.Errorf("letters in the sidebar and alt+ keys in the editor are fine: %v", err)
	}
}
