package tui

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/candy-tools/edit/libs/gitignore"
	"github.com/candy-tools/edit/libs/grep"
	"github.com/candy-tools/edit/libs/keymap"
	"github.com/candy-tools/edit/libs/picker"
)

const (
	// fileMatches is how many matches a file's search lists at most: a
	// short query can match millions of times in a file, and each row holds
	// some of its text.
	fileMatches = 10000
	// foundRoom is how many messages a search can send before the UI takes
	// them, so it runs ahead of the UI, and a waiter can merge what is ready.
	foundRoom = 64
	// searchPause is how long the workspace search waits after a key that
	// changed its query, so typing a word doesn't walk the workspace once
	// for each letter.
	searchPause = 100 * time.Millisecond
	// workspaceMatches is how many matches the workspace search lists at
	// most.
	workspaceMatches = 1000
)

// searchPopup is the open search popup: what it searches, its query and
// matches, and how the search for its query is going.
type searchPopup struct {
	doc       *document                  // the file it searches; nil for the workspace
	picker    *picker.Picker[grep.Match] // the query and the matches
	found     <-chan searchFoundMsg      // the running search's results; nil when none runs
	cancel    context.CancelFunc         // stops the running search; nil when none runs
	wait      bool                       // the typing pause is due, at the end of the Update
	start     bool                       // the search's start is due, at the end of the Update
	running   bool                       // a search runs, or is due
	fresh     bool                       // the next matches are the query's first: they replace the rows
	goPending bool                       // enter was pressed while the rows were the previous query's: go to the match once the query's own results land
	files     int                        // the files with matches so far
	matches   int                        // the matches so far
	more      bool                       // the search stopped at its limit
	err       error                      // why the workspace couldn't be searched
}

// searchFoundMsg delivers results of search n: matches of one or more
// files, in order, and, once the search is over, done, with more when it
// stopped at its limit and err when the workspace couldn't be read.
type searchFoundMsg struct {
	n       int
	matches []grep.Match
	files   int
	done    bool
	more    bool
	err     error
}

// searchDueMsg ends the typing pause of search n: it may start.
type searchDueMsg struct{ n int }

// searchTimer returns the command that ends search n's typing pause,
// searchPause from now.
func searchTimer(n int) tea.Cmd {
	return tea.Tick(searchPause, func(time.Time) tea.Msg { return searchDueMsg{n} })
}

// searchMsg takes a message of the popup's search: the end of a typing
// pause, which makes the search's start due unless its query changed or
// the popup closed since, or results, which searchFound takes into the
// list. It returns the model, gone to the match when an enter was waiting
// for the results, and the command that waits for the next ones.
func (m model) searchMsg(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case searchDueMsg:
		if p := m.search; p != nil && msg.n == m.searches {
			p.start = true
		}
	case searchFoundMsg:
		return m.searchFound(msg)
	}
	return m, nil
}

// searchEdits are what the popup's actions do to its picker: move in the
// list, or move and edit in the query.
var searchEdits = map[keymap.ID]func(*picker.Picker[grep.Match]){
	searchUp:          (*picker.Picker[grep.Match]).Up,
	searchDown:        (*picker.Picker[grep.Match]).Down,
	searchPageUp:      (*picker.Picker[grep.Match]).PageUp,
	searchPageDown:    (*picker.Picker[grep.Match]).PageDown,
	searchLeft:        (*picker.Picker[grep.Match]).Left,
	searchRight:       (*picker.Picker[grep.Match]).Right,
	searchLineStart:   (*picker.Picker[grep.Match]).Start,
	searchLineEnd:     (*picker.Picker[grep.Match]).End,
	searchDeleteLeft:  (*picker.Picker[grep.Match]).DeleteLeft,
	searchDeleteRight: (*picker.Picker[grep.Match]).DeleteRight,
}

// openSearch opens the search popup on what the focused pane searches: the
// workspace from the sidebar, the active tab's file from the editor, when
// it has text. It opens with the last query, selected, and searches for it
// at once.
func (m model) openSearch() model {
	var doc *document
	if m.focus == paneEditor {
		if doc = m.activeDocument(); doc == nil || doc.text == nil {
			return m
		}
	}
	m.search = &searchPopup{doc: doc, picker: picker.New[grep.Match]()}
	// The picker is sized before the query goes in: a query put into its
	// one-cell window starts scrolled to its end, and a wider one wouldn't
	// scroll it back.
	m.fitSearch()
	m.search.picker.Prefill(m.query)
	return m.queryChanged(false)
}

