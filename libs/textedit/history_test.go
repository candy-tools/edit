package textedit

import (
	"math"
	"slices"
	"strings"
	"testing"
)

// typeIn types text into b a character at a time, as keys type it.
func typeIn(b *Buffer, text string) {
	for _, r := range text {
		b.Insert(string(r))
	}
}

// undoes undoes the steps of b one at a time, failing the test unless the
// text after each undo is the next of want.
func undoes(t *testing.T, b *Buffer, want ...string) {
	t.Helper()
	for i, w := range want {
		b.Undo()
		if got := b.String(); got != w {
			t.Fatalf("after undo %d the text is %q, want %q", i+1, got, w)
		}
	}
}

// checkHeld fails the test unless held is the bytes of text the steps of
// both of b's lists hold.
func checkHeld(t *testing.T, b *Buffer) {
	t.Helper()
	n := 0
	for _, s := range slices.Concat(b.history.undo, b.history.redo) {
		n += s.size()
	}
	if n != b.history.held {
		t.Fatalf("held is %d, the steps hold %d bytes", b.history.held, n)
	}
}

func TestUndoTakesBackTypingAWordAtATime(t *testing.T) {
	b := New("", 4)
	typeIn(b, "hello world\n")
	undoes(t, b, "hello ", "", "")
	for i, want := range []string{"hello ", "hello world\n", "hello world\n"} {
		b.Redo()
		if got := b.String(); got != want {
			t.Fatalf("after redo %d the text is %q, want %q", i+1, got, want)
		}
	}
}

func TestTypedWhiteSpaceJoinsTheWordBefore(t *testing.T) {
	for _, c := range []struct {
		typed string
		steps []string
	}{
		{"a\tb", []string{"a\t", "b"}},
		{"a  b", []string{"a  ", "b"}},
		{"x\n  y", []string{"x\n  ", "y"}},
		{"  x", []string{"  ", "x"}},
		{`fmt.Println("x")`, []string{`fmt.Println("x")`}},
		{"日本語 テキスト", []string{"日本語 ", "テキスト"}},
	} {
		b := New("", 4)
		typeIn(b, c.typed)
		text := c.typed
		for i := len(c.steps) - 1; i >= 0; i-- {
			b.Undo()
			text = strings.TrimSuffix(text, c.steps[i])
			if got := b.String(); got != text {
				t.Fatalf("typed %q, undoing %q leaves %q, want %q", c.typed, c.steps[i], got, text)
			}
		}
	}
}

// Review focus: a cluster an input method typed in one key press is typing,
// not a paste.
func TestAClusterAnInputMethodTypedIsTyping(t *testing.T) {
	b := New("", 4)
	typeIn(b, "ok")
	b.Insert("👍🏽")
	b.Insert("🇪🇸")
	typeIn(b, " ")
	undoes(t, b, "")
}

func TestBackspaceInARowIsOneStep(t *testing.T) {
	b := New("ab\ncd", 4)
	b.Bottom()
	for range 3 { // d, c, and the line break
		b.DeleteLeft()
	}
	if got := b.String(); got != "ab" {
		t.Fatalf("after three backspaces the text is %q, want ab", got)
	}
	undoes(t, b, "ab\ncd")
	if line, off := b.Cursor(); line != 1 || off != 2 {
		t.Errorf("the cursor is at %d:%d, want back at 1:2", line, off)
	}
}

func TestDeleteInARowIsOneStep(t *testing.T) {
	b := New("ab\ncd", 4)
	b.Right()
	for range 3 { // b, the line break, and c
		b.DeleteRight()
	}
	if got := b.String(); got != "ad" {
		t.Fatalf("after three deletes the text is %q, want ad", got)
	}
	undoes(t, b, "ab\ncd")
	if line, off := b.Cursor(); line != 0 || off != 1 {
		t.Errorf("the cursor is at %d:%d, want back at 0:1", line, off)
	}
}

func TestAPasteIsAStepOfItsOwn(t *testing.T) {
	b := New("", 4)
	typeIn(b, "a")
	b.Insert("pasted")
	typeIn(b, "b")
	undoes(t, b, "apasted", "a", "")
}

func TestEachDeleteLineIsAStep(t *testing.T) {
	b := New("1\n2\n3\n", 4)
	b.DeleteLine()
	b.DeleteLine()
	undoes(t, b, "2\n3\n", "1\n2\n3\n")
}

