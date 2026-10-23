package tui

import (
	"errors"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// disk is an in-memory stand-in for the workspace's files on disk: what the
// model writes there, the writes in order, writes that fail, and writes
// held until the test lets them go.
type disk struct {
	mu      sync.Mutex
	files   map[string]string
	writes  []string         // "path=data" for each write that went through, in order
	fails   map[string]error // the error every write of a path fails with
	entered chan string      // when set, gets each write's path as the write starts
	hold    chan struct{}    // when set, each write waits for it to close
}

func newDisk() *disk {
	return &disk{files: map[string]string{}, fails: map[string]error{}}
}

// write is the disk's writeFunc.
func (d *disk) write(path string, data []byte) error {
	d.mu.Lock()
	entered, hold := d.entered, d.hold
	d.mu.Unlock()
	if entered != nil {
		entered <- path
	}
	if hold != nil {
		<-hold
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.fails[path]; err != nil {
		return err
	}
	d.files[path] = string(data)
	d.writes = append(d.writes, path+"="+string(data))
	return nil
}

// fail makes every write of path fail with a permission error, or succeed
// again when on is false.
func (d *disk) fail(path string, on bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if on {
		d.fails[path] = &fs.PathError{Op: "open", Path: "/work/project/" + path, Err: fs.ErrPermission}
	} else {
		delete(d.fails, path)
	}
}

// file returns what the disk holds at path, and whether anything was
// written there.
func (d *disk) file(path string) (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	text, ok := d.files[path]
	return text, ok
}

// log returns the writes that went through, in order.
func (d *disk) log() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.writes...)
}

// instantTick makes ticks that arrive at once, so tests drive the saves by
// draining commands instead of waiting.
func instantTick() tea.Cmd {
	return func() tea.Msg { return saveTickMsg{} }
}

// newEditModel returns a model on fsys that writes to a new disk, with
// instant ticks, the default keys and a 100x30 terminal.
func newEditModel(fsys fs.FS) (model, *disk) {
	d := newDisk()
	m := newModel("/work/project", fsys, d.write, testKeys, noColors, noMouse)
	m.tick = instantTick
	return send(m, tea.WindowSizeMsg{Width: 100, Height: 30}), d
}

// update applies msg and returns the updated model and its command.
func update(m model, msg tea.Msg) (model, tea.Cmd) {
	nm, cmd := m.Update(msg)
	return nm.(model), cmd
}

// drain runs cmd and what it leads to, one command after another: each
// write's result goes to m, and draining stops at a tick, which it
// reports instead of delivering.
func drain(m model, cmd tea.Cmd) (model, bool) {
	ticked := false
	for queue := []tea.Cmd{cmd}; len(queue) > 0; queue = queue[1:] {
		if queue[0] == nil {
			continue
		}
		switch msg := queue[0]().(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case saveTickMsg:
			ticked = true
		default:
			var next tea.Cmd
			m, next = update(m, msg)
			queue = append(queue, next)
		}
	}
	return m, ticked
}

// tick delivers a save tick and drains the writes it starts; it reports
// whether another tick is due.
func tick(m model) (model, bool) {
	m, cmd := update(m, saveTickMsg{})
	return drain(m, cmd)
}

// goModText is go.mod's text in testFS.
const goModText = "module example.com/project\n\ngo 1.26\n"

// typeInGoMod returns a model on testFS with go.mod open and "x" typed at
// its start, unsaved, and the disk it writes to.
func typeInGoMod(t *testing.T) (model, *disk) {
	t.Helper()
	m, d := newEditModel(testFS)
	m = press(m, "down", "enter", "x")
	if !m.activeDocument().unsaved() {
		t.Fatal("typing should leave go.mod with unsaved edits")
	}
	return m, d
}

func TestAnEditSchedulesATick(t *testing.T) {
	m, _ := newEditModel(testFS)
	m = press(m, "down", "enter")
	if _, cmd := update(m, keyMsg("right")); cmd != nil || m.ticking {
		t.Fatal("with nothing unsaved, no tick is due")
	}
	m, cmd := update(m, keyMsg("x"))
	if _, ticked := drain(m, cmd); !ticked || !m.ticking {
		t.Fatal("an edit should schedule a tick")
	}
	if _, cmd := update(m, keyMsg("y")); cmd != nil {
		t.Error("with a tick on its way, another edit schedules none")
	}
}

