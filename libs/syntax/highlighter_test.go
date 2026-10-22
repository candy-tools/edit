package syntax

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

// fakeLanguage parses a text into a fakeTree holding it, fails with err, or
// panics.
type fakeLanguage struct {
	err    error
	panics bool
}

func (l *fakeLanguage) Name() string { return "fake" }

func (l *fakeLanguage) Parse(text []byte) (Tree, error) {
	if l.panics {
		panic("fake parser bug")
	}
	if l.err != nil {
		return nil, l.err
	}
	return &fakeTree{text: string(text)}, nil
}

// fakeTree is the text it was parsed from, the edits it followed and
// whether it was closed. Each line's one span names the text and covers the
// bytes asked for, so a test can tell which tree answered, and what it was
// asked; with panics set, Edit and Spans panic.
type fakeTree struct {
	text   string
	edits  []Edit
	closed bool
	panics bool
}

func (t *fakeTree) Edit(e Edit) {
	if t.panics {
		panic("fake tree bug")
	}
	t.edits = append(t.edits, e)
}

func (t *fakeTree) Spans(_ []byte, from, to, lo, hi int) [][]Span {
	if t.panics {
		panic("fake tree bug")
	}
	spans := make([][]Span, to-from)
	for i := range spans {
		spans[i] = []Span{{Start: lo, End: hi, Capture: t.text}}
	}
	return spans
}

func (t *fakeTree) Close() { t.closed = true }

// texts is a Job text function for text that counts its calls.
type texts struct {
	text  string
	calls int
}

func (s *texts) get() []byte {
	s.calls++
	return []byte(s.text)
}

// parsed returns a Highlighter for lang whose tree is up to date with
// version 0 of text.
func parsed(t *testing.T, lang Language, text string) *Highlighter {
	t.Helper()
	h := NewHighlighter(lang, 100)
	job := h.Job(0, len(text), (&texts{text: text}).get)
	if job == nil {
		t.Fatal("a new Highlighter gave no job")
	}
	h.Done(job())
	return h
}

// treeText returns the text h's tree was parsed from, "" with no tree.
func treeText(h *Highlighter) string {
	if s := h.Spans(noText, 0, 1, 0, 1)[0]; len(s) > 0 {
		return s[0].Capture
	}
	return ""
}

// noText is a text function for Spans, when the text doesn't matter.
func noText() []byte { return nil }

func TestJobOnlyWhileTheTreeIsStale(t *testing.T) {
	h := NewHighlighter(&fakeLanguage{}, 100)
	v0 := &texts{text: "v0"}
	job := h.Job(0, 2, v0.get)
	if job == nil || v0.calls != 1 {
		t.Fatalf("a new Highlighter: job %v, text read %d times; want a job, read once", job != nil, v0.calls)
	}
	if h.Job(0, 2, v0.get) != nil || h.Job(1, 2, v0.get) != nil || v0.calls != 1 {
		t.Error("while a job runs, Job gives none and doesn't read the text")
	}
	h.Done(job())
	if h.Job(0, 2, v0.get) != nil || v0.calls != 1 {
		t.Error("with the tree up to date, Job gives none and doesn't read the text")
	}
	if got := treeText(h); got != "v0" {
		t.Errorf("tree parsed from %q, want v0", got)
	}
	if h.Job(1, 2, (&texts{text: "v1"}).get) == nil {
		t.Error("a newer version should give a job")
	}
}

func TestAFailedParseKeepsTheTreeAndIsNotRetried(t *testing.T) {
	lang := &fakeLanguage{}
	h := parsed(t, lang, "v0")
	lang.err = errors.New("timed out")
	h.Done(h.Job(1, 2, (&texts{text: "v1"}).get)())
	if got := treeText(h); got != "v0" {
		t.Errorf("after a failed parse the tree is from %q, want v0's", got)
	}
	if h.Job(1, 2, (&texts{text: "v1"}).get) != nil {
		t.Error("the version that failed is not parsed again")
	}
	if h.Job(2, 2, (&texts{text: "v2"}).get) == nil {
		t.Error("the next version is parsed")
	}
}

func TestATextOverTheSizeCapHasNoColors(t *testing.T) {
	h := parsed(t, &fakeLanguage{}, "v0")
	tree := h.tree.(*fakeTree)
	big := &texts{text: "big"}
	if h.Job(1, 101, big.get) != nil || big.calls != 0 {
		t.Error("over the cap: no job, and the text isn't read")
	}
	if !tree.closed || treeText(h) != "" {
		t.Error("over the cap the tree is closed: no colors")
	}
	if h.Job(1, 101, big.get) != nil || h.Job(2, 150, big.get) != nil || big.calls != 0 {
		t.Error("still over the cap: no job")
	}
	if h.Job(3, 100, (&texts{text: "v3"}).get) == nil {
		t.Error("back under the cap: a job")
	}
}

