package syntax

import "fmt"

// Highlighter keeps the tree of one text being edited up to date: it hands
// out one background parse at a time, and replays onto each parse's tree the
// edits made while it ran. It is not safe for concurrent use; only the jobs
// it hands out run elsewhere.
//
// A panic in the engine costs the text its colors, never the program: a
// parse that panics fails, and a tree that panics is dropped.
type Highlighter struct {
	lang    Language
	maxSize int

	tree    Tree   // the newest tree, following the edits since; nil for none
	handled int    // the last version parsed, over maxSize or failed; -1 for none
	running bool   // a job is out
	edits   []Edit // the edits made while the job runs, for its tree
	closed  bool
}

// Result is what a parse job found, for Done.
type Result struct {
	version int
	tree    Tree
	err     error
}

// NewHighlighter returns a Highlighter for a text in lang, which colors the
// text while it is at most maxSize bytes long. It has no tree until the first
// job's result is done.
func NewHighlighter(lang Language, maxSize int) *Highlighter {
	return &Highlighter{lang: lang, maxSize: maxSize, handled: -1}
}

// Edit makes the tree follow an edit to the text: at once, and again on the
// result of the job running now.
func (h *Highlighter) Edit(e Edit) {
	if h.running {
		h.edits = append(h.edits, e)
	}
	if h.tree != nil && !safely(func() { h.tree.Edit(e) }) {
		h.tree = nil
	}
}

// Job returns the parse to run next for the text at version, which is size
// bytes long: a job when the tree is stale and none is running, nil
// otherwise. text gives that text; Job calls it only when it returns a job.
// The job may run on any goroutine; pass its Result to Done.
//
// The tree is up to date with the version it was parsed from; Job returns
// nil for that version, the last one over the size cap and the last one
// whose job failed, as well as while a job runs and after Close. A version
// over the size cap closes the tree: a text that large has no colors.
func (h *Highlighter) Job(version, size int, text func() []byte) func() Result {
	switch {
	case h.running, h.closed, version == h.handled:
		return nil
	case size > h.maxSize:
		h.drop()
		h.handled = version
		return nil
	}
	src, lang := text(), h.lang
	h.running, h.edits = true, nil
	return func() (r Result) {
		r.version = version
		defer func() {
			if p := recover(); p != nil {
				r.tree, r.err = nil, fmt.Errorf("parsing %s panicked: %v", lang.Name(), p)
			}
		}()
		r.tree, r.err = lang.Parse(src)
		return r
	}
}

// Done takes the result of a job. After Close it frees the result's tree.
// A failed parse leaves the tree as it was, and Job won't retry its version;
// otherwise the result's tree follows the edits made while the job ran and
// replaces the tree.
func (h *Highlighter) Done(r Result) {
	h.running = false
	edits := h.edits
	h.edits = nil
	switch {
	case h.closed:
		if r.tree != nil {
			safely(r.tree.Close)
		}
		return
	case r.err != nil:
	case safely(func() {
		for _, e := range edits {
			r.tree.Edit(e)
		}
	}):
		h.drop()
		h.tree = r.tree
	}
	h.handled = r.version
}

// Spans returns the spans of lines from to to-1, on bytes lo to hi-1 of
// each, as Tree.Spans; with no tree, a nil slice for each line. text gives
// the text as it is now; Spans calls it only when it has a tree.
func (h *Highlighter) Spans(text func() []byte, from, to, lo, hi int) [][]Span {
	if h.tree != nil {
		var spans [][]Span
		if safely(func() { spans = h.tree.Spans(text(), from, to, lo, hi) }) {
			return spans
		}
		h.tree = nil
	}
	return make([][]Span, to-from)
}

// Close frees the tree. A job still running frees its own in Done.
func (h *Highlighter) Close() {
	h.closed = true
	h.drop()
}

// drop closes the tree, if there is one.
func (h *Highlighter) drop() {
	if h.tree != nil {
		safely(h.tree.Close)
		h.tree = nil
	}
}

// safely calls f, a call into the engine, and reports whether it returned
// rather than panicked.
func safely(f func()) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	f()
	return true
}