func TestATickWritesTheUnsavedTabs(t *testing.T) {
	m, d := typeInGoMod(t)
	m, again := tick(m)
	if got, _ := d.file("go.mod"); got != "x"+goModText {
		t.Errorf("go.mod on disk = %q, want %q", got, "x"+goModText)
	}
	if again || m.activeDocument().unsaved() {
		t.Error("once everything is saved, no tick is due")
	}
	m, _ = tick(m)
	press(m, "right") // a move saves nothing
	if got := d.log(); len(got) != 1 {
		t.Errorf("writes = %q, want just the one", got)
	}
}

// Review focus: typing while a file's write runs never starts a second
// write of that file; the newer text goes next time.
func TestOneWriteOfAFileAtATime(t *testing.T) {
	m, d := typeInGoMod(t)
	m, write := update(m, saveTickMsg{}) // the write of "x…", not run yet
	m, cmd := update(m, keyMsg("y"))
	if cmd != nil {
		t.Error("while go.mod's write runs, typing schedules no tick: the write's result will")
	}
	if m, cmd = update(m, saveTickMsg{}); cmd != nil {
		t.Error("a tick while go.mod's write runs starts no second write")
	}
	m, ticked := drain(m, write)
	if !ticked {
		t.Fatal("the write's result should schedule a tick for the newer text")
	}
	m, _ = tick(m)
	if got := d.log(); len(got) != 2 || got[1] != "go.mod=xy"+goModText {
		t.Errorf("writes = %q, want x… then xy…", got)
	}
	if m.activeDocument().unsaved() {
		t.Error("go.mod should be saved")
	}
}

func TestAFailedWriteShowsAndIsRetried(t *testing.T) {
	m, d := typeInGoMod(t)
	d.fail("go.mod", true)
	m, again := tick(m)
	if !again {
		t.Error("a failed write should be retried at the next tick")
	}
	if header := strings.Split(ansi.Strip(m.View().Content), "\n")[0]; !strings.Contains(header, "⚠ Can't save go.mod: permission denied") {
		t.Errorf("header %q should say why go.mod can't be saved", header)
	}
	if top := topBorder(m); !strings.Contains(top, "─ ⚠ go.mod ─") {
		t.Errorf("top border %q should mark go.mod's tab", top)
	}
	d.fail("go.mod", false)
	m, again = tick(m)
	if again || m.activeDocument().unsaved() {
		t.Error("the retry should save go.mod")
	}
	if header := strings.Split(ansi.Strip(m.View().Content), "\n")[0]; !strings.Contains(header, "/work/project") || strings.Contains(header, "⚠") {
		t.Errorf("header %q should show the workspace again", header)
	}
	if top := topBorder(m); strings.Contains(top, "⚠") {
		t.Errorf("top border %q should no longer mark go.mod", top)
	}
}

func TestClosingATabSavesItFirst(t *testing.T) {
	m, d := typeInGoMod(t)
	m = press(m, "ctrl+w")
	if got, _ := d.file("go.mod"); got != "x"+goModText || m.tabs.Len() != 0 {
		t.Errorf("after closing: go.mod on disk %q, %d tabs; want the edit saved and the tab closed", got, m.tabs.Len())
	}
}

func TestAClosingThatCantSaveAsksAgain(t *testing.T) {
	m, d := typeInGoMod(t)
	d.fail("go.mod", true)
	m = press(m, "ctrl+w")
	if m.tabs.Len() != 1 {
		t.Fatal("a tab that can't be saved stays open")
	}
	if got, want := footerText(m), " ctrl+w: Close anyway  any other key: Cancel"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
	m = press(m, "right") // any other key cancels
	if got := footerText(m); !strings.Contains(got, "ctrl+w: Close  ") {
		t.Errorf("after another key the footer = %q, want the usual hints", got)
	}
	m = press(m, "ctrl+w", "ctrl+w")
	if m.tabs.Len() != 0 {
		t.Error("close pressed twice in a row closes the tab anyway")
	}
	if _, ok := d.file("go.mod"); ok {
		t.Error("nothing could be written")
	}
}

