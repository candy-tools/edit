// Package keymap binds the keys a Bubble Tea program receives to the actions
// they trigger, and lets the user rebind them. Actions live in scopes: the
// global scope is active everywhere, and the caller says which other scope
// is active with it, such as the focused pane. Keys are written as Bubble Tea
// names them ("ctrl+w", "pgdown", "j"), with "space" for the space bar.
//
// A scope can be one where keys type text, such as a text editor's: there a
// key that types text can't be bound, nor in the global scope, which is
// active alongside it, and Typed says what a key press types.
//
// It is app-agnostic and depends only on the standard library and Bubble
// Tea; a depguard rule in .golangci.yaml keeps it from importing the app.
package keymap

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// Global is the scope whose actions are active alongside every other scope.
const Global = "global"

// ID names an action: its scope and its name within the scope.
type ID struct{ Scope, Name string }

// String is the ID as config files and messages write it: "sidebar.down".
func (id ID) String() string { return id.Scope + "." + id.Name }

// Action is one thing a key can do.
type Action struct {
	ID
	Keys     []string // the default keys, first key first
	Required bool     // must keep at least one key
}

// Map is the keys bound to a set of actions. It is read-only once built.
type Map struct {
	keys     map[ID][]string          // each action's keys, first key first; absent when it has none
	bindings map[string]map[string]ID // scope → key → the action the key triggers
}

// New binds each of actions to its default keys, except that an action
// overrides lists (scope → action name → keys) gets exactly the keys listed
// there instead, so an empty list leaves it without keys. A key listed twice
// for one action is kept once.
//
// typing names the scopes where keys type text, such as a text editor's. A
// key that types text — a single character, or space, without alt — can't
// be bound in them, nor in the global scope when there are any, since
// global actions are active alongside them.
//
// It returns an error listing every problem, one per line, sorted: a scope
// or an action in overrides that actions doesn't have, a key name Bubble Tea
// can't report, a key that types text bound where keys type text, a
// Required action left with no keys, and a key bound to two actions that are
// active together, because they are in the same scope or one of them is
// global. The defaults are checked too, since an override can clash with
// another action's default key.
//
// An action with an empty scope or name, two actions with the same ID, or a
// typing scope no action is in, is a bug in the caller: New panics.
func New(actions []Action, overrides map[string]map[string][]string, typing ...string) (*Map, error) {
	ids, scopes := index(actions)
	typed := typingScopes(typing, scopes)
	problems := unknown(overrides, ids, scopes)
	m := &Map{keys: map[ID][]string{}, bindings: map[string]map[string]ID{}}
	byKey := map[string]map[string][]ID{} // scope → key → the actions it is bound to
	for scope := range scopes {
		byKey[scope] = map[string][]ID{}
	}
	for _, a := range actions {
		keys := a.Keys
		if listed, ok := overrides[a.Scope][a.Name]; ok {
			keys = listed
		}
		keys = clean(keys)
		if a.Required && len(keys) == 0 {
			problems = append(problems, a.String()+": needs at least one key")
		}
		for _, k := range keys {
			if !valid(k) {
				problems = append(problems, fmt.Sprintf("%s: unknown key %q", a.ID, k))
				continue
			}
			if typed[a.Scope] && typesText(k) {
				problems = append(problems, fmt.Sprintf("%s: key %q types text", a.ID, k))
				continue
			}
			m.keys[a.ID] = append(m.keys[a.ID], k)
			byKey[a.Scope][k] = append(byKey[a.Scope][k], a.ID)
		}
	}
	problems = append(problems, conflicts(byKey)...)
	if len(problems) > 0 {
		slices.Sort(problems)
		errs := make([]error, 0, len(problems))
		for _, p := range slices.Compact(problems) {
			errs = append(errs, errors.New(p))
		}
		return nil, errors.Join(errs...)
	}
	for scope, keys := range byKey {
		m.bindings[scope] = make(map[string]ID, len(keys))
		for k, bound := range keys {
			m.bindings[scope][k] = bound[0]
		}
	}
	return m, nil
}