func TestEditsDuringAJobAreReplayedOntoItsTree(t *testing.T) {
	h := parsed(t, &fakeLanguage{}, "v0")
	old := h.tree.(*fakeTree)
	before := Edit{StartByte: 9}
	h.Edit(before) // with no job running: the tree only
	job := h.Job(1, 2, (&texts{text: "v1"}).get)
	e1, e2 := Edit{StartByte: 1, NewEndByte: 2}, Edit{StartByte: 3, OldEndByte: 4}
	h.Edit(e1)
	h.Edit(e2)
	if want := []Edit{before, e1, e2}; !slices.Equal(old.edits, want) {
		t.Errorf("the tree followed %v at once, want %v", old.edits, want)
	}
	r := job()
	h.Done(r)
	if got := r.tree.(*fakeTree).edits; !slices.Equal(got, []Edit{e1, e2}) {
		t.Errorf("the job's tree followed %v, want the edits made while it ran, in order", got)
	}
	if !old.closed || treeText(h) != "v1" {
		t.Error("the job's tree replaces the old one, which is closed")
	}
}

func TestCloseFreesTheTrees(t *testing.T) {
	h := parsed(t, &fakeLanguage{}, "v0")
	old := h.tree.(*fakeTree)
	job := h.Job(1, 2, (&texts{text: "v1"}).get)
	h.Close()
	if !old.closed {
		t.Error("Close closes the tree")
	}
	r := job()
	h.Done(r)
	if !r.tree.(*fakeTree).closed {
		t.Error("a job done after Close frees its tree")
	}
	if h.Job(2, 2, (&texts{text: "v2"}).get) != nil {
		t.Error("after Close, no jobs")
	}
}

func TestSpansWithoutATree(t *testing.T) {
	text := &texts{text: "a\nb\nc"}
	if got := NewHighlighter(&fakeLanguage{}, 100).Spans(text.get, 3, 6, 0, 1); !reflect.DeepEqual(got, make([][]Span, 3)) || text.calls != 0 {
		t.Errorf("Spans = %v, the text read %d times; want three nil slices, the text not read", got, text.calls)
	}
}

func TestSpansAsksTheTreeForTheBytesOfTheWindow(t *testing.T) {
	h := parsed(t, &fakeLanguage{}, "v0")
	want := [][]Span{{{Start: 5, End: 9, Capture: "v0"}}, {{Start: 5, End: 9, Capture: "v0"}}}
	if got := h.Spans(noText, 2, 4, 5, 9); !reflect.DeepEqual(got, want) {
		t.Errorf("Spans = %v, want the tree's spans of bytes 5 to 9 of lines 2 and 3", got)
	}
}

// Review focus: the engine is young; a bug in it may cost a text its
// colors, never the program.
func TestAPanickingEngineCostsTheColorsOnly(t *testing.T) {
	h := NewHighlighter(&fakeLanguage{panics: true}, 100)
	h.Done(h.Job(0, 2, (&texts{text: "v0"}).get)())
	if got := treeText(h); got != "" {
		t.Errorf("a parse that panicked left a tree from %q", got)
	}
	for _, call := range []func(h *Highlighter){
		func(h *Highlighter) { h.Edit(Edit{}) },
		func(h *Highlighter) { h.Spans(noText, 0, 1, 0, 1) },
	} {
		h := parsed(t, &fakeLanguage{}, "v0")
		h.tree.(*fakeTree).panics = true
		call(h)
		if got := h.Spans(noText, 0, 2, 0, 1); !reflect.DeepEqual(got, make([][]Span, 2)) {
			t.Errorf("after a tree panicked, Spans = %v, want no colors", got)
		}
	}
}

// The job runs on its own goroutine while edits go on: run with -race.
func TestAJobRunsAlongsideEdits(t *testing.T) {
	h := parsed(t, &fakeLanguage{}, "v0")
	job := h.Job(1, 2, (&texts{text: "v1"}).get)
	done := make(chan Result)
	go func() { done <- job() }()
	for range 100 {
		h.Edit(Edit{StartByte: 1})
		h.Spans(noText, 0, 1, 0, 1)
	}
	h.Done(<-done)
	if got := len(h.tree.(*fakeTree).edits); got != 100 {
		t.Errorf("the job's tree followed %d edits, want 100", got)
	}
}

func TestCapturesHasNoDuplicates(t *testing.T) {
	c := Captures()
	slices.Sort(c)
	if len(slices.Compact(c)) != len(captures) {
		t.Errorf("Captures lists a name twice: %q", captures)
	}
}
