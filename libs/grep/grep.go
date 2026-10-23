// Package grep finds text in other text: in one text, or in every text file
// of a folder tree, which it walks in order, one file at a time, reporting
// each file's matches as it goes, until it is cancelled or has found enough.
// A query is found as it is, with smart case: it ignores case unless it
// holds a capital letter.
//
// It is app-agnostic and depends only on the standard library; a depguard
// rule in .golangci.yaml keeps it from importing the app.
package grep

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// longLine is the longest line a Match holds whole; of a longer one it
	// holds the match and around bytes on each side of it.
	longLine = 1 << 10
	around   = 256
	// sniffLen is how many leading bytes of a file are searched for a NUL
	// byte, which marks it as binary, as git does.
	sniffLen = 8000
)

// A Query is text to find, as it is, with smart case: it ignores case
// unless it holds an upper-case letter.
type Query struct {
	text []byte // the bytes to find, lowered when fold is set; nil for a query that finds nothing
	fold bool   // the query holds no upper-case letter: case is ignored
}

// NewQuery returns the Query that finds text. A query holding "\n" or "\r",
// which no match can span, finds nothing, and neither does "".
func NewQuery(text string) Query {
	if text == "" || strings.ContainsAny(text, "\r\n") {
		return Query{}
	}
	if strings.ContainsFunc(text, unicode.IsUpper) {
		return Query{text: []byte(text)}
	}
	return Query{text: lower(nil, []byte(text)), fold: true}
}

// lower appends text to dst with each character put as its small letter
// when that takes as many bytes, and every other byte as it is, so every
// offset in what it appends is the same offset in text.
func lower(dst, text []byte) []byte {
	for i := 0; i < len(text); {
		if c := text[i]; c < utf8.RuneSelf {
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			dst = append(dst, c)
			i++
			continue
		}
		r, n := utf8.DecodeRune(text[i:])
		if l := unicode.ToLower(r); r != utf8.RuneError && utf8.RuneLen(l) == n {
			dst = utf8.AppendRune(dst, l)
		} else {
			dst = append(dst, text[i:i+n]...)
		}
		i += n
	}
	return dst
}

// A Match is one place a Query was found.
type Match struct {
	Path   string // the file, slash-separated, from the root of the tree; "" in a text searched on its own
	Line   int    // the match's line, from 0
	Column int    // the byte offset of the match in its line
	Text   string // the line, without its line break, or the part of a long line around the match
	Start  int    // the match is bytes Start to End-1 of Text
	End    int
}

// Find returns the places q is found in text, in order: at most limit of
// them, or all of them for a limit of 0 or less, and whether there were
// more. It never writes to text.
func Find(text []byte, q Query, limit int) (matches []Match, more bool) {
	var s scanner
	s.scan(text, q, func(m Match) bool {
		if limit > 0 && len(matches) == limit {
			more = true
			return false
		}
		matches = append(matches, m)
		return true
	})
	return matches, more
}

// scanner finds a Query in texts, one after another, lowering each into a
// buffer it reuses.
type scanner struct{ lowered []byte }

// line is the line the last match was on: its number, where it starts and
// ends, its line break left out, and its copy, which the matches on it
// share, when it is no longer than longLine.
type line struct {
	number     int
	start, end int
	text       string
}

// scan calls emit with each place q is found in text, in order, until emit
// returns false. Matches start where the one before ends or later, so they
// never overlap.
func (s *scanner) scan(text []byte, q Query, emit func(Match) bool) {
	if q.text == nil {
		return
	}
	hay := text
	if q.fold {
		s.lowered = lower(s.lowered[:0], text)
		hay = s.lowered
	}
	at, counted := 0, 0 // where to look next, and how far the line breaks are counted
	cur := line{number: -1}
	n, start := 0, 0 // the line counted up to, and where it starts
	for {
		i := bytes.Index(hay[at:], q.text)
		if i < 0 {
			return
		}
		from, to := at+i, at+i+len(q.text)
		if breaks := text[counted:from]; bytes.IndexByte(breaks, '\n') >= 0 {
			n += bytes.Count(breaks, []byte{'\n'})
			start = counted + bytes.LastIndexByte(breaks, '\n') + 1
		}
		counted = from
		if cur.number != n {
			cur = lineAt(text, n, start)
		}
		if !emit(cur.match(text, from, to)) {
			return
		}
		at = to
	}
}

// lineAt returns line number n of text, which starts at offset start: its
// end, without a "\r" before its "\n", and its copy, when it is short.
func lineAt(text []byte, n, start int) line {
	end := len(text)
	if i := bytes.IndexByte(text[start:], '\n'); i >= 0 {
		end = start + i
		if end > start && text[end-1] == '\r' {
			end--
		}
	}
	l := line{number: n, start: start, end: end}
	if end-start <= longLine {
		l.text = string(text[start:end])
	}
	return l
}