func TestAMoveEndsAStep(t *testing.T) {
	b := New("", 4)
	typeIn(b, "ab")
	b.Left()
	typeIn(b, "c")
	undoes(t, b, "ab", "")

	b = New("x\ny", 4)
	typeIn(b, "a")
	b.Down() // a vertical move
	typeIn(b, "b")
	undoes(t, b, "ax\ny", "x\ny")
}

func TestAnEditOfAnotherKindEndsAStep(t *testing.T) {
	b := New("", 4)
	typeIn(b, "ab")
	b.DeleteLeft()
	typeIn(b, "c")
	undoes(t, b, "a", "ab", "")
}

// Review focus: typing a ZWJ between two emoji joins them into one cluster
// and snaps the cursor past both; the next character goes in there, apart
// from the ZWJ, and starts a step of its own.
func TestAJoinedClusterKeepsTheNextCharacterOut(t *testing.T) {
	b := New("👨👩", 4)
	b.Right()
	b.Insert("\u200d")
	typeIn(b, "x")
	undoes(t, b, "👨\u200d👩", "👨👩")
}

// Review focus: a deletion that joins the cells around the cursor into one
// cluster snaps the cursor past it, so the next deletion is elsewhere, and
// a step of its own.
func TestADeletionThatJoinsClustersEndsTheStep(t *testing.T) {
	b := New("👨\u200dx👩", 4) // a man and a joiner, x, a woman
	b.Right()
	b.Right()      // after x
	b.DeleteLeft() // x: the joiner joins man and woman, the cursor snaps past them
	b.DeleteLeft() // the joined cluster
	undoes(t, b, "👨\u200d👩", "👨\u200dx👩")

	b = New("👨\u200dx👩y", 4)
	b.Right()       // before x
	b.DeleteRight() // x: the cursor snaps past the joined cluster
	b.DeleteRight() // y
	undoes(t, b, "👨\u200d👩y", "👨\u200dx👩y")
}

func TestUndoAndRedoPutTheCursorBack(t *testing.T) {
	b := New("one\ntwo\nthree", 4)
	b.Down()
	b.LineEnd()
	typeIn(b, "!")
	b.Top()
	b.Undo()
	if line, off := b.Cursor(); line != 1 || off != 3 {
		t.Errorf("after the undo the cursor is at %d:%d, want 1:3, where the typing began", line, off)
	}
	b.Bottom()
	b.Redo()
	if line, off := b.Cursor(); line != 1 || off != 4 {
		t.Errorf("after the redo the cursor is at %d:%d, want 1:4, where the typing ended", line, off)
	}
	b.Top()
	b.Down()
	b.Right()
	b.Right()
	b.DeleteLine()
	b.Bottom()
	b.Undo()
	if line, off := b.Cursor(); b.String() != "one\ntwo!\nthree" || line != 1 || off != 2 {
		t.Errorf("after undoing a deleted line: %q, cursor at %d:%d; want the line back, the cursor at 1:2", b.String(), line, off)
	}
}

// Review focus: an edit after an undo drops what could be redone.
func TestAnEditDropsWhatCouldBeRedone(t *testing.T) {
	b := New("", 4)
	typeIn(b, "ab")
	b.Undo()
	typeIn(b, "c")
	b.Redo()
	if got := b.String(); got != "c" {
		t.Errorf("redo after an edit gives %q, want c: nothing to redo", got)
	}
}

func TestRedoWorksAfterTheCursorMoved(t *testing.T) {
	b := New("xy", 4)
	b.LineEnd()
	typeIn(b, "ab")
	b.Undo()
	b.Top()
	b.Redo()
	if line, off := b.Cursor(); b.String() != "xyab" || line != 0 || off != 4 {
		t.Errorf("redo after a move: %q, cursor at %d:%d; want xyab, the cursor at 0:4", b.String(), line, off)
	}
}

func TestNothingToUndoOrRedoChangesNothing(t *testing.T) {
	b := sized("ab", 80, 5)
	changes := record(b)
	b.Undo()
	b.Redo()
	if b.Version() != 0 || len(*changes) != 0 {
		t.Errorf("undo and redo of nothing: version %d, changes %+v; want 0 and none", b.Version(), *changes)
	}
	typeIn(b, "x")
	b.Redo()
	if b.Version() != 1 || len(*changes) != 1 {
		t.Errorf("redo with nothing undone: version %d, %d changes; want 1 and 1", b.Version(), len(*changes))
	}
}

