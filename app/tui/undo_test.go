package tui

import "testing"

// docText returns the text of m's active tab.
func docText(m model) string { return m.activeDocument().text.String() }

func TestUndoAndRedoTheActiveTab(t *testing.T) {
	m := press(newTestModel(), "down", "enter", "x", "y") // go.mod open, xy typed
	if m = press(m, "ctrl+z"); docText(m) != goModText {
		t.Errorf("after ctrl+z go.mod reads %q, want it as it was", docText(m))
	}
	if m = press(m, "ctrl+shift+z"); docText(m) != "xy"+goModText {
		t.Errorf("after ctrl+shift+z go.mod reads %q, want xy back", docText(m))
	}
}

// Review focus: on a terminal that sends ctrl+shift+z as ctrl+z, redo still
// has a key: ctrl+y.
func TestCtrlYRedoesToo(t *testing.T) {
	m := press(newTestModel(), "down", "enter", "x", "ctrl+z", "ctrl+y")
	if got := docText(m); got != "x"+goModText {
		t.Errorf("after ctrl+z and ctrl+y go.mod reads %q, want x back", got)
	}
}

// Review focus: each tab has a history of its own: an undo in one never
// touches another.
func TestEachTabHasItsOwnHistory(t *testing.T) {
	m := press(newTestModel(), "down", "enter", "a") // go.mod: a typed
	m = press(m, "shift+tab", "down", "enter", "b")  // README.md: b typed
	m = press(m, "ctrl+z")                           // README.md's b undone
	tabs, _ := m.tabs.Tabs()
	if got := tabs[0].Value.text.String(); got != "a"+goModText {
		t.Errorf("an undo in README.md left go.mod %q, want its a kept", got)
	}
	if got := tabs[1].Value.text.String(); got != "# project\n" {
		t.Errorf("README.md reads %q after its undo, want it as it was", got)
	}
	m = press(m, "ctrl+pgup", "ctrl+z") // go.mod's a undone
	if got := docText(m); activeKey(m) != "go.mod" || got != goModText {
		t.Errorf("%s reads %q after its undo, want go.mod as it was", activeKey(m), got)
	}
}

// Review focus: closing a tab drops its history, so the file opened again
// has nothing to undo.
func TestAReopenedFileHasNothingToUndo(t *testing.T) {
	m, _ := newEditModel(testFS)
	m = press(m, "down", "enter", "a", "ctrl+w") // go.mod edited, saved and closed
	m = press(m, "enter", "ctrl+z")              // opened again, then ctrl+z
	if v := m.activeDocument().text.Version(); activeKey(m) != "go.mod" || v != 0 {
		t.Errorf("%s, version %d after ctrl+z: want go.mod untouched, version 0", activeKey(m), v)
	}
}

// Review focus: an undo is an edit like any other, so it is saved.
func TestAnUndoIsSaved(t *testing.T) {
	m, d := typeInGoMod(t) // x typed, unsaved
	m = press(m, "ctrl+z")
	tick(m)
	if got, _ := d.file("go.mod"); got != goModText {
		t.Errorf("go.mod on disk = %q, want the undo saved", got)
	}
}

// Review focus: ctrl+z on a tab just opened has nothing to undo, and so
// nothing to save.
func TestUndoOnAFreshTabWritesNothing(t *testing.T) {
	m, d := newEditModel(testFS)
	m = press(m, "down", "enter", "ctrl+z")
	if m.ticking || len(d.log()) != 0 {
		t.Errorf("ctrl+z on a fresh tab: ticking %t, writes %q; want no tick and no write", m.ticking, d.log())
	}
}

// A tab keeps its last maxUndoSteps steps, and drops the older ones.
func TestATabKeepsItsLastThousandSteps(t *testing.T) {
	m := press(newTestModel(), "down", "enter")
	for range maxUndoSteps + 1 {
		m = press(m, "a", "space") // a step a word
	}
	for range maxUndoSteps + 1 {
		m = press(m, "ctrl+z")
	}
	if got := docText(m); got != "a "+goModText {
		t.Errorf("after undoing everything go.mod starts %q, want the oldest step kept: a ", got[:min(len(got), 8)])
	}
}