// index returns the IDs and the scopes of actions. It panics on an action
// with an empty scope or name, and on two actions with the same ID.
func index(actions []Action) (ids map[ID]bool, scopes map[string]bool) {
	ids, scopes = make(map[ID]bool, len(actions)), map[string]bool{}
	for _, a := range actions {
		if a.Scope == "" || a.Name == "" {
			panic(fmt.Sprintf("keymap: action %q has an empty scope or name", a.ID))
		}
		if ids[a.ID] {
			panic(fmt.Sprintf("keymap: action %s is listed twice", a.ID))
		}
		ids[a.ID], scopes[a.Scope] = true, true
	}
	return ids, scopes
}

// typingScopes returns the scopes where a key that types text can't be
// bound: typing and, when it names any, the global scope. It panics on a
// typing scope that is not among scopes: Lookup finds nothing in a scope
// without actions, so it can only be a typo, which would turn the check off.
func typingScopes(typing []string, scopes map[string]bool) map[string]bool {
	typed := make(map[string]bool, len(typing)+1)
	for _, scope := range typing {
		if !scopes[scope] {
			panic(fmt.Sprintf("keymap: typing scope %q has no actions", scope))
		}
		typed[scope], typed[Global] = true, true
	}
	return typed
}

// unknown lists the scopes and the actions in overrides that are not among
// scopes and ids.
func unknown(overrides map[string]map[string][]string, ids map[ID]bool, scopes map[string]bool) []string {
	var problems []string
	for scope, names := range overrides {
		if !scopes[scope] {
			problems = append(problems, scope+": unknown scope")
			continue
		}
		for name := range names {
			if id := (ID{scope, name}); !ids[id] {
				problems = append(problems, id.String()+": unknown action")
			}
		}
	}
	return problems
}

// conflicts lists the keys bound to two actions that are active together:
// in the same scope, or one of them global. byKey is scope → key → the
// actions the key is bound to. The actions in a message are in sorted
// order, and a key on three or more actions is reported against the first
// of them, once for each of the others. A clash between global actions is
// found again in every scope, so the caller drops the repeats.
func conflicts(byKey map[string]map[string][]ID) []string {
	var problems []string
	for scope, keys := range byKey {
		for k, bound := range keys {
			bound = slices.Clone(bound)
			if scope != Global {
				bound = append(bound, byKey[Global][k]...)
			}
			if len(bound) < 2 {
				continue
			}
			slices.SortFunc(bound, func(a, b ID) int { return strings.Compare(a.String(), b.String()) })
			for _, other := range bound[1:] {
				problems = append(problems, fmt.Sprintf("key %q is bound to both %s and %s", k, bound[0], other))
			}
		}
	}
	return problems
}

// Lookup returns the action that msg triggers while scope is active: a
// global action first, then one of scope's. Pasted text never triggers an
// action, and neither does a key bound to nothing, nor any key in an
// unknown scope. It doesn't change the map, so it is safe to call from a
// Bubble Tea View.
func (m *Map) Lookup(scope string, msg tea.KeyMsg) (ID, bool) {
	keys, ok := m.bindings[scope]
	if !ok || msg.Paste {
		return ID{}, false
	}
	k := spell(msg.String())
	if id, ok := m.bindings[Global][k]; ok {
		return id, true
	}
	id, ok := keys[k]
	return id, ok
}

// Presses splits msg into the key presses it holds. Bubble Tea reports the
// characters that reach it together as one message, so a key held down, or
// keys typed fast, can arrive as "jjj": three presses of j. Any other
// message, pasted text included, is one press.
func Presses(msg tea.KeyMsg) []tea.KeyMsg {
	if msg.Type != tea.KeyRunes || msg.Paste || len(msg.Runes) < 2 {
		return []tea.KeyMsg{msg}
	}
	presses := make([]tea.KeyMsg, len(msg.Runes))
	for i, r := range msg.Runes {
		presses[i] = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: msg.Alt}
	}
	return presses
}

// Typed returns the text msg types where keys type text, and whether it
// types any: the characters of a key press or of pasted text, or a space for
// the space bar. A key pressed with alt types nothing. These are the keys
// New won't bind where keys type text.
func Typed(msg tea.KeyMsg) (string, bool) {
	switch {
	case msg.Alt:
		return "", false
	case msg.Type == tea.KeyRunes && len(msg.Runes) > 0:
		return string(msg.Runes), true
	case msg.Type == tea.KeySpace:
		return " ", true
	}
	return "", false
}

