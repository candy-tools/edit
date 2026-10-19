package keymap

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// overrides is the shape of New's overrides: scope → action name → keys.
type overrides = map[string]map[string][]string

// The actions of a small program: global ones, and two panes, a list and a
// view, that share some keys.
var (
	quit     = ID{Global, "quit"}
	next     = ID{Global, "next"}
	prev     = ID{Global, "prev"}
	listUp   = ID{"list", "up"}
	listDown = ID{"list", "down"}
	listOpen = ID{"list", "open"}
	viewUp   = ID{"view", "up"}
	viewDown = ID{"view", "down"}
)

// testActions returns the small program's actions with their default keys.
func testActions() []Action {
	return []Action{
		{ID: quit, Keys: []string{"ctrl+q", "ctrl+c"}, Required: true},
		{ID: next, Keys: []string{"ctrl+pgdown"}},
		{ID: prev, Keys: []string{"ctrl+pgup"}},
		{ID: listUp, Keys: []string{"up"}},
		{ID: listDown, Keys: []string{"down"}},
		{ID: listOpen, Keys: []string{"enter"}},
		{ID: viewUp, Keys: []string{"up", "k"}},
		{ID: viewDown, Keys: []string{"down", "j"}},
	}
}

// build builds the small program's map with o, failing the test on an error.
func build(t *testing.T, o overrides) *Map {
	t.Helper()
	m, err := New(testActions(), o)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m
}

// problems returns the problems New reports for the small program with o,
// one per line, or "" when there are none.
func problems(o overrides) string {
	if _, err := New(testActions(), o); err != nil {
		return err.Error()
	}
	return ""
}

