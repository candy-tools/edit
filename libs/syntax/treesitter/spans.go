package treesitter

import (
	"math"
	"slices"
	"strings"

	"github.com/candy-tools/edit/libs/syntax"
	ts "github.com/odvcencio/gotreesitter"
)

// tree is a syntax.Tree: the syntax trees of a text, its own first, then
// those of the parts of it another language is injected into, all in the
// text's coordinates.
type tree struct {
	layers []layer
}

// layer is one syntax tree of a text, and the language whose query colors
// it.
type layer struct {
	lang     *language
	tree     *ts.Tree
	injected bool // an injection: its captures win over the host's
	owns     bool // an injection that owns its part of the text: the host's colors stop there
}

// Edit makes every layer follow the edit.
func (t *tree) Edit(e syntax.Edit) {
	edit := ts.InputEdit{
		StartByte:   u32(e.StartByte),
		OldEndByte:  u32(e.OldEndByte),
		NewEndByte:  u32(e.NewEndByte),
		StartPoint:  point(e.Start),
		OldEndPoint: point(e.OldEnd),
		NewEndPoint: point(e.NewEnd),
	}
	for _, l := range t.layers {
		l.tree.Edit(edit)
	}
}

// Close releases every layer.
func (t *tree) Close() {
	for _, l := range t.layers {
		l.tree.Release()
	}
	t.layers = nil
}

// capture is one capture of a highlight query on a node: the node's place,
// and what settles which of two captures that overlap colors their bytes.
type capture struct {
	name               string
	start, end         ts.Point
	startByte, endByte uint32
	injected           bool
	pattern            int
}

// loses reports whether c gives way to d where they overlap: a host's
// capture to an injection's, then the wider to the narrower, then, on the
// same bytes, the capture of the earlier pattern of a query to the later.
func (c capture) loses(d capture) bool {
	if c.injected != d.injected {
		return d.injected
	}
	if cw, dw := c.endByte-c.startByte, d.endByte-d.startByte; cw != dw {
		return cw > dw
	}
	return c.pattern < d.pattern
}

// Spans returns the spans of lines from to to-1 of text, on bytes lo to
// hi-1 of each.
func (t *tree) Spans(text []byte, from, to, lo, hi int) [][]syntax.Span {
	spans := make([][]syntax.Span, to-from)
	if lo < hi {
		for row := from; row < to; row++ {
			spans[row-from] = t.lineSpans(text, row, lo, hi)
		}
	}
	return spans
}

// lineSpans returns the spans of line row of text on bytes lo to hi-1. It
// queries the nodes on those bytes alone, so a long line costs what is
// asked of it.
func (t *tree) lineSpans(text []byte, row, lo, hi int) []syntax.Span {
	var caps []capture
	for _, l := range t.layers {
		caps = l.captures(text, row, lo, hi, caps)
	}
	slices.SortStableFunc(caps, func(c, d capture) int {
		switch {
		case c.loses(d):
			return -1
		case d.loses(c):
			return 1
		}
		return 0
	})
	segs := make([]segment, 0, len(caps))
	for rank, c := range caps {
		if int(c.start.Row) > row || int(c.end.Row) < row {
			continue // the capture of a match found on the line, lying on another
		}
		s := segment{start: 0, end: math.MaxInt, name: c.name, rank: rank}
		if row == int(c.start.Row) {
			s.start = int(c.start.Column)
		}
		if row == int(c.end.Row) {
			s.end = int(c.end.Column)
		}
		if s.start < s.end {
			segs = append(segs, s)
		}
	}
	return resolve(segs)
}

// captures appends to caps the captures of l's query on the nodes on bytes
// lo to hi-1 of line row of text. A layer that owns its part of the text
// captures it all as "none" first, below every capture of its own.
func (l layer) captures(text []byte, row, lo, hi int, caps []capture) []capture {
	root := l.tree.RootNode()
	if root == nil || int(root.EndPoint().Row) < row || int(root.StartPoint().Row) > row {
		return caps
	}
	if l.owns {
		caps = append(caps, capture{name: "none", start: root.StartPoint(), end: root.EndPoint(),
			startByte: root.StartByte(), endByte: root.EndByte(), injected: true, pattern: -1})
	}
	c := l.lang.query.Exec(root, l.lang.lang, text)
	c.SetPointRange(ts.Point{Row: u32(row), Column: u32(lo)}, ts.Point{Row: u32(row), Column: u32(hi)})
	for {
		m, ok := c.NextMatch()
		if !ok {
			return caps
		}
		for _, cp := range m.Captures {
			if cp.Node == nil || strings.HasPrefix(cp.Name, "_") {
				continue // a capture for a predicate only
			}
			caps = append(caps, capture{name: cp.Name, start: cp.Node.StartPoint(), end: cp.Node.EndPoint(),
				startByte: cp.Node.StartByte(), endByte: cp.Node.EndByte(), injected: l.injected, pattern: m.PatternIndex})
		}
	}
}

// segment is the part of a capture on one line: its bytes there, up to
// math.MaxInt when it carries on to the next line, its name, and its rank
// among the captures, the higher winning.
type segment struct {
	start, end int
	name       string
	rank       int
}

// resolve turns the segments of a line into its spans: each byte takes the
// name of the highest-ranked segment on it, and "none" colors nothing. The
// segments' bounds cut the line into intervals, and each segment marks the
// ones it covers: as all but a few captures are narrow, that takes time
// about linear in their number, which on a line of a JSON-lines file runs
// to thousands.
func resolve(segs []segment) []syntax.Span {
	bounds := make([]int, 0, 2*len(segs))
	for _, s := range segs {
		bounds = append(bounds, s.start, s.end)
	}
	slices.Sort(bounds)
	bounds = slices.Compact(bounds)
	names := make([]string, len(bounds)) // the name of the interval from bounds[i] to bounds[i+1]
	ranks := make([]int, len(bounds))    // the rank of the segment that named it, -1 for none
	for i := range ranks {
		ranks[i] = -1
	}
	for _, s := range segs {
		i, _ := slices.BinarySearch(bounds, s.start)
		for ; bounds[i] < s.end; i++ {
			if s.rank > ranks[i] {
				names[i], ranks[i] = s.name, s.rank
			}
		}
	}
	var spans []syntax.Span
	for i := 0; i+1 < len(bounds); i++ {
		start, end, name := bounds[i], bounds[i+1], names[i]
		switch n := len(spans); {
		case name == "" || name == "none":
		case n > 0 && spans[n-1].End == start && spans[n-1].Capture == name:
			spans[n-1].End = end
		default:
			spans = append(spans, syntax.Span{Start: start, End: end, Capture: name})
		}
	}
	return spans
}

// point is p as gotreesitter counts it.
func point(p syntax.Point) ts.Point {
	return ts.Point{Row: u32(p.Line), Column: u32(p.Column)}
}

// u32 is n as gotreesitter counts offsets: n itself, as no text edit colors
// comes near 4 GB.
func u32(n int) uint32 {
	return uint32(min(max(n, 0), math.MaxUint32))
}