// match returns the Match at bytes from to to of text, on l: Text is l's
// copy or, for a long line, a copy of the match and around bytes on each
// side, cut at character boundaries.
func (l line) match(text []byte, from, to int) Match {
	m := Match{Line: l.number, Column: from - l.start}
	if l.end-l.start <= longLine {
		m.Text, m.Start, m.End = l.text, from-l.start, to-l.start
		return m
	}
	lo, hi := max(from-around, l.start), min(to+around, l.end)
	for lo < from && !utf8.RuneStart(text[lo]) {
		lo++
	}
	for hi > to && hi < l.end && !utf8.RuneStart(text[hi]) {
		hi--
	}
	m.Text, m.Start, m.End = string(text[lo:hi]), from-lo, to-lo
	return m
}

// Options shape a Search.
type Options struct {
	Skip     func(path string, isDir bool) bool // leaves out each file and folder it reports, and all a folder holds; nil leaves out none
	Order    func(a, b fs.DirEntry) int         // the order of a folder's entries; nil is by name
	Contents map[string][]byte                  // texts searched in place of the files at these paths
	MaxSize  int64                              // leaves out files larger than this; 0 or less for no limit
	Limit    int                                // stops after this many matches; 0 or less for no limit
}

// errLimit ends a walk that found a match past the limit.
var errLimit = errors.New("grep: limit reached")

// Search searches the text files of fsys for q, one at a time, in a walk
// from the root that takes each folder's entries in opts.Order, a folder's
// before the entry after it, and calls found with the matches of each file
// that has any, in order. It leaves out what opts.Skip reports, every entry
// that is neither a regular file nor a folder, symlinks included, binary
// files, files over opts.MaxSize, and files and folders it can't read. It
// stops when ctx is done, returning ctx's error, or on finding a match past
// opts.Limit, reporting more, the matches past the limit left out;
// otherwise it returns once every file is searched. Besides a cancellation,
// it fails only when the root can't be read. found gets a new slice each
// time, which Search never touches again, and Search never writes to
// Contents.
func Search(ctx context.Context, fsys fs.FS, q Query, opts Options, found func([]Match)) (more bool, err error) {
	if q.text == nil {
		return false, nil
	}
	w := walk{ctx: ctx, fsys: fsys, q: q, opts: opts, found: found}
	entries, err := w.readDir(".")
	if err != nil {
		return false, err
	}
	switch err := w.folder(".", entries); {
	case errors.Is(err, errLimit):
		return true, nil
	case err != nil:
		return false, err
	}
	return false, nil
}

// walk is one Search: what it searches, and how far it has got.
type walk struct {
	ctx     context.Context
	fsys    fs.FS
	q       Query
	opts    Options
	found   func([]Match)
	n       int     // the matches found so far
	scanner scanner // lowers each file into one buffer
}

// readDir reads the folder dir, its entries in Order.
func (w *walk) readDir(dir string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(w.fsys, dir)
	if err != nil {
		return nil, err
	}
	if w.opts.Order != nil {
		slices.SortStableFunc(entries, w.opts.Order)
	}
	return entries, nil
}

// folder searches entries, the folder dir's, in order: a file as it comes,
// a folder and all it holds before the next entry.
func (w *walk) folder(dir string, entries []fs.DirEntry) error {
	for _, e := range entries {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		p := path.Join(dir, e.Name())
		if w.opts.Skip != nil && w.opts.Skip(p, e.IsDir()) {
			continue
		}
		switch {
		case e.IsDir():
			sub, err := w.readDir(p)
			if err != nil {
				continue // a folder it can't read is left out
			}
			if err := w.folder(p, sub); err != nil {
				return err
			}
		case e.Type().IsRegular():
			if err := w.file(p, e); err != nil {
				return err
			}
		}
	}
	return nil
}

// file searches the file at p, whose entry is e, and hands its matches to
// found. It returns errLimit once a match past the limit turns up.
func (w *walk) file(p string, e fs.DirEntry) error {
	text, ok := w.opts.Contents[p]
	if !ok {
		if text, ok = w.read(p, e); !ok {
			return nil
		}
	}
	var matches []Match
	limited := false
	w.scanner.scan(text, w.q, func(m Match) bool {
		if w.opts.Limit > 0 && w.n == w.opts.Limit {
			limited = true
			return false
		}
		m.Path = p
		matches = append(matches, m)
		w.n++
		return true
	})
	if len(matches) > 0 {
		w.found(matches)
	}
	if limited {
		return errLimit
	}
	return nil
}

// read reads the file at p, whose entry is e, for a search, and reports
// false when it is left out: over MaxSize, binary, or unreadable. Its size
// comes from its entry first, so a file too large is never read; then it
// reads at most MaxSize+1 bytes, in case the file grew.
func (w *walk) read(p string, e fs.DirEntry) ([]byte, bool) {
	limit := int64(-1)
	if w.opts.MaxSize > 0 {
		info, err := e.Info()
		if err != nil || info.Size() > w.opts.MaxSize {
			return nil, false
		}
		limit = w.opts.MaxSize
	}
	f, err := w.fsys.Open(p)
	if err != nil {
		return nil, false
	}
	var r io.Reader = f
	if limit >= 0 {
		r = io.LimitReader(f, limit+1)
	}
	text, err := io.ReadAll(r)
	err = errors.Join(err, f.Close())
	switch {
	case err != nil, limit >= 0 && int64(len(text)) > limit:
		return nil, false
	case bytes.IndexByte(text[:min(len(text), sniffLen)], 0) >= 0:
		return nil, false
	}
	return text, true
}