// Review focus: esc is how a prompt is cancelled, so a double esc, which
// closes a tab, never closes one anyway: its first esc cancels the prompt,
// and its second closes, which asks again.
func TestADoubleEscNeverClosesAnyway(t *testing.T) {
	m, d := typeInGoMod(t)
	d.fail("go.mod", true)
	m = press(m, "ctrl+w", "esc", "esc")
	if m.tabs.Len() != 1 {
		t.Fatal("a double esc must not close a tab that can't be saved")
	}
	if got, want := footerText(m), " ctrl+w: Close anyway  any other key: Cancel"; got != want {
		t.Errorf("footer = %q, want it to ask again: %q", got, want)
	}
}

// The prompt offers the first key of close or quit that is no sequence,
// since only such a key closes or quits anyway.
func TestThePromptOffersAKeyNotASequence(t *testing.T) {
	for _, c := range []struct {
		name string
		o    overrides
		keys []string // the keys that close the tab or quit, which then ask again
		want string
	}{
		{"a sequence first", overrides{"global": {"close-tab": {"esc esc", "ctrl+x"}}}, []string{"esc", "esc"},
			" ctrl+x: Close anyway  any other key: Cancel"},
		{"only a sequence", overrides{"global": {"close-tab": {"esc esc"}}}, []string{"esc", "esc"},
			" any other key: Cancel"},
		{"quit", overrides{"global": {"quit": {"esc q", "ctrl+q"}}}, []string{"ctrl+q"},
			" ctrl+q: Quit anyway  any other key: Cancel"},
	} {
		d := newDisk()
		m := send(newModel("/work/project", testFS, d.write, rebound(t, c.o), noColors, noMouse), tea.WindowSizeMsg{Width: 100, Height: 30})
		m = press(m, "down", "enter", "x")
		d.fail("go.mod", true)
		if got := footerText(press(m, c.keys...)); got != c.want {
			t.Errorf("%s: footer = %q, want %q", c.name, got, c.want)
		}
	}
}