// press is the message Bubble Tea sends for a key of type t.
func press(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

// typed is the message Bubble Tea sends for typing s.
func typed(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestIDString(t *testing.T) {
	if got, want := listDown.String(), "list.down"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestNewBindsTheDefaultKeys(t *testing.T) {
	m := build(t, nil)
	for _, a := range testActions() {
		if got := m.Keys(a.ID); !slices.Equal(got, a.Keys) {
			t.Errorf("%s: keys %q, want the defaults %q", a.ID, got, a.Keys)
		}
	}
}

func TestAnOverrideReplacesAllOfAnActionsKeys(t *testing.T) {
	m := build(t, overrides{"view": {"down": {"j"}}})
	if got := m.Keys(viewDown); !slices.Equal(got, []string{"j"}) {
		t.Errorf("view.down: keys %q, want just j", got)
	}
	if got := m.Keys(viewUp); !slices.Equal(got, []string{"up", "k"}) {
		t.Errorf("view.up: keys %q, want its defaults", got)
	}
}

func TestAnEmptyListUnbindsAnAction(t *testing.T) {
	m := build(t, overrides{"view": {"down": {}}, "list": {"down": nil}})
	if got := m.Keys(viewDown); got != nil {
		t.Errorf("view.down: keys %q, want nil", got)
	}
	if got := m.Keys(listDown); got != nil {
		t.Errorf("list.down: keys %q, want nil", got)
	}
	if _, ok := m.Lookup("view", press(tea.KeyDown)); ok {
		t.Error("down should trigger nothing once view.down is unbound")
	}
}

func TestAKeyListedTwiceIsKeptOnce(t *testing.T) {
	m := build(t, overrides{"view": {"down": {"j", "down", "j", " ", "space"}}})
	if got, want := m.Keys(viewDown), []string{"j", "down", "space"}; !slices.Equal(got, want) {
		t.Errorf("view.down: keys %q, want %q", got, want)
	}
}

func TestKeyNames(t *testing.T) {
	for _, k := range []string{
		"ctrl+w", "pgdown", "ctrl+shift+left", "shift+tab", "f20", "backspace", "ctrl+@", "esc",
		"j", "J", "?", "ñ", "+", "space",
		"alt+x", "alt+J", "alt+right", "alt++", "alt+space", "alt+enter",
	} {
		if p := problems(overrides{"list": {"open": {k}}}); p != "" {
			t.Errorf("%q should be a key: %s", k, p)
		}
	}
	for _, k := range []string{
		"ctrl+shift+w", "Ctrl+W", "shift+j", "ctrl+i", "ctrl+m", "ctrl+[", "jk", "",
		"\x01", "\x7f", "\xff", "runes", "alt+", "alt+alt+x", "alt+jk",
	} {
		if p, want := problems(overrides{"list": {"open": {k}}}), fmt.Sprintf("list.open: unknown key %q", k); p != want {
			t.Errorf("%q: problems %q, want %q", k, p, want)
		}
	}
}

func TestTheSpaceBarIsSpelledSpace(t *testing.T) {
	m := build(t, overrides{"list": {"open": {" "}, "down": {"alt+ "}}})
	if got := m.Keys(listOpen); !slices.Equal(got, []string{"space"}) {
		t.Errorf("list.open: keys %q, want space", got)
	}
	if got := m.Keys(listDown); !slices.Equal(got, []string{"alt+space"}) {
		t.Errorf("list.down: keys %q, want alt+space", got)
	}
}

func TestNewReportsUnknownScopesAndActions(t *testing.T) {
	got := problems(overrides{"lst": {"up": {"k"}}, "list": {"donw": {"j"}}, Global: {"quti": {"x"}}})
	want := "global.quti: unknown action\nlist.donw: unknown action\nlst: unknown scope"
	if got != want {
		t.Errorf("problems =\n%s\nwant\n%s", got, want)
	}
}

func TestARequiredActionNeedsAKey(t *testing.T) {
	if got, want := problems(overrides{Global: {"quit": {}}}), "global.quit: needs at least one key"; got != want {
		t.Errorf("quit: []: problems %q, want %q", got, want)
	}
	// Given only a key that doesn't exist, the key is the problem.
	if got, want := problems(overrides{Global: {"quit": {"Ctrl+Q"}}}), `global.quit: unknown key "Ctrl+Q"`; got != want {
		t.Errorf("quit: [Ctrl+Q]: problems %q, want %q", got, want)
	}
	// An action that isn't required can lose all its keys.
	if got := problems(overrides{Global: {"next": {}}}); got != "" {
		t.Errorf("next: []: problems %q, want none", got)
	}
}

func TestNewReportsKeysBoundTwice(t *testing.T) {
	for _, c := range []struct {
		name string
		o    overrides
		want string
	}{
		{"in one scope", overrides{"list": {"up": {"down"}}},
			`key "down" is bound to both list.down and list.up`},
		{"a global key in a pane", overrides{"list": {"down": {"ctrl+q"}}},
			`key "ctrl+q" is bound to both global.quit and list.down`},
		{"a global key in both panes", overrides{"list": {"open": {"ctrl+c"}}, "view": {"up": {"ctrl+c"}}},
			"key \"ctrl+c\" is bound to both global.quit and list.open\nkey \"ctrl+c\" is bound to both global.quit and view.up"},
		{"two global actions, reported once", overrides{Global: {"next": {"ctrl+q"}}},
			`key "ctrl+q" is bound to both global.next and global.quit`},
		{"three actions in one scope", overrides{"list": {"up": {"x"}, "down": {"x"}, "open": {"x"}}},
			"key \"x\" is bound to both list.down and list.open\nkey \"x\" is bound to both list.down and list.up"},
		{"an override against another action's default", overrides{Global: {"next": {"enter"}}},
			`key "enter" is bound to both global.next and list.open`},
	} {
		if got := problems(c.o); got != c.want {
			t.Errorf("%s: problems =\n%s\nwant\n%s", c.name, got, c.want)
		}
	}
}

func TestTheSameKeyInTwoPanesIsFine(t *testing.T) {
	m := build(t, overrides{"list": {"down": {"j"}}}) // view.down has j too
	if id, _ := m.Lookup("list", typed("j")); id != listDown {
		t.Errorf("j in the list = %v, want list.down", id)
	}
	if id, _ := m.Lookup("view", typed("j")); id != viewDown {
		t.Errorf("j in the view = %v, want view.down", id)
	}
}

func TestRebindingBothSidesOfAClashIsFine(t *testing.T) {
	if got := problems(overrides{Global: {"next": {"enter"}}, "list": {"open": {"l"}}}); got != "" {
		t.Errorf("problems %q, want none", got)
	}
}

func TestNewReportsEveryProblemSorted(t *testing.T) {
	got := problems(overrides{
		Global: {"quit": {}},
		"list": {"up": {"j"}, "down": {"j"}, "donw": {"x"}},
		"lst":  {"up": {"k"}},
		"view": {"up": {"ctrl+uo"}},
	})
	want := strings.Join([]string{
		`global.quit: needs at least one key`,
		`key "j" is bound to both list.down and list.up`,
		`list.donw: unknown action`,
		`lst: unknown scope`,
		`view.up: unknown key "ctrl+uo"`,
	}, "\n")
	if got != want {
		t.Errorf("problems =\n%s\nwant\n%s", got, want)
	}
}

func TestNewPanicsOnABadActionTable(t *testing.T) {
	for name, actions := range map[string][]Action{
		"an empty scope": {{ID: ID{"", "up"}}},
		"an empty name":  {{ID: ID{"list", ""}}},
		"a repeated ID":  {{ID: listUp, Keys: []string{"up"}}, {ID: listUp, Keys: []string{"k"}}},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: New should panic", name)
				}
			}()
			_, _ = New(actions, nil)
		}()
	}
}