func TestUndoMovesTheWindowToTheCursor(t *testing.T) {
	b := sized(numbered(100), 80, 5)
	typeIn(b, "x")
	b.Bottom()
	b.Undo()
	if from, _ := b.Visible(); from != 0 {
		t.Errorf("after undoing an edit on line 0 the window starts at line %d", from)
	}
}

// Review focus: a "\r" a line holds comes back as itself after an undo,
// never as a line break.
func TestACarriageReturnInALineComesBack(t *testing.T) {
	for _, text := range []string{"a\r\nb\nc", "a\r\r\nb"} {
		b := New(text, 4)
		b.LineEnd()
		b.DeleteLeft() // the "\r"
		b.Undo()
		if got := b.String(); got != text {
			t.Errorf("undoing a deleted \\r of %q gives %q", text, got)
		}
		b.DeleteLine()
		b.Undo()
		if got := b.String(); got != text {
			t.Errorf("undoing the deleted first line of %q gives %q", text, got)
		}
	}
}

func TestTheHistoryKeepsItsStepLimit(t *testing.T) {
	b := New("", 4)
	b.LimitHistory(3, 1<<20)
	typeIn(b, "a b c d e") // a step a word: "a ", "b ", "c ", "d ", "e"
	for range 5 {
		b.Undo()
	}
	if got := b.String(); got != "a b " {
		t.Errorf("undoing everything leaves %q, want a b : the two oldest steps dropped", got)
	}
}

func TestTheHistoryDropsTheOldestStepsOverItsSize(t *testing.T) {
	b := New("", 4)
	b.LimitHistory(100, 10)
	b.Insert("12345")
	b.Insert("67890") // 10 bytes held: both kept
	b.Insert("abc")   // 13: the oldest goes
	for range 3 {
		b.Undo()
	}
	if got := b.String(); got != "12345" {
		t.Errorf("undoing everything leaves %q, want 12345: the oldest paste dropped", got)
	}
}

func TestAStepLargerThanTheSizeIsKept(t *testing.T) {
	b := New("", 4)
	b.LimitHistory(100, 4)
	b.Insert("123456")
	undoes(t, b, "")

	b = New("", 4)
	b.LimitHistory(100, 3)
	typeIn(b, "abcdef") // an open step that grows past the size
	undoes(t, b, "")
}

func TestLimitsBelowOneCountAsOne(t *testing.T) {
	b := New("", 4)
	b.LimitHistory(0, -1)
	typeIn(b, "a b")
	undoes(t, b, "a ", "a ")
}

func TestHeldCountsTheStepsText(t *testing.T) {
	b := New("one\ntwo", 4)
	for _, op := range []func(){
		func() { typeIn(b, "ab c") },
		b.Undo, b.Redo, b.Undo,
		func() { b.Insert("pasted") },
		b.DeleteLeft, b.DeleteLeft, b.Bottom, b.DeleteLine, b.Undo, b.Undo, b.Redo,
	} {
		op()
		checkHeld(t, b)
	}
}

// Review focus: whatever the text and the edits, undoing every step gives
// back the text New was given, byte for byte, and redoing every step the
// text after the edits.
func FuzzUndoAll(f *testing.F) {
	f.Add("a\r\nb\nc", []byte{5, 13, 15, 10, 11, 12, 14})
	f.Add("a\r\r\nb", []byte{5, 13, 15, 3, 15, 10})
	f.Add("e\U00000301\xff\t日👨👩\r\nx", []byte{1, 1, 1, 12, 5, 0, 13, 11, 7, 6, 14, 15})
	f.Add("ab\ncd", []byte{0, 1, 2, 3, 9, 10, 13, 14, 15, 16})
	f.Fuzz(func(t *testing.T, text string, ops []byte) {
		b := sized(text, 7, 3)
		b.LimitHistory(len(ops)+1, math.MaxInt)
		for _, op := range ops {
			edit(b, op%17) // the moves, edits and resizes, no undo or redo
		}
		final := b.String()
		for range ops {
			b.Undo()
		}
		if got := b.String(); got != text {
			t.Fatalf("undoing every step gives %q, want %q", got, text)
		}
		for range ops {
			b.Redo()
		}
		if got := b.String(); got != final {
			t.Fatalf("redoing every step gives %q, want %q", got, final)
		}
	})
}
