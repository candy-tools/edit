package keymap

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
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

// press is the message Bubble Tea sends for the key with code c, modifiers
// mod held.
func press(c rune, mod tea.KeyMod) tea.KeyPressMsg { return tea.KeyPressMsg{Code: c, Mod: mod} }

// typed is the message Bubble Tea sends for typing the small letter, digit
// or sign s.
func typed(s string) tea.KeyPressMsg {
	r, _ := utf8.DecodeRuneInString(s)
	return tea.KeyPressMsg{Code: r, Text: s}
}

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
	if _, ok := m.Lookup("view", press(tea.KeyDown, 0)); ok {
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
		"ctrl+w", "pgdown", "ctrl+shift+left", "shift+tab", "f20", "f63", "backspace", "ctrl+@", "esc",
		"j", "J", "?", "ñ", "+", "space", "ctrl+space", "shift+space",
		"alt+x", "alt+J", "alt+right", "alt++", "alt+space", "alt+enter",
		"ctrl+shift+w", "ctrl+i", "ctrl+m", "ctrl+[", "shift+enter", "super+f5", "meta+x", "hyper+x",
		"shift+ctrl+z", "ctrl+A", "alt+Ñ", "printscreen", "menu", "mediaplay", "begin",
		"alt+?", "ctrl+!", "alt+<",
	} {
		if p := problems(overrides{"list": {"open": {k}}}); p != "" {
			t.Errorf("%q should be a key: %s", k, p)
		}
	}
	for _, k := range []string{
		"Ctrl+W", "shift+j", "shift+J", "Enter", "F5", "jk", "",
		"\x01", "\x7f", "\xff", "runes", "alt+", "alt+alt+x", "ctrl+shift+ctrl+x", "alt+jk",
		// keys edit never receives: a modifier key alone, a lock key, keys
		// Bubble Tea never decodes
		"leftshift", "ctrl+rightalt", "isolevel3shift", "capslock", "numlock", "scrolllock", "find", "select",
		// shift before a sign: the sign shift types is the key
		"alt+shift+/", "ctrl+shift+1",
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

// Review focus: a key is kept as Bubble Tea names the key press, however the
// config spells it, so configs written for v1 keep working.
func TestKeysAreSpelledAsBubbleTeaNamesThem(t *testing.T) {
	for in, want := range map[string]string{
		"shift+ctrl+z": "ctrl+shift+z", // Bubble Tea's order
		"ctrl+A":       "ctrl+shift+a", // a capital is shift and the small letter
		"alt+J":        "alt+shift+j",  // v1's alt+J
		"super+alt+f5": "alt+super+f5",
		"alt+ ":        "alt+space",
		"ctrl+@":       "ctrl+space", // v1's name for what most terminals send for ctrl+space
		"alt+ctrl+@":   "ctrl+alt+space",
		"alt+Ñ":        "alt+shift+ñ",
		"J":            "J", // a capital alone is the character it types
	} {
		m := build(t, overrides{"list": {"open": {in}}})
		if got := m.Keys(listOpen); !slices.Equal(got, []string{want}) {
			t.Errorf("%q: keys %q, want %q", in, got, want)
		}
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
		{"two spellings of one key", overrides{"list": {"up": {"shift+ctrl+z"}, "down": {"ctrl+shift+z"}}},
			`key "ctrl+shift+z" is bound to both list.down and list.up`},
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
		msg   tea.KeyPressMsg
		want  ID // the zero ID for none
	}{
		{"a global key in the list", "list", press('q', tea.ModCtrl), quit},
		{"a global key in the view", "view", press('c', tea.ModCtrl), quit},
		{"a global key in the global scope", Global, press(tea.KeyPgDown, tea.ModCtrl), next},
		{"the list's key", "list", press(tea.KeyDown, 0), listDown},
		{"the same key in the view", "view", press(tea.KeyDown, 0), viewDown},
		{"a letter", "view", typed("j"), viewDown},
		{"another pane's key", "list", typed("j"), ID{}},
		{"a pane's key in the global scope", Global, press(tea.KeyDown, 0), ID{}},
		{"a key bound to nothing", "list", press(tea.KeyF5, 0), ID{}},
		{"a key press of several characters", "view", tea.KeyPressMsg{Code: tea.KeyExtended, Text: "jj"}, ID{}},
		{"an unknown scope", "nope", press('q', tea.ModCtrl), ID{}},
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
		msg  tea.KeyPressMsg
		want ID // the zero ID for none
	}{
		{"the space bar", typed(" "), listOpen},
		{"alt and an arrow", press(tea.KeyUp, tea.ModAlt), listUp},
		{"alt and a letter", press('k', tea.ModAlt), listUp},
		{"alt and the space bar", press(tea.KeySpace, tea.ModAlt), listDown},
		{"the letter without alt", typed("k"), ID{}},
		{"the arrow without alt", press(tea.KeyUp, 0), ID{}},
	} {
		id, ok := m.Lookup("list", c.msg)
		if id != c.want || ok != (c.want != ID{}) {
			t.Errorf("%s: Lookup = %v, %t; want %v", c.name, id, ok, c.want)
		}
	}
}

// Review focus: a key matches its binding however the terminal encodes it.
func TestLookupMatchesEveryEncoding(t *testing.T) {
	m := build(t, overrides{"list": {"up": {"ctrl+shift+z"}, "down": {"alt+shift+a"}, "open": {"ctrl+z"}}})
	for _, c := range []struct {
		name string
		msg  tea.KeyPressMsg
		want ID
	}{
		{"ctrl+shift+z, the kitty protocol", press('z', tea.ModCtrl|tea.ModShift), listUp},
		{"ctrl+shift+z, xterm", press('Z', tea.ModCtrl|tea.ModShift), listUp},
		{"ctrl+z with caps lock, xterm", press('Z', tea.ModCtrl), listOpen},
		{"ctrl+z on a Russian layout, the kitty protocol", tea.KeyPressMsg{Code: 'я', BaseCode: 'z', Mod: tea.ModCtrl}, listOpen},
		{"alt+A, ESC A", tea.KeyPressMsg{Code: 'a', ShiftedCode: 'A', Mod: tea.ModAlt | tea.ModShift}, listDown},
		{"ctrl+z with caps lock", press('z', tea.ModCtrl|tea.ModCapsLock), listOpen},
		{"ctrl+z with num lock", press('z', tea.ModCtrl|tea.ModNumLock), listOpen},
	} {
		if id, _ := m.Lookup("list", c.msg); id != c.want {
			t.Errorf("%s: Lookup = %v, want %v", c.name, id, c.want)
		}
	}
}

func TestNames(t *testing.T) {
	for _, c := range []struct {
		name string
		key  tea.Key
		want string
	}{
		{"ctrl+z, a control byte", tea.Key{Code: 'z', Mod: tea.ModCtrl}, "ctrl+z"},
		{"ctrl+shift+z, the kitty protocol", tea.Key{Code: 'z', Mod: tea.ModCtrl | tea.ModShift}, "ctrl+shift+z"},
		{"ctrl+shift+z, xterm", tea.Key{Code: 'Z', Mod: tea.ModCtrl | tea.ModShift}, "ctrl+shift+z"},
		{"ctrl+z with caps lock, xterm", tea.Key{Code: 'Z', Mod: tea.ModCtrl}, "ctrl+z"},
		{"alt+A, ESC A", tea.Key{Code: 'a', ShiftedCode: 'A', Mod: tea.ModAlt | tea.ModShift}, "alt+shift+a"},
		{"alt+Ñ, ESC Ñ", tea.Key{Code: 'Ñ', Mod: tea.ModAlt}, "alt+shift+ñ"},
		{"alt+?, ESC ?", tea.Key{Code: '?', Mod: tea.ModAlt}, "alt+?"},
		{"alt+?, xterm", tea.Key{Code: '?', Mod: tea.ModAlt | tea.ModShift}, "alt+?"},
		{"alt+?, the kitty protocol", tea.Key{Code: '/', ShiftedCode: '?', Mod: tea.ModAlt | tea.ModShift}, "alt+?"},
		{"ctrl+shift+1, the kitty protocol", tea.Key{Code: '1', ShiftedCode: '!', Mod: tea.ModCtrl | tea.ModShift}, "ctrl+!"},
		{"ctrl+q on a Russian layout, the kitty protocol", tea.Key{Code: 'й', BaseCode: 'q', Mod: tea.ModCtrl}, "ctrl+q"},
		{"ctrl+shift+q on a Russian layout", tea.Key{Code: 'й', ShiftedCode: 'q', BaseCode: 'q', Mod: tea.ModCtrl | tea.ModShift}, "ctrl+shift+q"},
		{"ctrl+ñ on a Spanish layout, a Latin letter", tea.Key{Code: 'ñ', BaseCode: ';', Mod: tea.ModCtrl}, "ctrl+ñ"},
		{"alt+й on a Russian layout, as ESC й sends it", tea.Key{Code: 'й', BaseCode: 'q', Mod: tea.ModAlt}, "alt+й"},
		{"й typed", tea.Key{Code: 'й', BaseCode: 'q', Text: "й"}, "й"},
		{"J typed", tea.Key{Code: 'j', ShiftedCode: 'J', Mod: tea.ModShift, Text: "J"}, "J"},
		{"A with caps lock", tea.Key{Code: 'a', Mod: tea.ModCapsLock, Text: "A"}, "A"},
		{"ñ typed", tea.Key{Code: 'ñ', Text: "ñ"}, "ñ"},
		{"a cluster an input method typed", tea.Key{Code: tea.KeyExtended, Text: "👍🏽"}, "👍🏽"},
		{"the space bar", tea.Key{Code: tea.KeySpace, Text: " "}, "space"},
		{"shift and the space bar", tea.Key{Code: tea.KeySpace, Mod: tea.ModShift, Text: " "}, "shift+space"},
		{"shift+tab", tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}, "shift+tab"},
		{"keypad enter", tea.Key{Code: tea.KeyKpEnter}, "enter"},
		{"ctrl+z with caps lock", tea.Key{Code: 'z', Mod: tea.ModCtrl | tea.ModCapsLock}, "ctrl+z"},
		{"super+f5", tea.Key{Code: tea.KeyF5, Mod: tea.ModSuper}, "super+f5"},
	} {
		if got := name(c.key); got != c.want {
			t.Errorf("%s: name = %q, want %q", c.name, got, c.want)
		}
	}
}

