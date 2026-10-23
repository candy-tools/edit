package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

// saveInterval is how often the tabs' edits are written to disk in the
// background: an edit reaches the disk about this long after it is made.
const saveInterval = 500 * time.Millisecond

// writeFunc writes data to the file at path, slash-separated and relative to
// the workspace root.
type writeFunc func(path string, data []byte) error

// diskWriter is the writeFunc for the workspace directory root. It writes a
// file in place, truncated and rewritten, so the file keeps its inode, mode,
// owner and hard links, and a symlink is followed to its target, as os.DirFS
// followed it to read the file. A file that is gone is created again.
func diskWriter(root string) writeFunc {
	return func(path string, data []byte) error {
		// The mode only applies, less the umask, to a file that has to be
		// created again, which should get the mode any new file gets.
		return os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), data, 0o666) //nolint:gosec // G306: the user's own file, created again with the usual mode
	}
}

// errExiting is the error of a write refused because edit is exiting.
var errExiting = errors.New("edit is exiting")

// gate passes writes on to write until it closes, then refuses them. Run
// closes it on the way out, which waits for the writes in progress, so edit
// never exits in the middle of one, even when a panic leaves no model to
// save.
type gate struct {
	write  writeFunc
	mu     sync.RWMutex // held for reading by each write, and for writing by close
	closed bool
}

// pass writes through the gate: it is the writeFunc the gate hands out.
func (g *gate) pass(path string, data []byte) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.closed {
		return errExiting
	}
	return g.write(path, data)
}

// close refuses every later write, once the writes in progress are done.
func (g *gate) close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closed = true
}

// file writes one document's text back to disk, from the background or from
// Update: one write at a time, and never older text over newer. A background
// write and the save of a close or a quit can race for it, and whichever
// comes second writes newer text or nothing.
type file struct {
	path  string // slash-separated, relative to the workspace root
	write writeFunc

	mu      sync.Mutex // held for a whole write, and guards the fields below
	written int        // the newest version of the text written
	retired bool       // the document's tab has closed: nothing more is written
}

// save writes text, the document's version-th, unless this or a newer
// version is written already or the tab has closed. It waits for a write in
// progress first.
func (f *file) save(version int, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.retired || version <= f.written {
		return nil
	}
	if err := f.write(f.path, []byte(text)); err != nil {
		return err
	}
	f.written = version
	return nil
}

// retire stops every later write, once a write in progress is done: the
// document's tab has closed.
func (f *file) retire() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retired = true
}

// saveTickMsg starts a background write of each tab with unsaved edits.
type saveTickMsg struct{}

// savedMsg reports how a background write of doc's version-th text went.
type savedMsg struct {
	doc     *document
	version int
	err     error
}

// saveTick returns the command that delivers the next saveTickMsg,
// saveInterval from now. It makes a new command each time: a tea.Tick's
// timer starts when the command is made, not when it runs.
func saveTick() tea.Cmd {
	return tea.Tick(saveInterval, func(time.Time) tea.Msg { return saveTickMsg{} })
}

// saveDue reports whether a tick should be on its way: some open tab has
// unsaved edits and no write of its own running. The check is per tab, so a
// write that hangs, on a network mount say, doesn't stop the other tabs'
// saves.
func (m model) saveDue() bool {
	tabs, _ := m.tabs.Tabs()
	return slices.ContainsFunc(tabs, func(t openTab) bool { return t.Value.unsaved() && !t.Value.writing })
}

// scheduleSave returns the next tick when one is due and none is on its way.
// While any tab has unsaved edits, a tick is on its way or that tab's write
// is running, and the write's result schedules the next tick: so a failing
// write is retried every saveInterval, and nothing runs once all is saved.
func (m model) scheduleSave() (model, tea.Cmd) {
	if m.ticking || !m.saveDue() {
		return m, nil
	}
	m.ticking = true
	return m, m.tick()
}

// startWrites starts a background write of each tab with unsaved edits and
// no write running: what a tick does.
func (m model) startWrites() (model, tea.Cmd) {
	m.ticking = false
	var cmds []tea.Cmd
	tabs, _ := m.tabs.Tabs()
	for _, t := range tabs {
		if doc := t.Value; doc.unsaved() && !doc.writing {
			doc.writing = true
			cmds = append(cmds, writeCmd(doc))
		}
	}
	return m, tea.Batch(cmds...)
}

// writeCmd is a background write of doc's text as it is now. It touches
// nothing of doc but its file, so it is safe to run alongside Update.
func writeCmd(doc *document) tea.Cmd {
	f, v, text := doc.file, doc.text.Version(), doc.text.String()
	return func() tea.Msg {
		return savedMsg{doc: doc, version: v, err: f.save(v, text)}
	}
}

// saved records how a background write went, unless its tab has closed
// since. A result is matched to its tab by document, never by path, so a
// file closed and opened again doesn't take the old tab's result.
func (m model) saved(msg savedMsg) model {
	tabs, _ := m.tabs.Tabs()
	if slices.ContainsFunc(tabs, func(t openTab) bool { return t.Value == msg.doc }) {
		msg.doc.writing = false
		msg.doc.record(msg.version, msg.err)
	}
	return m
}

// flushAll writes every tab's unsaved edits now, and reports whether every
// tab is saved.
func (m model) flushAll() bool {
	ok := true
	tabs, _ := m.tabs.Tabs()
	for _, t := range tabs {
		if t.Value.flush() != nil {
			ok = false
		}
	}
	return ok
}

// saveAll is the final save, once the program has ended: it writes every
// tab's unsaved edits and retires every file, so no background write can
// start, or still be running, after it. Its error says which tabs couldn't
// be saved, one per line.
func (m model) saveAll() error {
	var errs []error
	tabs, _ := m.tabs.Tabs()
	for _, t := range tabs {
		doc := t.Value
		if err := doc.flush(); err != nil {
			errs = append(errs, fmt.Errorf("can't save %s: %s", doc.path, errText(err)))
		}
		doc.retire()
	}
	return errors.Join(errs...)
}

// saveProblem returns the tab whose failed write the header tells about:
// the active tab, if its last write failed, else the leftmost such tab; nil
// when no write failed.
func (m model) saveProblem() *document {
	tabs, active := m.tabs.Tabs()
	if active >= 0 && tabs[active].Value.saveErr != nil {
		return tabs[active].Value
	}
	for _, t := range tabs {
		if t.Value.saveErr != nil {
			return t.Value
		}
	}
	return nil
}
