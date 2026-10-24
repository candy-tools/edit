package tui

import (
	"path"

	"github.com/candy-tools/trazo/libs/syntax"
	"github.com/candy-tools/trazo/libs/syntax/treesitter"
	"github.com/candy-tools/trazo/libs/textedit"
	tea "charm.land/bubbletea/v2"
)

const (
	// maxColorSize is the largest text the editor colors: 256 KB. A larger
	// one would color late, and its syntax tree take hundreds of MB.
	maxColorSize = 256 << 10
	// maxColorLine is the longest line the editor colors, in bytes, as VS
	// Code's editor.maxTokenizationLineLength: coloring a minified line
	// would make every redraw slow.
	maxColorLine = 20000
	// spanChunk is how many bytes of a line the editor asks the spans of at
	// a time, from the first one the window draws: about what a window shows
	// of a line, so a line, however long, costs a query or two a frame.
	spanChunk = 128
)

// parsedMsg delivers the result of a background parse of doc's text.
type parsedMsg struct {
	doc    *document
	result syntax.Result
}

// highlight gives d, a document a tab just opened, a Highlighter for its
// text's language, found by the base name of its path and its first line,
// and makes the text tell it of every edit. It does nothing for a note, a
// language trazo doesn't color, or a theme that colors nothing.
func (d *document) highlight(theme Theme) {
	if d.text == nil || !theme.colors() {
		return
	}
	lang, ok := treesitter.Detect(path.Base(d.path), d.text.LineText(0))
	if !ok {
		return
	}
	h := syntax.NewHighlighter(lang, maxColorSize)
	d.syntax = h
	d.text.OnChange(func(c textedit.Change) { h.Edit(syntaxEdit(c)) })
}

// syntaxEdit is c, a change to a text, as an edit of its syntax.
func syntaxEdit(c textedit.Change) syntax.Edit {
	point := func(p textedit.Position) syntax.Point { return syntax.Point{Line: p.Line, Column: p.Column} }
	return syntax.Edit{
		StartByte: c.Start.Offset, OldEndByte: c.OldEnd.Offset, NewEndByte: c.NewEnd.Offset,
		Start: point(c.Start), OldEnd: point(c.OldEnd), NewEnd: point(c.NewEnd),
	}
}

// parses asks the Highlighter of each open tab for its next parse and
// returns them as commands, each delivering a parsedMsg: what the end of
// every Update but a quit does, as it does for saves.
func (m model) parses() tea.Cmd {
	var cmds []tea.Cmd
	tabs, _ := m.tabs.Tabs()
	for _, t := range tabs {
		doc := t.Value
		if doc.syntax == nil {
			continue
		}
		if job := doc.syntax.Job(doc.text.Version(), doc.text.Size(), doc.source); job != nil {
			cmds = append(cmds, func() tea.Msg { return parsedMsg{doc: doc, result: job()} })
		}
	}
	return tea.Batch(cmds...)
}

// classifier returns the classifier that colors the window of d's text in
// theme: a cell takes the class of the capture of the span on it, or 0. It
// asks for a line's spans as the window draws the line, spanChunk bytes at a
// time from the first byte drawn, so a long line costs what the window shows
// of it. A line longer than maxColorLine stays plain: its spans are never
// asked for. With no Highlighter it is nil, and every cell plain.
func (d *document) classifier(theme Theme) func(i, off int) int {
	if d.syntax == nil {
		return nil
	}
	line, lo, hi := -1, 0, 0 // spans holds the spans of bytes lo to hi-1 of line
	var spans []syntax.Span
	return func(i, off int) int {
		if i != line || off < lo || off >= hi {
			line, lo, hi, spans = i, off, off+spanChunk, nil
			if len(d.text.LineText(i)) <= maxColorLine {
				spans = d.syntax.Spans(d.source, i, i+1, lo, hi)[0]
			}
		}
		for _, s := range spans {
			if s.Start <= off && off < s.End {
				return theme.class(s.Capture)
			}
		}
		return 0
	}
}