// A tab's ×, like close, saves the tab first: when it can't, the tab stays
// and is shown, the footer asks again, and its × again closes it anyway.
func TestClickingTheXOfATabThatCantBeSaved(t *testing.T) {
	d := newDisk()
	m := send(newModel("/work/project", testFS, d.write, testKeys, noColors, true), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = press(m, "down", "enter", "x", "shift+tab", "down", "enter") // go.mod edited, then README.md, active
	d.fail("go.mod", true)
	m = click(t, m, tea.MouseLeft, "×", 0) // go.mod's
	if got := activeKey(m); m.tabs.Len() != 2 || got != "go.mod" {
		t.Fatalf("after go.mod's ×: %d tabs, active %q; want go.mod kept and shown", m.tabs.Len(), got)
	}
	if got, want := footerText(m), " ctrl+w: Close anyway  any other key: Cancel"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
	m = click(t, m, tea.MouseLeft, "×", 0)
	if keys, _ := tabKeys(m); !slices.Equal(keys, []string{"README.md"}) {
		t.Errorf("after go.mod's × again: tabs %q, want it closed anyway", keys)
	}
}

// Review focus: the prompt is for the tab that couldn't be saved; another
// tab's × tries that tab, and never closes it anyway.
func TestAnotherTabsXDoesntCloseItAnyway(t *testing.T) {
	d := newDisk()
	m := send(newModel("/work/project", testFS, d.write, testKeys, noColors, true), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = press(m, "down", "enter", "x", "shift+tab", "down", "enter", "y") // go.mod and README.md edited
	d.fail("go.mod", true)
	d.fail("README.md", true)
	m = click(t, m, tea.MouseLeft, "×", 0) // go.mod's: the prompt
	m = click(t, m, tea.MouseLeft, "×", 1) // README.md's
	if got := activeKey(m); m.tabs.Len() != 2 || got != "README.md" {
		t.Errorf("after README.md's ×: %d tabs, active %q; want both kept, README.md shown", m.tabs.Len(), got)
	}
}

func TestQuittingSavesEveryTab(t *testing.T) {
	m, d := typeInGoMod(t)
	m = press(m, "shift+tab", "down", "enter", "y") // README.md too
	_, cmd := update(m, keyMsg("ctrl+q"))
	if cmd == nil || cmd() != (tea.QuitMsg{}) {
		t.Fatal("ctrl+q should quit")
	}
	for path, want := range map[string]string{"go.mod": "x" + goModText, "README.md": "y# project\n"} {
		if got, _ := d.file(path); got != want {
			t.Errorf("%s on disk = %q, want %q", path, got, want)
		}
	}
}

func TestAQuitThatCantSaveAsksAgain(t *testing.T) {
	m, d := typeInGoMod(t)
	d.fail("go.mod", true)
	m, cmd := update(m, keyMsg("ctrl+q"))
	m, _ = drain(m, cmd) // a retry's tick, not a quit
	if got, want := footerText(m), " ctrl+q: Quit anyway  any other key: Cancel"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
	if _, cmd = update(m, keyMsg("ctrl+q")); cmd == nil || cmd() != (tea.QuitMsg{}) {
		t.Error("quit pressed twice in a row quits anyway")
	}
}

// Review focus: closing a tab while its background write is still to run
// never lets that older text land after the close.
func TestNoWriteLandsAfterItsTabClosed(t *testing.T) {
	m, d := typeInGoMod(t)
	m, write := update(m, saveTickMsg{}) // the write of "x…", not run yet
	m = press(m, "y", "ctrl+w")          // the close writes "xy…"
	m, _ = drain(m, write)
	if got := d.log(); len(got) != 1 || got[0] != "go.mod=xy"+goModText {
		t.Errorf("writes = %q, want only the close's", got)
	}
	if m.tabs.Len() != 0 {
		t.Error("the tab should be closed")
	}
}

// Review focus: a tab closed anyway, its file opened again, then the old
// tab's queued write: nothing lands, and the new tab doesn't take the old
// one's result.
func TestCloseAnywayReopenThenTheOldWrite(t *testing.T) {
	m, d := typeInGoMod(t)
	m, write := update(m, saveTickMsg{}) // the write of "x…", not run yet
	d.fail("go.mod", true)
	m = press(m, "ctrl+w", "ctrl+w") // closed anyway
	d.fail("go.mod", false)
	m = press(m, "enter") // go.mod again, the cursor still on it
	reopened := m.activeDocument()
	drain(m, write)
	if got := d.log(); len(got) != 0 {
		t.Errorf("writes = %q, want none", got)
	}
	if got := reopened.text.String(); got != goModText || reopened.unsaved() || reopened.saveErr != nil {
		t.Errorf("the reopened tab: %q, unsaved %t, error %v; want the file as it was", got, reopened.unsaved(), reopened.saveErr)
	}
}

// Review focus: a background result can arrive after a flush wrote newer
// text; it must neither mark a saved tab nor hide a newer failure.
func TestOutcomesArrivingOutOfOrder(t *testing.T) {
	failure := errors.New("disk full")
	saved := &document{}
	saved.record(9, nil)
	saved.record(7, failure)
	if saved.saveErr != nil || saved.saved != 9 {
		t.Errorf("a late v7 failure after a v9 success: error %v, saved %d; want none and 9", saved.saveErr, saved.saved)
	}
	failing := &document{}
	failing.record(9, failure)
	failing.record(7, nil)
	if failing.saveErr == nil || failing.saved != 7 {
		t.Errorf("a late v7 success after a v9 failure: error %v, saved %d; want the failure kept and 7", failing.saveErr, failing.saved)
	}
	failing.record(8, failure)
	failing.record(9, nil)
	if failing.saveErr != nil || failing.saved != 9 {
		t.Errorf("v9 saved at last: error %v, saved %d; want none and 9", failing.saveErr, failing.saved)
	}
}

func TestSaveAllIsTheFinalSave(t *testing.T) {
	m, d := typeInGoMod(t)
	m = press(m, "shift+tab", "down", "enter", "y") // README.md too
	d.fail("README.md", true)
	err := m.saveAll()
	if err == nil || err.Error() != "can't save README.md: permission denied" {
		t.Errorf("saveAll = %v, want it to name README.md", err)
	}
	if got, _ := d.file("go.mod"); got != "x"+goModText {
		t.Errorf("go.mod on disk = %q", got)
	}
	d.fail("README.md", false)
	tabs, _ := m.tabs.Tabs()
	for _, tab := range tabs {
		if err := tab.Value.file.save(99, "late"); err != nil || len(d.log()) != 1 {
			t.Errorf("%s: a write after the final save went through: %q", tab.Key, d.log())
		}
	}
}

func TestACRLFFileIsSavedWithCRLF(t *testing.T) {
	m, d := newEditModel(fstest.MapFS{"dos.txt": {Data: []byte("one\r\ntwo\r\n")}})
	m = press(m, "enter", "end", "enter", "x")
	tick(m)
	if got, _ := d.file("dos.txt"); got != "one\r\nx\r\ntwo\r\n" {
		t.Errorf("dos.txt on disk = %q, want CRLF line breaks throughout", got)
	}
}

func TestAFileIsWrittenOnlyOnce(t *testing.T) {
	d := newDisk()
	f := &file{path: "f", write: d.write}
	if err := f.save(2, "v2"); err != nil {
		t.Fatal(err)
	}
	if err := f.save(1, "v1"); err != nil || len(d.log()) != 1 {
		t.Errorf("an older version after a newer one: %v, writes %q; want it skipped", err, d.log())
	}
	if err := f.save(2, "v2 again"); err != nil || len(d.log()) != 1 {
		t.Errorf("the same version again: %v, writes %q; want it skipped", err, d.log())
	}
	f.retire()
	if err := f.save(3, "v3"); err != nil || len(d.log()) != 1 {
		t.Errorf("after retire: %v, writes %q; want nothing written", err, d.log())
	}
}

// Review focus: the save of a close or quit waits for the background write
// in progress, then writes the newer text: writes never overlap or go
// backwards.
func TestASaveWaitsForTheWriteInProgress(t *testing.T) {
	d := newDisk()
	d.entered, d.hold = make(chan string, 2), make(chan struct{})
	f := &file{path: "f", write: d.write}
	first := make(chan error)
	go func() { first <- f.save(1, "v1") }()
	<-d.entered // the first write is in progress
	second := make(chan error)
	go func() { second <- f.save(2, "v2") }()
	select {
	case <-second:
		t.Fatal("the second save returned while the first write was still in progress")
	case <-time.After(50 * time.Millisecond):
	}
	close(d.hold)
	if err := errors.Join(<-first, <-second); err != nil {
		t.Fatal(err)
	}
	if got := d.log(); len(got) != 2 || got[0] != "f=v1" || got[1] != "f=v2" {
		t.Errorf("writes = %q, want v1 then v2", got)
	}
}

func TestTheGate(t *testing.T) {
	d := newDisk()
	g := &gate{write: d.write}
	if err := g.pass("a", []byte("1")); err != nil {
		t.Fatal(err)
	}
	d.entered, d.hold = make(chan string, 1), make(chan struct{})
	written := make(chan error)
	go func() { written <- g.pass("b", []byte("2")) }()
	<-d.entered // the write of b is in progress
	closed := make(chan struct{})
	go func() {
		g.close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("close returned while a write was in progress")
	case <-time.After(50 * time.Millisecond):
	}
	close(d.hold)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	<-closed
	d.entered = nil
	if err := g.pass("c", []byte("3")); !errors.Is(err, errExiting) {
		t.Errorf("a write after close: %v, want it refused", err)
	}
	if got := d.log(); len(got) != 2 {
		t.Errorf("writes = %q, want a and b only", got)
	}
}

func TestDiskWriterWritesInPlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := diskWriter(dir)("f.txt", []byte("new")); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || after.Mode().Perm() != 0o600 {
		t.Errorf("the file was replaced or its mode changed: same file %t, mode %v", os.SameFile(before, after), after.Mode())
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "link.txt")); string(got) != "new" {
		t.Errorf("the hard link reads %q, want the new text", got)
	}
}

func TestDiskWriterFollowsASymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, "real.txt", filepath.Join(dir, "sym.txt"))
	if err := diskWriter(dir)("sym.txt", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "real.txt")); string(got) != "new" {
		t.Errorf("the target reads %q, want the new text", got)
	}
	if fi, err := os.Lstat(filepath.Join(dir, "sym.txt")); err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("the link should still be a symlink: %v, %v", fi, err)
	}
}

func TestDiskWriterCreatesADeletedFileAgain(t *testing.T) {
	dir := t.TempDir()
	if err := diskWriter(dir)("app/gone.go", []byte("x")); err == nil || errText(err) != "no such file or directory" {
		t.Errorf("writing into a missing folder: %v, want no such file or directory", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "app"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := diskWriter(dir)("app/gone.go", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "app", "gone.go")); string(got) != "x" {
		t.Errorf("the file reads %q, want x", got)
	}
}

// runOpts are the options that let a real Bubble Tea program run in a test:
// keys from in, the screen thrown away, a 100x30 terminal, and no signal
// handling of its own.
func runOpts(in io.Reader) []tea.ProgramOption {
	return []tea.ProgramOption{tea.WithInput(in), tea.WithOutput(io.Discard), tea.WithWindowSize(100, 30), tea.WithoutSignalHandler()}
}

func TestRunSavesWhenTheProgramQuits(t *testing.T) {
	m, d := newEditModel(testFS)
	m = press(m, "down", "enter")
	m.tick = saveTick
	if err := run(m, make(chan os.Signal), runOpts(strings.NewReader("x\x11"))...); err != nil { // x, then ctrl+q
		t.Fatal(err)
	}
	if got, _ := d.file("go.mod"); got != "x"+goModText {
		t.Errorf("go.mod on disk = %q, want the typed x saved", got)
	}
}

// Review focus: the terminal closing sends SIGHUP, and kill sends SIGTERM or
// SIGINT; edit quits and saves on each.
func TestRunSavesOnASignal(t *testing.T) {
	for _, sig := range []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM} {
		m, d := typeInGoMod(t)
		m.tick = saveTick
		sigs := make(chan os.Signal, 1)
		done := make(chan error)
		go func() { done <- run(m, sigs, runOpts(nil)...) }()
		sigs <- sig
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%v: %v", sig, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("run didn't quit on %v", sig)
		}
		if got, _ := d.file("go.mod"); got != "x"+goModText {
			t.Errorf("%v: go.mod on disk = %q, want the edit saved", sig, got)
		}
	}
}