// recorder is a Bubble Tea program that keeps the key presses it gets, and
// quits once it has want of them.
type recorder struct {
	want int
	keys []tea.Key
}

func (r recorder) Init() tea.Cmd { return nil }

func (r recorder) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		r.keys = append(r.keys, k.Key())
		if len(r.keys) == r.want {
			return r, tea.Quit
		}
	}
	return r, nil
}

func (r recorder) View() tea.View { return tea.NewView("") }

// decode runs a real Bubble Tea program on input, as a terminal sends it,
// and returns the names of the first n key presses it decodes.
func decode(t *testing.T, input string, n int) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p := tea.NewProgram(recorder{want: n}, tea.WithContext(ctx), tea.WithInput(strings.NewReader(input)),
		tea.WithOutput(io.Discard), tea.WithoutSignalHandler(), tea.WithWindowSize(80, 24))
	final, err := p.Run()
	if err != nil {
		t.Fatalf("a program on %q: %v", input, err)
	}
	var names []string
	for _, k := range final.(recorder).keys {
		names = append(names, name(k))
	}
	return names
}

// Review focus: the names follow what Bubble Tea's own decoder makes of each
// encoding, so an upgrade that changes it fails here rather than at a key.
func TestTheDecoderAgreesWithTheNames(t *testing.T) {
	for _, c := range []struct{ name, input, want string }{
		{"ctrl+z, a control byte", "\x1a", "ctrl+z"},
		{"ctrl+shift+z, the kitty protocol", "\x1b[122;6u", "ctrl+shift+z"},
		{"ctrl+shift+z, xterm's modifyOtherKeys", "\x1b[27;6;90~", "ctrl+shift+z"},
		{"ctrl+z with caps lock, the kitty protocol", "\x1b[122;69u", "ctrl+z"},
		{"ctrl+z with num lock, the kitty protocol", "\x1b[122;133u", "ctrl+z"},
		{"ctrl+z with caps lock, xterm's modifyOtherKeys", "\x1b[27;5;90~", "ctrl+z"},
		{"alt+A, ESC A", "\x1bA", "alt+shift+a"},
		{"alt+Ñ, ESC Ñ", "\x1bÑ", "alt+shift+ñ"},
		{"J", "J", "J"},
		{"the space bar", " ", "space"},
		{"shift+tab", "\x1b[Z", "shift+tab"},
		{"keypad enter, the kitty protocol", "\x1b[57414u", "enter"},
		{"ctrl+c, a control byte", "\x03", "ctrl+c"},
		{"ctrl+pgdown", "\x1b[6;5~", "ctrl+pgdown"},
		{"alt+right", "\x1b[1;3C", "alt+right"},
		{"ctrl+shift+left", "\x1b[1;6D", "ctrl+shift+left"},
		{"shift+enter, the kitty protocol", "\x1b[13;2u", "shift+enter"},
		{"f5", "\x1b[15~", "f5"},
		{"alt+?, ESC ?", "\x1b?", "alt+?"},
		{"alt+?, xterm's modifyOtherKeys", "\x1b[27;4;63~", "alt+?"},
		{"alt+?, the kitty protocol with alternate keys", "\x1b[47:63;4u", "alt+?"},
		{"ctrl+q on a Russian layout, the kitty protocol with alternate keys", "\x1b[1081::113;5u", "ctrl+q"},
	} {
		if got := decode(t, c.input, 1); !slices.Equal(got, []string{c.want}) {
			t.Errorf("%s: %q decodes as %q, want %q", c.name, c.input, got, c.want)
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
		{"meta, hyper and super", overrides{"list": {"up": {"super+up"}, "down": {"super+down"}}},
			[]ID{listUp, listDown}, "super+↑/↓"},
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

// typingProblems returns the problems New reports for the small program
// with o when the view is a scope where keys type text, one per line, or ""
// when there are none.
func typingProblems(o overrides) string {
	if _, err := New(testActions(), o, "view"); err != nil {
		return err.Error()
	}
	return ""
}

// cleanView is the overrides that leave the view without letters, binding
// view.up to up and view.down to keys.
func cleanView(keys ...string) overrides {
	return overrides{"view": {"up": {"up"}, "down": keys}}
}

func TestKeysThatTypeTextInATypingScope(t *testing.T) {
	if got, want := typingProblems(nil), "view.down: key \"j\" types text\nview.up: key \"k\" types text"; got != want {
		t.Errorf("the view's default letters: problems =\n%s\nwant\n%s", got, want)
	}
	for _, c := range []struct {
		name string
		o    overrides
		want string
	}{
		{"a letter", cleanView("j"), `view.down: key "j" types text`},
		{"a capital", cleanView("J"), `view.down: key "J" types text`},
		{"a letter with an accent", cleanView("ñ"), `view.down: key "ñ" types text`},
		{"a digit", cleanView("1"), `view.down: key "1" types text`},
		{"the space bar", cleanView(" ", "space"), `view.down: key "space" types text`},
		{"shift and the space bar", cleanView("shift+space"), `view.down: key "shift+space" types text`},
		{"a letter in the global scope", overrides{"view": cleanView("down")["view"], Global: {"next": {"x"}}},
			`global.next: key "x" types text`},
		{"alt and a letter", cleanView("alt+j"), ""},
		{"alt and the space bar", cleanView("alt+space"), ""},
		{"a control key", cleanView("ctrl+k"), ""},
		{"ctrl and shift and a letter", cleanView("ctrl+shift+z"), ""},
		{"named keys", cleanView("enter", "f5", "tab", "backspace"), ""},
		{"a letter in another pane", overrides{"view": cleanView("down")["view"], "list": {"down": {"j"}}}, ""},
		{"an unknown key is only unknown", cleanView("ctrl+uo"), `view.down: unknown key "ctrl+uo"`},
		{"sorted with the other problems", overrides{"view": {"up": {"ctrl+uo"}, "down": {"j"}}, Global: {"quit": {}}},
			"global.quit: needs at least one key\nview.down: key \"j\" types text\nview.up: unknown key \"ctrl+uo\""},
	} {
		if got := typingProblems(c.o); got != c.want {
			t.Errorf("%s: problems =\n%s\nwant\n%s", c.name, got, c.want)
		}
	}
}

func TestWithoutATypingScopeGlobalLettersAreFine(t *testing.T) {
	if got := problems(overrides{Global: {"next": {"x"}}}); got != "" {
		t.Errorf("problems %q, want none", got)
	}
}

func TestATypingScopeMustHaveActions(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("New should panic on a typing scope no action is in")
		}
	}()
	_, _ = New(testActions(), cleanView("down"), "viwe")
}

func TestTyped(t *testing.T) {
	for _, c := range []struct {
		name string
		msg  tea.KeyPressMsg
		want string // "" when msg types nothing
	}{
		{"a letter", typed("j"), "j"},
		{"a capital", tea.KeyPressMsg{Code: 'j', ShiftedCode: 'J', Mod: tea.ModShift, Text: "J"}, "J"},
		{"a capital with caps lock", tea.KeyPressMsg{Code: 'j', Mod: tea.ModCapsLock, Text: "J"}, "J"},
		{"a cluster an input method typed", tea.KeyPressMsg{Code: tea.KeyExtended, Text: "👍🏽"}, "👍🏽"},
		{"the space bar", typed(" "), " "},
		{"shift and the space bar", tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModShift, Text: " "}, " "},
		{"alt and a letter", press('j', tea.ModAlt), ""},
		{"alt and the space bar", press(tea.KeySpace, tea.ModAlt), ""},
		{"ctrl and shift and a letter", press('z', tea.ModCtrl|tea.ModShift), ""},
		{"enter", press(tea.KeyEnter, 0), ""},
		{"tab", press(tea.KeyTab, 0), ""},
		{"a control key", press('k', tea.ModCtrl), ""},
		{"a key with no text", press(tea.KeyF1, 0), ""},
	} {
		text, ok := Typed(c.msg)
		if text != c.want || ok != (c.want != "") {
			t.Errorf("%s: Typed = %q, %t; want %q", c.name, text, ok, c.want)
		}
	}
}
