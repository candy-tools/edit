// Package keymap binds the keys a Bubble Tea program receives to the actions
// they trigger, and lets the user rebind them. Actions live in scopes: the
// caller says which scope is active, such as the focused pane, and the
// global scope is active alongside it unless that scope is modal. Keys are
// written as Bubble Tea names them, modifiers first ("ctrl+w", "pgdown",
// "ctrl+shift+z", "j"), with "space" for the space bar. A terminal can send
// one key in several ways — xterm reports ctrl+shift+z as a capital Z with
// shift, the kitty keyboard protocol as a small z — so the map gives each
// key press, and each key it is given, one name: see New.
//
// An action can also be bound to a sequence of keys, pressed one after the
// other, written with a space between them ("esc esc", "ctrl+k w"). Its
// first keys start it, and the caller keeps them until its last key, as
// Lookup returns them; how long it waits for the next key is up to the
// caller.
//
// A scope can be one where keys type text, such as a text editor's: there a
// key that types text can't be bound, nor in the global scope, which is
// active alongside it, and Typed says what a key press types. A scope can
// also be modal, as a popup that takes every key is: the global actions are
// not active alongside it, so its keys don't clash with theirs.
//
// It is app-agnostic and depends only on the standard library and Bubble
// Tea; a depguard rule in .golangci.yaml keeps it from importing the app.
package keymap

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// Global is the scope whose actions are active alongside every other scope
// but a modal one.
const Global = "global"

// ID names an action: its scope and its name within the scope.
type ID struct{ Scope, Name string }

// String is the ID as config files and messages write it: "sidebar.down".
func (id ID) String() string { return id.Scope + "." + id.Name }

// Action is one thing a key can do.
type Action struct {
	ID
	Keys     []string // the default keys or sequences, first key first
	Required bool     // must keep at least one key
}

// A Scope says how keys work in one of the scopes other than the global
// one. A scope that New is given no Scope for is plain: keys type no text
// there, and the global actions are active alongside it.
type Scope struct {
	Name string
	// Typing says keys type text in the scope, as in a text editor: a key
	// that types text can't be bound there.
	Typing bool
	// Modal says the global actions are not active alongside the scope, as
	// in a popup that takes every key.
	Modal bool
}

// Map is the keys bound to a set of actions. It is read-only once built.
type Map struct {
	keys     map[ID][]string            // each action's keys, first key first; absent when it has none
	bindings map[string]map[string]ID   // scope → key → the action the key triggers
	starts   map[string]map[string]bool // scope → the starts of its sequences: their first key, first two keys, …
	modal    map[string]bool            // the modal scopes, where the global actions are not active
}