// Review focus: a signal can come as the program ends, or after; edit's own
// handling never holds the exit up.
func TestALateSignalDoesntHoldTheExitUp(t *testing.T) {
	m, _ := newEditModel(testFS)
	sigs := make(chan os.Signal, 3)
	sigs <- syscall.SIGHUP // quits the program…
	sigs <- syscall.SIGTERM
	sigs <- syscall.SIGTERM // …and these come as it ends
	done := make(chan error)
	go func() { done <- run(m, sigs, runOpts(nil)...) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run hung on signals that came as it ended")
	}
}

// Review focus: one tab's write hanging, on a network mount say, must not
// hold up the other tabs' saves.
func TestAHungWriteDoesntHoldUpTheOtherTabs(t *testing.T) {
	m, d := typeInGoMod(t)
	m, _ = update(m, saveTickMsg{})            // go.mod's write starts, and never finishes
	m = press(m, "shift+tab", "down", "enter") // README.md
	m, _ = update(m, keyMsg("y"))
	if !m.ticking {
		t.Fatal("README.md's edit should schedule a tick while go.mod's write hangs")
	}
	tick(m)
	if got, _ := d.file("README.md"); got != "y# project\n" {
		t.Errorf("README.md on disk = %q, want its edit saved", got)
	}
}

// savingPaths are the files of testFS a savingRun opens.
var savingPaths = []string{"go.mod", "README.md", "app/main.go"}

// savingRun is one run of TestSavingNeverStops: a model, its disk, and the
// commands its ticks started that haven't run yet.
type savingRun struct {
	t       *testing.T
	seed    uint64
	m       model
	d       *disk
	pending []tea.Cmd
}