// queryChanged forgets the running search and the enter waiting for it,
// after the query changed or the popup opened, and makes the next step due:
// none for an empty query, which empties the list; the typing pause, for the
// workspace when pause is set; otherwise the search's start.
func (m model) queryChanged(pause bool) model {
	p := m.search
	m.searches++
	p.stop()
	p.goPending = false
	if p.picker.Query() == "" {
		p.picker.SetItems(nil)
		p.running, p.fresh, p.files, p.matches, p.more, p.err = false, false, 0, 0, false, nil
		return m
	}
	p.running, p.fresh = true, true
	if pause && p.doc == nil {
		p.wait = true
	} else {
		p.start = true
	}
	return m
}

// stop cancels the running search, if one runs, and what is due.
func (p *searchPopup) stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.found, p.cancel, p.wait, p.start = nil, nil, false, false
}

// searchAction applies an action of the popup: open goes to the match on
// the list's cursor row, or, while the rows are the previous query's, waits
// for the query's own matches and goes to the one its list starts on (see
// searchFound); close closes the popup; and the others move in the list or
// edit the query, which searches again when the query changed.
func (m model) searchAction(id keymap.ID) model {
	switch id {
	case searchOpen:
		if m.search.fresh {
			m.search.goPending = true // the rows are the previous query's
			return m
		}
		return m.goToMatch()
	case searchClose:
		return m.closeSearch()
	}
	p := m.search
	before := p.picker.Query()
	if edit, ok := searchEdits[id]; ok {
		edit(p.picker)
	}
	if p.picker.Query() != before {
		m = m.queryChanged(true)
	}
	return m
}

// searchType types text in the popup's query, a key's or pasted text, and
// searches again when the query changed.
func (m model) searchType(text string) model {
	before := m.search.picker.Query()
	m.search.picker.Type(text)
	if m.search.picker.Query() != before {
		m = m.queryChanged(true)
	}
	return m
}

// closeSearch closes the popup: it cancels the running search, raises the
// count of searches so nothing of the popup's lands later, and keeps the
// query for the next popup.
func (m model) closeSearch() model {
	m.search.stop()
	m.query = m.search.picker.Query()
	m.search = nil
	m.searches++
	return m
}

// goToMatch closes the popup and goes to the match on its list's cursor
// row: for the workspace, it opens the match's file in a tab, or switches
// to its tab; then it puts the cursor at the match's start, the line in the
// middle of the window when it was out of view, and focuses the editor.
// With no match in the list, it does nothing.
func (m model) goToMatch() model {
	match, ok := m.search.picker.Current()
	if !ok {
		return m
	}
	doc := m.search.doc
	m = m.closeSearch()
	if doc == nil {
		m = m.open(entry{name: path.Base(match.Path), path: match.Path})
		doc = m.activeDocument()
	}
	if doc.text != nil {
		doc.text.Goto(match.Line, match.Column)
	}
	m.focus = paneEditor
	return m
}

// searchCmd returns the command the popup's search needs at the end of an
// Update: the typing pause, or the start of its search, when one is due.
func (m model) searchCmd() tea.Cmd {
	p := m.search
	switch {
	case p == nil:
		return nil
	case p.wait:
		p.wait = false
		return m.pause(m.searches)
	case p.start:
		p.start = false
		return m.startSearch()
	}
	return nil
}

// startSearch starts the search for the popup's query in the background,
// once any search still running is cancelled: it takes what the search
// reads now — the file's text, or the open tabs' texts — then searches on
// its own goroutine, which sends what it finds on the popup's channel, and
// returns the command that waits for it.
func (m model) startSearch() tea.Cmd {
	p := m.search
	p.stop()
	ctx, cancel := context.WithCancel(context.Background())
	found := make(chan searchFoundMsg, foundRoom)
	p.found, p.cancel = found, cancel
	q := grep.NewQuery(p.picker.Query())
	if p.doc != nil {
		go findInFile(ctx, found, m.searches, q, p.doc.source())
	} else {
		w := workspaceSearch{fsys: m.fsys, ignores: m.ignores, inside: m.inside, texts: m.openTexts()}
		go searchWorkspace(ctx, found, m.searches, q, w)
	}
	return waitFound(found)
}

// workspaceSearch is what the workspace search reads: the workspace, the
// repository its ignore files are read from and the workspace's folder in
// it, and the open tabs' texts.
type workspaceSearch struct {
	fsys    fs.FS
	ignores fs.FS
	inside  string
	texts   map[string][]byte
}

// searchWorkspace searches the workspace for q, on its own goroutine: it
// sends each file's matches of search n as they are found, then the last
// message, unless ctx is done, and closes found. It leaves out .git and
// what git ignores, and reads the open tabs' texts in place of their files.
func searchWorkspace(ctx context.Context, found chan<- searchFoundMsg, n int, q grep.Query, w workspaceSearch) {
	defer close(found)
	ignored := gitignore.New(w.ignores, w.inside)
	opts := grep.Options{
		Skip: func(name string, isDir bool) bool {
			return path.Base(name) == ".git" || ignored.Ignored(name, isDir)
		},
		Order:    compareDirEntries,
		Contents: w.texts,
		MaxSize:  maxDocumentSize,
		Limit:    workspaceMatches,
	}
	more, err := grep.Search(ctx, w.fsys, q, opts, func(matches []grep.Match) {
		deliver(ctx, found, searchFoundMsg{n: n, matches: matches, files: 1})
	})
	if ctx.Err() == nil {
		deliver(ctx, found, searchFoundMsg{n: n, done: true, more: more, err: err})
	}
}