// Keys returns the keys of the action id, first key first, in a new slice:
// nil when it has none or id is not an action.
func (m *Map) Keys(id ID) []string {
	return slices.Clone(m.keys[id])
}

// Label writes the keys of a footer hint: the first key of each of ids that
// has keys, in order, and "" when none has. Each key splits into its
// modifiers, the leading "ctrl+", "alt+" and "shift+", and its base, with an
// arrow drawn as ↑, ↓, ← or →. When all the keys have the same modifiers,
// those are written once, then the bases: run together when there are no
// modifiers and every base is an arrow ("↑↓"), joined with "/" otherwise
// ("k/j", "ctrl+pgup/pgdown", "alt+←/→"). Otherwise each key is written in
// full and joined with ", " ("ctrl+pgup, pgdown"), so the label never shows
// a shared modifier the keys don't have.
func (m *Map) Label(ids ...ID) string {
	var mods, bases []string
	allArrows := true
	for _, id := range ids {
		keys := m.keys[id]
		if len(keys) == 0 {
			continue
		}
		mod, base := split(keys[0])
		if arrow, ok := arrows[base]; ok {
			base = arrow
		} else {
			allArrows = false
		}
		mods, bases = append(mods, mod), append(bases, base)
	}
	switch {
	case len(bases) == 0:
		return ""
	case slices.ContainsFunc(mods, func(mod string) bool { return mod != mods[0] }):
		for i := range bases {
			bases[i] = mods[i] + bases[i]
		}
		return strings.Join(bases, ", ")
	case mods[0] == "" && allArrows:
		return strings.Join(bases, "")
	}
	return mods[0] + strings.Join(bases, "/")
}

// modifiers are the prefixes a key's name can start with.
var modifiers = []string{"ctrl+", "alt+", "shift+"}

// arrows draws the bases of the arrow keys.
var arrows = map[string]string{"up": "↑", "down": "↓", "left": "←", "right": "→"}

// split splits key into its modifiers, the leading run of modifiers that
// leaves something after it, and its base: "ctrl+shift+up" is "ctrl+shift+"
// and "up", "alt++" is "alt+" and "+".
func split(key string) (mods, base string) {
	base = key
	for {
		i := strings.IndexByte(base, '+') + 1
		if i == 0 || i == len(base) || !slices.Contains(modifiers, base[:i]) {
			return key[:len(key)-len(base)], base
		}
		base = base[i:]
	}
}

// named holds the names Bubble Tea gives its key types, in the config's
// spelling. It asks Bubble Tea to name every key type, from KeyF20, the
// lowest, to KeyBackspace, the highest, so it follows Bubble Tea's own
// table; the types that have no name, KeyRunes among them, name themselves
// "".
var named = func() map[string]bool {
	names := map[string]bool{}
	for t := tea.KeyF20; t <= tea.KeyBackspace; t++ {
		if name := (tea.Key{Type: t}).String(); name != "" {
			names[spell(name)] = true
		}
	}
	return names
}()

// spell turns a key's name as Bubble Tea writes it into the config's
// spelling, which calls the space bar "space" instead of " ".
func spell(name string) string {
	switch name {
	case " ":
		return "space"
	case "alt+ ":
		return "alt+space"
	}
	return name
}

// clean spells keys the config's way and drops the repeats, keeping the
// first of each.
func clean(keys []string) []string {
	var out []string
	for _, k := range keys {
		if k = spell(k); !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// typesText reports whether key, a valid key name, types text: the space bar
// or a single character, without alt. No name Bubble Tea gives a key type is
// a single character, so one character is always a character key.
func typesText(key string) bool {
	return key == "space" || utf8.RuneCountInString(key) == 1
}

// valid reports whether Bubble Tea can report key: a name it gives one of
// its key types, a single character other than a control character, or
// either of those after "alt+".
func valid(key string) bool {
	key = strings.TrimPrefix(key, "alt+")
	if named[key] {
		return true
	}
	r, size := utf8.DecodeRuneInString(key)
	return size > 0 && size == len(key) && r != utf8.RuneError && !unicode.IsControl(r)
}