// act does one thing to the model, picked by rnd, and says what it did.
func (r *savingRun) act(rnd *rand.Rand) string {
	switch rnd.IntN(8) {
	case 0, 1:
		r.m, _ = update(r.m, keyMsg("x"))
		return "typing"
	case 2:
		r.m, _ = update(r.m, keyMsg("ctrl+pgdown"))
		return "switching tabs"
	case 3:
		r.m = r.m.open(entry{path: savingPaths[rnd.IntN(len(savingPaths))]})
		return "opening a file"
	case 4:
		if r.m.ticking {
			r.tick()
		}
		return "a tick"
	case 5:
		if len(r.pending) > 0 {
			r.finish(rnd.IntN(len(r.pending)))
		}
		return "a write finishing"
	case 6:
		r.d.fail(savingPaths[rnd.IntN(len(savingPaths))], rnd.IntN(2) == 0)
		return "a file failing or working again"
	default:
		r.m, _ = update(r.m, keyMsg("ctrl+w"))
		return "closing a tab"
	}
}

// tick delivers a save tick and keeps the writes it starts pending.
func (r *savingRun) tick() {
	var cmd tea.Cmd
	if r.m, cmd = update(r.m, saveTickMsg{}); cmd != nil {
		r.pending = append(r.pending, cmd)
	}
}

// finish runs the i-th pending command: a batch hands back the commands it
// holds, still pending; anything else is a write, whose result goes to the
// model.
func (r *savingRun) finish(i int) {
	c := r.pending[i]
	r.pending = slices.Delete(r.pending, i, i+1)
	switch msg := c().(type) {
	case tea.BatchMsg:
		r.pending = append(r.pending, msg...)
	default:
		r.m, _ = update(r.m, msg)
	}
}

// check fails the test unless each tab with unsaved edits has a write
// running or a tick on its way.
func (r *savingRun) check(after string) {
	r.t.Helper()
	tabs, _ := r.m.tabs.Tabs()
	for _, tab := range tabs {
		if doc := tab.Value; doc.unsaved() && !doc.writing && !r.m.ticking {
			r.t.Fatalf("seed %d, after %s: %s has unsaved edits, no write running and no tick on its way", r.seed, after, tab.Key)
		}
	}
}

// settle lets every write work again, then runs the pending commands and
// the ticks until nothing is left to save.
func (r *savingRun) settle() {
	r.t.Helper()
	for _, p := range savingPaths {
		r.d.fail(p, false)
	}
	for guard := 0; len(r.pending) > 0 || r.m.ticking; guard++ {
		if guard > 1000 {
			r.t.Fatalf("seed %d: saving never settles", r.seed)
		}
		if len(r.pending) > 0 {
			r.finish(0)
		} else {
			r.tick()
		}
		r.check("settling")
	}
}

// verify fails the test unless the disk holds the text of each edited tab.
func (r *savingRun) verify() {
	r.t.Helper()
	tabs, _ := r.m.tabs.Tabs()
	for _, tab := range tabs {
		if doc := tab.Value; doc.text.Version() > 0 {
			if got, _ := r.d.file(tab.Key); got != doc.text.String() {
				r.t.Fatalf("seed %d: %s on disk = %q, want the tab's %q", r.seed, tab.Key, got, doc.text.String())
			}
		}
	}
}

// Review focus: whatever the mix of typing, switching and closing tabs,
// ticks, writes finishing in any order, and writes failing, saving never
// silently stops: after every message each tab with unsaved edits has a
// write running or a tick on its way, and once writes work again, every
// tab's text reaches the disk.
func TestSavingNeverStops(t *testing.T) {
	for seed := range uint64(300) {
		m, d := newEditModel(testFS)
		r := &savingRun{t: t, seed: seed, m: m, d: d}
		rnd := rand.New(rand.NewPCG(seed, 1))
		for range 150 {
			r.check(r.act(rnd))
		}
		r.settle()
		r.verify()
	}
}

func TestRunReportsWhatItCouldntSave(t *testing.T) {
	m, d := typeInGoMod(t)
	m.tick = saveTick
	d.fail("go.mod", true)
	err := run(m, make(chan os.Signal), runOpts(strings.NewReader("\x11\x11"))...) // ctrl+q twice: quit anyway
	if err == nil || err.Error() != "can't save go.mod: permission denied" {
		t.Errorf("run = %v, want it to say go.mod couldn't be saved", err)
	}
}