// openTexts returns the text of each open tab that has one, by path: what
// the workspace search reads in place of those files. Each is its
// document's source, which stays as it is while edits go on.
func (m model) openTexts() map[string][]byte {
	texts := map[string][]byte{}
	tabs, _ := m.tabs.Tabs()
	for _, t := range tabs {
		if doc := t.Value; doc.text != nil {
			texts[doc.path] = doc.source()
		}
	}
	return texts
}

// repository returns where the workspace at root, an absolute path, reads
// its ignore files from: the root of the git repository it is in — the
// nearest folder at or above root that holds .git, a folder or a file —
// and root's path in it, slash-separated; or root itself and "." when it is
// in none.
func repository(root string) (dir, inside string) {
	for d := root; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			if rel, err := filepath.Rel(d, root); err == nil {
				return d, filepath.ToSlash(rel)
			}
		}
		if filepath.Dir(d) == d {
			return root, "."
		}
	}
}

// findInFile searches text, a file's, for q, on its own goroutine: it sends
// the matches of search n as its last message, unless ctx is done, and
// closes found.
func findInFile(ctx context.Context, found chan<- searchFoundMsg, n int, q grep.Query, text []byte) {
	defer close(found)
	matches, more := grep.Find(text, q, fileMatches)
	deliver(ctx, found, searchFoundMsg{n: n, matches: matches, files: min(len(matches), 1), done: true, more: more})
}

// deliver sends msg on found, unless ctx is done first: nobody waits for a
// cancelled search's results.
func deliver(ctx context.Context, found chan<- searchFoundMsg, msg searchFoundMsg) {
	select {
	case found <- msg:
	case <-ctx.Done():
	}
}

// waitFound returns the command that waits for a search's next results on
// found, then takes every message already sent after them, merged into
// one, so a fast search lands in a few Updates rather than one per file. A
// channel closed with nothing on it, a cancelled search's, delivers
// nothing. One waitFound is in flight per search: the start returns the
// first, and each message that isn't done the next, so the matches arrive,
// and append, in order.
func waitFound(found <-chan searchFoundMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-found
		if !ok {
			return nil
		}
		for !msg.done {
			select {
			case next, ok := <-found:
				if !ok {
					return msg
				}
				msg.matches = append(msg.matches, next.matches...)
				msg.files += next.files
				msg.done, msg.more, msg.err = next.done, next.more, next.err
			default:
				return msg
			}
		}
		return msg
	}
}

// searchFound takes results of the popup's search into its list: the first
// of a query, or its end with none, replace the rows, the list's cursor on
// the first match after the editor's for a file; later ones add to them.
// When an enter was waiting for the query's own results, it goes to the match
// the list starts on, which closes the popup, or, when the search ends with
// none, the popup stays. Otherwise it returns the command that waits for the
// next results, until the search is done. Results of another search are
// dropped.
func (m model) searchFound(msg searchFoundMsg) (model, tea.Cmd) {
	p := m.search
	if p == nil || msg.n != m.searches {
		return m, nil
	}
	if p.fresh {
		p.picker.SetItems(msg.matches)
		p.fresh, p.files, p.matches = false, 0, 0
		if p.doc != nil {
			line, offset := p.doc.text.Cursor()
			p.picker.Select(firstAfter(msg.matches, line, offset))
		}
	} else {
		p.picker.Append(msg.matches...)
	}
	p.files += msg.files
	p.matches += len(msg.matches)
	if p.goPending && (p.picker.Len() > 0 || msg.done) {
		p.goPending = false
		if p.picker.Len() > 0 {
			return m.goToMatch(), nil // goToMatch closes the popup, which cancels the search
		}
	}
	if !msg.done {
		return m, waitFound(p.found)
	}
	p.stop()
	p.running, p.more, p.err = false, msg.more, msg.err
	return m, nil
}

// firstAfter returns the index of the first of matches that starts after
// line and offset, the editor's cursor, or 0 when none does.
func firstAfter(matches []grep.Match, line, offset int) int {
	for i, m := range matches {
		if m.Line > line || m.Line == line && m.Column > offset {
			return i
		}
	}
	return 0
}

// fitSearch sizes the open popup's picker to the popup's box at the
// terminal's size: the query's window, what the first row leaves it, and
// the list's, the rows below.
func (m model) fitSearch() {
	if m.search == nil {
		return
	}
	_, _, width, height := searchBox(m.size())
	m.search.picker.SetSize(queryWidth(width), height-3)
}