func TestLookup(t *testing.T) {
	m := build(t, nil)
	for _, c := range []struct {
		name  string
		scope string
		msg   tea.KeyMsg
		want  ID // the zero ID for none
	}{
		{"a global key in the list", "list", press(tea.KeyCtrlQ), quit},
		{"a global key in the view", "view", press(tea.KeyCtrlC), quit},
		{"a global key in the global scope", Global, press(tea.KeyCtrlPgDown), next},
		{"the list's key", "list", press(tea.KeyDown), listDown},
		{"the same key in the view", "view", press(tea.KeyDown), viewDown},
		{"a letter", "view", typed("j"), viewDown},
		{"another pane's key", "list", typed("j"), ID{}},
		{"a pane's key in the global scope", Global, press(tea.KeyDown), ID{}},
		{"a key bound to nothing", "list", press(tea.KeyF5), ID{}},
		{"several letters at once", "view", typed("jj"), ID{}},
		{"an unknown scope", "nope", press(tea.KeyCtrlQ), ID{}},
		{"pasted text", "view", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j"), Paste: true}, ID{}},
	} {
		id, ok := m.Lookup(c.scope, c.msg)
		if id != c.want || ok != (c.want != ID{}) {
			t.Errorf("%s: Lookup = %v, %t; want %v", c.name, id, ok, c.want)
		}
	}
}

func TestLookupSpaceAndAlt(t *testing.T) {
	m := build(t, overrides{"list": {"open": {"space"}, "up": {"alt+up", "alt+k"}, "down": {"alt+space"}}})
	for _, c := range []struct {
		name string
		msg  tea.KeyMsg
		want ID // the zero ID for none
	}{
		{"the space bar", press(tea.KeySpace), listOpen},
		{"a typed space", typed(" "), listOpen},
		{"alt and an arrow", tea.KeyMsg{Type: tea.KeyUp, Alt: true}, listUp},
		{"alt and a letter", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k"), Alt: true}, listUp},
		{"alt and the space bar", tea.KeyMsg{Type: tea.KeySpace, Alt: true}, listDown},
		{"the letter without alt", typed("k"), ID{}},
		{"the arrow without alt", press(tea.KeyUp), ID{}},
	} {
		id, ok := m.Lookup("list", c.msg)
		if id != c.want || ok != (c.want != ID{}) {
			t.Errorf("%s: Lookup = %v, %t; want %v", c.name, id, ok, c.want)
		}
	}
}

// Review focus: a key held down reaches Bubble Tea faster than it reads, so
// its repeats arrive as one message; each must count as a press.
func TestPresses(t *testing.T) {
	paste := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jj"), Paste: true}
	for _, c := range []struct {
		name string
		msg  tea.KeyMsg
		want []tea.KeyMsg
	}{
		{"typed runes", typed("jjk"), []tea.KeyMsg{typed("j"), typed("j"), typed("k")}},
		{"one typed rune", typed("j"), []tea.KeyMsg{typed("j")}},
		{"a named key", press(tea.KeyDown), []tea.KeyMsg{press(tea.KeyDown)}},
		{"pasted text", paste, []tea.KeyMsg{paste}},
	} {
		if got := Presses(c.msg); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: Presses = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestKeysReturnsANewSlice(t *testing.T) {
	m := build(t, nil)
	keys := m.Keys(viewDown)
	keys[0] = "x"
	if got := m.Keys(viewDown); !slices.Equal(got, []string{"down", "j"}) {
		t.Errorf("view.down: keys %q after changing a returned slice, want down, j", got)
	}
	if got := m.Keys(ID{"nope", "down"}); got != nil {
		t.Errorf("an unknown action's keys = %q, want nil", got)
	}
}

func TestLabel(t *testing.T) {
	for _, c := range []struct {
		name string
		o    overrides
		ids  []ID
		want string
	}{
		{"bare arrows run together", nil, []ID{listUp, listDown}, "↑↓"},
		{"the first key of each action", nil, []ID{viewUp, viewDown}, "↑↓"},
		{"a shared modifier is written once", nil, []ID{prev, next}, "ctrl+pgup/pgdown"},
		{"a shared modifier on arrows", overrides{Global: {"prev": {"alt+left"}, "next": {"alt+right"}}},
			[]ID{prev, next}, "alt+←/→"},
		{"two shared modifiers", overrides{"list": {"up": {"ctrl+shift+up"}, "down": {"ctrl+shift+down"}}},
			[]ID{listUp, listDown}, "ctrl+shift+↑/↓"},
		{"letters are joined with /", overrides{"view": {"up": {"k"}, "down": {"j"}}},
			[]ID{viewUp, viewDown}, "k/j"},
		{"an arrow and a letter", overrides{"view": {"down": {"j"}}}, []ID{viewUp, viewDown}, "↑/j"},
		{"different modifiers are written in full", overrides{Global: {"next": {"pgdown"}}},
			[]ID{prev, next}, "ctrl+pgup, pgdown"},
		{"a modifier on one key only", overrides{"list": {"up": {"shift+tab"}, "down": {"tab"}}},
			[]ID{listUp, listDown}, "shift+tab, tab"},
		{"+ after a modifier", overrides{"list": {"up": {"alt++"}, "down": {"alt+-"}}},
			[]ID{listUp, listDown}, "alt++/-"},
		{"one key", nil, []ID{quit}, "ctrl+q"},
		{"an unbound action is skipped", overrides{"view": {"up": {}}}, []ID{viewUp, viewDown}, "↓"},
		{"an unknown action is skipped", nil, []ID{{"nope", "x"}, listOpen}, "enter"},
		{"no action has keys", overrides{"view": {"up": {}, "down": {}}}, []ID{viewUp, viewDown}, ""},
		{"no actions", nil, nil, ""},
	} {
		if got := build(t, c.o).Label(c.ids...); got != c.want {
			t.Errorf("%s: Label = %q, want %q", c.name, got, c.want)
		}
	}
}