// New binds each of actions to its default keys, except that an action
// overrides lists (scope → action name → keys) gets exactly the keys listed
// there instead, so an empty list leaves it without keys. A key listed twice
// for one action is kept once.
//
// A key is any of the modifiers "ctrl+", "alt+", "shift+", "meta+", "hyper+"
// and "super+", each at most once and in any order, then the name Bubble Tea
// gives a key that isn't a character ("enter", "pgdown", "f5", "space") or a
// single character other than a control character; " " is the space bar.
// Shift alone before a character is no key: the character it types is ("J",
// not "shift+j"). New keeps a key in the spelling the key press gets: the
// modifiers in Bubble Tea's order, ctrl, alt, shift, meta, hyper, super, and
// a capital letter after a modifier as shift and the small letter, so
// "shift+ctrl+Z" is "ctrl+shift+z".
//
// A sequence is two keys or more with a space between each two, each key
// spelled as above. A space is part of a key when nothing but modifiers come
// before it in the key, so "alt+ " is still alt and the space bar.
//
// scopes says how keys work in the scopes that aren't plain. In a typing
// scope, such as a text editor's, a key that types text — a single
// character, or the space bar with shift or without — can't be bound, nor
// start a sequence, and neither can it in the global scope when a typing
// scope isn't modal, since the global actions are active alongside it. Such
// a key can go on with a sequence ("ctrl+k w"): the sequence's start takes
// it before it types. In a modal scope, such as a popup's, the global
// actions are not active, so its keys are checked against each other only.
//
// It returns an error listing every problem, one per line, sorted: a scope
// or an action in overrides that actions doesn't have, a key that is no key,
// a key that types text bound where keys type text, a Required action left
// with no keys, a key bound to two actions that are active together,
// because they are in the same scope or one of them is global and the
// other's scope isn't modal, and a key bound to one action that starts a
// sequence bound to another active with it, which the key would both
// trigger and start. The defaults are checked too, since an override can
// clash with another action's default key.
//
// An action with an empty scope or name, two actions with the same ID, or a
// Scope that is the global scope, that no action is in or that is given
// twice, is a bug in the caller: New panics.
func New(actions []Action, overrides map[string]map[string][]string, scopes ...Scope) (*Map, error) {
	ids, names := index(actions)
	typed, modal := describe(scopes, names)
	problems := unknown(overrides, ids, names)
	m := &Map{keys: map[ID][]string{}, bindings: map[string]map[string]ID{}, starts: map[string]map[string]bool{}, modal: modal}
	byKey := map[string]map[string][]ID{} // scope → key → the actions it is bound to
	for scope := range names {
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
			if _, err := spellBinding(k); err != nil {
				problems = append(problems, a.String()+": "+err.Error())
				continue
			}
			if first, _, _ := strings.Cut(k, " "); typed[a.Scope] && typesText(first) {
				problems = append(problems, fmt.Sprintf("%s: key %s types text", a.ID, quoteKey(first, k)))
				continue
			}
			m.keys[a.ID] = append(m.keys[a.ID], k)
			byKey[a.Scope][k] = append(byKey[a.Scope][k], a.ID)
		}
	}
	problems = append(problems, conflicts(byKey, modal)...)
	problems = append(problems, ambiguities(byKey, modal)...)
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
		m.starts[scope] = map[string]bool{}
		for k, bound := range keys {
			m.bindings[scope][k] = bound[0]
			seq := strings.Split(k, " ")
			for n := 1; n < len(seq); n++ {
				m.starts[scope][strings.Join(seq[:n], " ")] = true
			}
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

// describe returns the scopes where a key that types text can't be bound —
// the typing scopes and, when one of them isn't modal, the global scope,
// which is active alongside it — and the modal scopes. It panics on a Scope
// that is the global scope, that names a scope no action is in, or that is
// given twice: Lookup finds nothing in a scope without actions, so the name
// can only be a typo, which would turn the checks off.
func describe(scopes []Scope, names map[string]bool) (typed, modal map[string]bool) {
	typed, modal = map[string]bool{}, map[string]bool{}
	seen := make(map[string]bool, len(scopes))
	for _, s := range scopes {
		switch {
		case s.Name == Global:
			panic("keymap: the global scope can't be given a Scope")
		case !names[s.Name]:
			panic(fmt.Sprintf("keymap: scope %q has no actions", s.Name))
		case seen[s.Name]:
			panic(fmt.Sprintf("keymap: scope %q is described twice", s.Name))
		}
		seen[s.Name] = true
		if s.Typing {
			typed[s.Name] = true
			if !s.Modal {
				typed[Global] = true
			}
		}
		if s.Modal {
			modal[s.Name] = true
		}
	}
	return typed, modal
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
// in the same scope, or one of them global while the other's scope isn't
// modal. byKey is scope → key → the actions the key is bound to, and modal
// holds the modal scopes. The actions in a message are in sorted order, and
// a key on three or more actions is reported against the first of them,
// once for each of the others. A clash between global actions is found
// again in every scope, so the caller drops the repeats.
func conflicts(byKey map[string]map[string][]ID, modal map[string]bool) []string {
	var problems []string
	for scope, keys := range byKey {
		for k, bound := range keys {
			bound = slices.Clone(bound)
			if scope != Global && !modal[scope] {
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

// ambiguities lists the keys bound to an action that start a sequence bound
// to another, when the two are active together: in the same scope, or one
// of them global while the other's scope isn't modal. The key would both
// trigger its action and start the sequence. byKey is scope → key → the
// actions the key is bound to, and modal holds the modal scopes. A clash
// between global actions is found again in every scope, so the caller drops
// the repeats.
func ambiguities(byKey map[string]map[string][]ID, modal map[string]bool) []string {
	var problems []string
	for scope := range byKey {
		alongside := scope != Global && !modal[scope] // the global actions are active with scope's
		active := func(k string) []ID {
			bound := slices.Clone(byKey[scope][k])
			if alongside {
				bound = append(bound, byKey[Global][k]...)
			}
			return bound
		}
		keysets := []map[string][]ID{byKey[scope]}
		if alongside {
			keysets = append(keysets, byKey[Global])
		}
		for _, keys := range keysets {
			for k := range keys {
				seq := strings.Split(k, " ")
				for n := 1; n < len(seq); n++ {
					start := strings.Join(seq[:n], " ")
					for _, short := range active(start) {
						for _, long := range active(k) {
							problems = append(problems, fmt.Sprintf("key %q is bound to %s and starts %q of %s", start, short, k, long))
						}
					}
				}
			}
		}
	}
	return problems
}

// Lookup returns what msg does while scope is active, pressed after
// pending, the keys of a sequence pressed so far ("" for none): the action
// it triggers, a global action first, unless scope is modal, then one of
// scope's; or, when it goes on with a sequence without ending it, no action
// and next, the keys of the sequence pressed so far, for the caller to pass
// to the next Lookup. A key that goes on with no sequence after pending
// counts on its own. A key bound to nothing triggers none, and neither does
// any key in an unknown scope. It doesn't change the map, so it is safe to
// call from a Bubble Tea View.
//
// A terminal without the kitty keyboard protocol sends alt and a key as esc
// then the key, so Bubble Tea decodes esc and a key pressed together, such
// as a quick double esc, as alt and the key. An alt key that does nothing is
// therefore taken as esc then the key, if they trigger an action or go on
// with a sequence: alt+esc as esc esc. An alt+esc that does nothing even so
// is taken as one esc, when esc triggers an action, its second esc dropped:
// a fast double esc closes a popup bound to esc. Other alt keys are never
// taken for esc alone, since Bubble Tea decodes real alt chords, alt+b or
// alt+left, the same way.
func (m *Map) Lookup(scope, pending string, msg tea.KeyPressMsg) (id ID, next string, ok bool) {
	if _, known := m.bindings[scope]; !known {
		return ID{}, "", false
	}
	key := msg.Key()
	id, next, ok = m.step(scope, pending, name(key))
	if ok || next != "" || key.Mod&tea.ModAlt == 0 {
		return id, next, ok
	}
	escID, afterEsc, escOK := m.step(scope, pending, "esc")
	if afterEsc != "" {
		if id, next, ok := m.exact(scope, afterEsc+" "+name(withoutAlt(key))); ok || next != "" {
			return id, next, ok
		}
	}
	if escOK && key.Code == tea.KeyEscape {
		return escID, "", true
	}
	return ID{}, "", false
}

// step is Lookup for the key named k: what pending and k do, or else what k
// does on its own.
func (m *Map) step(scope, pending, k string) (id ID, next string, ok bool) {
	if pending != "" {
		if id, next, ok := m.exact(scope, pending+" "+k); ok || next != "" {
			return id, next, ok
		}
	}
	return m.exact(scope, k)
}

// exact returns what the keys seq do while scope is active: the action they
// trigger or, when they start a sequence, seq itself as next. The global
// actions count first, unless scope is modal.
func (m *Map) exact(scope, seq string) (id ID, next string, ok bool) {
	scopes := []string{Global, scope}
	if m.modal[scope] {
		scopes = scopes[1:]
	}
	for _, s := range scopes {
		if id, ok := m.bindings[s][seq]; ok {
			return id, "", true
		}
	}
	for _, s := range scopes {
		if m.starts[s][seq] {
			return ID{}, seq, false
		}
	}
	return ID{}, "", false
}

// withoutAlt is the key k, pressed with alt, as it is pressed without: alt
// taken off and, for a capital, the text it types put back, which Bubble
// Tea leaves off a key pressed with alt. Without its text, a capital would
// be named shift and the small letter.
func withoutAlt(k tea.Key) tea.Key {
	k.Mod &^= tea.ModAlt
	if k.Mod&^locks == tea.ModShift && k.ShiftedCode != 0 {
		k.Text = string(k.ShiftedCode)
	}
	return k
}

// Typed returns the text msg types where keys type text, and whether it
// types any: the text of a key pressed with no modifier but shift and the
// lock keys, a space for the space bar. A key pressed with ctrl, alt, meta,
// hyper or super types nothing. These are the keys New won't bind where
// keys type text.
func Typed(msg tea.KeyPressMsg) (string, bool) {
	k := msg.Key()
	if k.Text == "" || k.Mod&^(locks|tea.ModShift) != 0 {
		return "", false
	}
	return k.Text, true
}

// Keys returns the keys of the action id, first key first, in a new slice:
// nil when it has none or id is not an action.
func (m *Map) Keys(id ID) []string {
	return slices.Clone(m.keys[id])
}

// Label writes the keys of a footer hint: the first key of each of ids that
// has keys, in order, and "" when none has. Each key splits into its
// modifiers, the leading "ctrl+", "alt+", "shift+", "meta+", "hyper+" and
// "super+", and its base, with an arrow drawn as ↑, ↓, ← or →. When all the
// keys have the same modifiers, those are written once, then the bases: run
// together when there are no modifiers and every base is an arrow ("↑↓"),
// joined with "/" otherwise ("k/j", "ctrl+pgup/pgdown", "alt+←/→").
// Otherwise, or when one of the keys is a sequence, each key is written in
// full and joined with ", " ("ctrl+pgup, pgdown", "ctrl+k p, ctrl+k n"), so
// the label never shows a shared modifier the keys don't have.
func (m *Map) Label(ids ...ID) string {
	return label(ids, func(id ID) []string { return m.keys[id] })
}

// SingleLabel is Label for the keys of ids that aren't sequences: for a hint
// whose keys only work pressed on their own, such as the answer to a prompt
// that the first key of a sequence would cancel.
func (m *Map) SingleLabel(ids ...ID) string {
	return label(ids, func(id ID) []string {
		return slices.DeleteFunc(slices.Clone(m.keys[id]), func(k string) bool { return strings.Contains(k, " ") })
	})
}

// label is Label for the keys that keys returns for each of ids.
func label(ids []ID, keys func(ID) []string) string {
	var mods, bases []string
	allArrows, sequence := true, false
	for _, id := range ids {
		keys := keys(id)
		if len(keys) == 0 {
			continue
		}
		mod, base := split(keys[0])
		if arrow, ok := arrows[base]; ok {
			base = arrow
		} else {
			allArrows = false
		}
		sequence = sequence || strings.Contains(base, " ")
		mods, bases = append(mods, mod), append(bases, base)
	}
	switch {
	case len(bases) == 0:
		return ""
	case sequence || slices.ContainsFunc(mods, func(mod string) bool { return mod != mods[0] }):
		for i := range bases {
			bases[i] = mods[i] + bases[i]
		}
		return strings.Join(bases, ", ")
	case mods[0] == "" && allArrows:
		return strings.Join(bases, "")
	}
	return mods[0] + strings.Join(bases, "/")
}

// arrows draws the bases of the arrow keys.
var arrows = map[string]string{"up": "↑", "down": "↓", "left": "←", "right": "→"}

// split splits key into its modifiers, the leading run of modifiers that
// leaves something after it, and its base: "ctrl+shift+up" is "ctrl+shift+"
// and "up", "alt++" is "alt+" and "+".
func split(key string) (mods, base string) {
	base = key
	for {
		_, rest, ok := cutModifier(base)
		if !ok {
			return key[:len(key)-len(base)], base
		}
		base = rest
	}
}

// locks are the lock keys, which a key's name leaves out: caps lock, num
// lock and scroll lock change the text a key types, not the key.
const locks = tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock

// name is the name of the key k: the text it types, when it types any but a
// space with no modifier held but shift; otherwise its modifiers, in the
// order Bubble Tea writes them, then its key, Bubble Tea's name for it or
// its character. A key gets the same name however the terminal encodes it:
//
//   - With ctrl, a letter of a script other than Latin is named by the key it
//     is on in the PC-101 layout, which the kitty keyboard protocol reports
//     beside it when asked for alternate keys: ctrl+й, on the Q key, is
//     ctrl+q, the control byte a legacy terminal sends for it. Other
//     letters keep their own name, ctrl+ñ and ctrl+a on any layout.
//   - A sign typed with shift is named by that sign, without shift: alt+? is
//     alt+?, which the kitty protocol reports as / with shift and the
//     shifted ?, and xterm as ? with shift.
//   - A capital letter is written small: xterm reports ctrl+shift+z as a
//     capital Z with shift, the kitty protocol as a small z with shift, and
//     caps lock and ctrl+z as a capital Z with ctrl alone, which is ctrl+z.
//     Bubble Tea puts a shift that was held in the modifiers, but for a
//     capital that isn't ASCII, as ESC Ñ, which gets its shift here.
func name(k tea.Key) string {
	mod := k.Mod &^ locks
	if k.Text != "" && k.Text != " " && mod&^tea.ModShift == 0 {
		return k.Text
	}
	code := k.Code
	if mod&tea.ModCtrl != 0 && foreign(code) && k.BaseCode > ' ' && k.BaseCode < utf8.RuneSelf {
		code = k.BaseCode
	}
	if mod&tea.ModShift != 0 && caseless(code) {
		if k.ShiftedCode != 0 {
			code = k.ShiftedCode
		}
		mod &^= tea.ModShift
	}
	if unicode.IsUpper(code) {
		if code >= utf8.RuneSelf {
			mod |= tea.ModShift
		}
		code = unicode.ToLower(code)
	}
	return tea.Key{Code: code, Mod: mod}.Keystroke()
}

// foreign reports whether r is a letter of a script other than Latin, such
// as Cyrillic or Greek.
func foreign(r rune) bool { return unicode.IsLetter(r) && !unicode.Is(unicode.Latin, r) }

// caseless reports whether r is a printable character other than the space
// that has neither a capital nor a small form, as a digit or a sign has.
func caseless(r rune) bool {
	return r != ' ' && unicode.IsPrint(r) && unicode.ToUpper(r) == r && unicode.ToLower(r) == r
}

// spell returns key, a key as the config writes it, in the spelling name
// gives the key press it stands for, and whether it is a key at all: any of
// the modifiers, each once, then the name of a special key or a single
// character other than a control character. A capital letter after a
// modifier is shift and the small letter. " " is the space bar, and so is
// "@" after ctrl: Bubble Tea v1 named ctrl+space by the ctrl+@ most
// terminals send for it. Shift alone before a character is no key: the
// character it types is; nor is shift before a sign with other modifiers:
// the sign shift types is the key, alt+? rather than alt+shift+/.
func spell(key string) (string, bool) {
	var mod tea.KeyMod
	base := key
	for {
		m, rest, ok := cutModifier(base)
		if !ok {
			break
		}
		if mod&m != 0 {
			return "", false
		}
		mod, base = mod|m, rest
	}
	if base == " " || base == "@" && mod&tea.ModCtrl != 0 {
		base = "space"
	}
	code, special := specials[base]
	if !special {
		r, size := utf8.DecodeRuneInString(base)
		if size == 0 || size != len(base) || r == utf8.RuneError || unicode.IsControl(r) || mod == tea.ModShift ||
			mod&tea.ModShift != 0 && caseless(r) {
			return "", false
		}
		if mod == 0 {
			return base, true
		}
		if unicode.IsUpper(r) {
			r, mod = unicode.ToLower(r), mod|tea.ModShift
		}
		code = r
	}
	return name(tea.Key{Code: code, Mod: mod}), true
}

// modifiers are the modifiers a key's name can start with, in the order
// Bubble Tea writes them.
var modifiers = []struct {
	prefix string
	mod    tea.KeyMod
}{
	{"ctrl+", tea.ModCtrl}, {"alt+", tea.ModAlt}, {"shift+", tea.ModShift},
	{"meta+", tea.ModMeta}, {"hyper+", tea.ModHyper}, {"super+", tea.ModSuper},
}

// cutModifier cuts the modifier key starts with off key, when it starts with
// one and something is left after it.
func cutModifier(key string) (mod tea.KeyMod, rest string, ok bool) {
	for _, m := range modifiers {
		if rest, ok := strings.CutPrefix(key, m.prefix); ok && rest != "" {
			return m.mod, rest, true
		}
	}
	return 0, key, false
}

// specials holds the names Bubble Tea gives the keys that aren't characters,
// with a code for each. It asks Bubble Tea to name each such key — enter,
// tab, backspace, esc and space, then every key code from KeyUp up to the
// modifier keys — so it follows Bubble Tea's own table. A key Bubble Tea
// names as a character, such as a keypad digit, is left out: the character
// is its name. So are the keys edit never receives: a modifier key or a
// lock key pressed alone, which only a kitty flag edit doesn't ask for
// reports, and find and select, which Bubble Tea never decodes. A name two
// keys share, as enter does with keypad enter, keeps the first code, since
// Bubble Tea writes both the same.
var specials = func() map[string]rune {
	names := map[string]rune{}
	add := func(c rune) {
		if n := (tea.Key{Code: c}).Keystroke(); utf8.RuneCountInString(n) > 1 {
			if _, ok := names[n]; !ok {
				names[n] = c
			}
		}
	}
	for _, c := range []rune{tea.KeyEnter, tea.KeyTab, tea.KeyBackspace, tea.KeyEscape, tea.KeySpace} {
		add(c)
	}
	never := []rune{tea.KeyCapsLock, tea.KeyNumLock, tea.KeyScrollLock, tea.KeyFind, tea.KeySelect}
	for c := tea.KeyUp; c < tea.KeyLeftShift; c++ {
		if !slices.Contains(never, c) {
			add(c)
		}
	}
	return names
}()

// keysOf splits binding, as the config writes it, into its keys: one key,
// or a sequence of keys with a space between each two. A space is part of a
// key when nothing but modifiers come before it in the key, so " " and
// "alt+ " are the space bar.
func keysOf(binding string) []string {
	var keys []string
	start := 0
	for i := range len(binding) {
		if binding[i] == ' ' && !onlyModifiers(binding[start:i]) {
			keys = append(keys, binding[start:i])
			start = i + 1
		}
	}
	return append(keys, binding[start:])
}

// onlyModifiers reports whether s is nothing but modifiers, such as "" or
// "ctrl+alt+".
func onlyModifiers(s string) bool {
	for s != "" {
		before := s
		for _, m := range modifiers {
			s = strings.TrimPrefix(s, m.prefix)
		}
		if s == before {
			return false
		}
	}
	return true
}

// spellBinding spells binding as Keys returns it: each of its keys as spell
// spells it, with one space between them. Its error names the key that is
// no key, and the sequence it is in.
func spellBinding(binding string) (string, error) {
	keys := keysOf(binding)
	for i, k := range keys {
		s, ok := spell(k)
		if !ok {
			return "", fmt.Errorf("unknown key %s", quoteKey(k, binding))
		}
		keys[i] = s
	}
	return strings.Join(keys, " "), nil
}

// quoteKey quotes key, one of the keys of binding, for a problem: with the
// sequence it is in, when binding is one.
func quoteKey(key, binding string) string {
	if key == binding {
		return strconv.Quote(key)
	}
	return fmt.Sprintf("%q in %q", key, binding)
}

// clean spells bindings as spellBinding does and drops the repeats, keeping
// the first of each. A binding with a key that is no key stays as it is
// written, for New to report.
func clean(bindings []string) []string {
	var out []string
	for _, b := range bindings {
		if s, err := spellBinding(b); err == nil {
			b = s
		}
		if !slices.Contains(out, b) {
			out = append(out, b)
		}
	}
	return out
}

// typesText reports whether key, spelled as spell spells it, types text: a
// single character, or the space bar, with shift or without.
func typesText(key string) bool {
	return key == "space" || key == "shift+space" || utf8.RuneCountInString(key) == 1
}
