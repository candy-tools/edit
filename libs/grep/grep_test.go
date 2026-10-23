package grep

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"unicode/utf8"
)

// places returns each of ms as "line:column:match", for short comparisons.
func places(ms []Match) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = fmt.Sprintf("%d:%d:%s", m.Line, m.Column, m.Text[m.Start:m.End])
	}
	return out
}

// findAll returns every place q is found in text, as places writes them.
func findAll(text, q string) []string {
	ms, _ := Find([]byte(text), NewQuery(q), 0)
	return places(ms)
}

func TestSmartCase(t *testing.T) {
	text := "map Map MAP mAp\nÉcole école\n\u212A k İ i ẞ ß" // \u212A: the Kelvin sign, three bytes
	for _, c := range []struct {
		query string
		want  []string
	}{
		{"map", []string{"0:0:map", "0:4:Map", "0:8:MAP", "0:12:mAp"}},
		{"Map", []string{"0:4:Map"}},
		{"école", []string{"1:0:École", "1:7:école"}},
		{"École", []string{"1:0:École"}},
		{"k", []string{"2:4:k"}},  // not the Kelvin sign, whose small letter is k
		{"i", []string{"2:9:i"}},  // not İ, whose small letter is i
		{"ß", []string{"2:15:ß"}}, // not ẞ, whose small letter is ß
	} {
		if got := findAll(text, c.query); !slices.Equal(got, c.want) {
			t.Errorf("%q: found %q, want %q", c.query, got, c.want)
		}
	}
}

func TestFind(t *testing.T) {
	for _, c := range []struct {
		name, text, query string
		want              []string
	}{
		{"matches don't overlap", "aaaa", "aa", []string{"0:0:aa", "0:2:aa"}},
		{"several in a line, on several lines", "ab ab\nx\nab", "ab", []string{"0:0:ab", "0:3:ab", "2:0:ab"}},
		{"columns in bytes", "日本 x\nx", "x", []string{"0:7:x", "1:0:x"}},
		{"CRLF", "ab\r\nxab\r\n", "ab", []string{"0:0:ab", "1:1:ab"}},
		{"invalid bytes around a match", "\xffab\xfe", "ab", []string{"0:1:ab"}},
		{"an empty query", "abc", "", nil},
		{"a line break in the query", "a\nb", "a\nb", nil},
		{"a carriage return in the query", "a\r\nb", "a\r", nil},
		{"nowhere", "abc", "x", nil},
	} {
		if got := findAll(c.text, c.query); !slices.Equal(got, c.want) {
			t.Errorf("%s: found %q, want %q", c.name, got, c.want)
		}
	}
}

func TestALinesTextLeavesItsLineBreakOut(t *testing.T) {
	ms, _ := Find([]byte("ab\r\nab\r"), NewQuery("ab"), 0)
	if len(ms) != 2 || ms[0].Text != "ab" || ms[1].Text != "ab\r" {
		t.Errorf("found %+v, want the first line without its \\r\\n, the last as it is", ms)
	}
}

func TestALongLineIsCutAroundItsMatch(t *testing.T) {
	line := strings.Repeat("日", 200) + "needle" + strings.Repeat("ü", 300) // 600 bytes before the match
	ms, _ := Find([]byte("x\n"+line+"\ny"), NewQuery("needle"), 0)
	if len(ms) != 1 {
		t.Fatalf("found %d matches, want 1", len(ms))
	}
	m := ms[0]
	if m.Line != 1 || m.Column != 600 || m.Text[m.Start:m.End] != "needle" {
		t.Errorf("match at line %d column %d holding %q, want line 1 column 600, needle", m.Line, m.Column, m.Text[m.Start:m.End])
	}
	if !utf8.ValidString(m.Text) || m.Start > 256 || len(m.Text)-m.End > 256 {
		t.Errorf("Text %q, Start %d: want at most 256 bytes on each side, cut between characters", m.Text, m.Start)
	}
}

func TestFindsLimit(t *testing.T) {
	for _, c := range []struct {
		text     string
		limit    int
		n        int
		wantMore bool
	}{
		{"a a a", 2, 2, true},
		{"a a", 2, 2, false},
		{"a a a", 0, 3, false},
		{"a a a", -1, 3, false},
	} {
		if ms, more := Find([]byte(c.text), NewQuery("a"), c.limit); len(ms) != c.n || more != c.wantMore {
			t.Errorf("%q, limit %d: %d matches, more %t; want %d, %t", c.text, c.limit, len(ms), more, c.n, c.wantMore)
		}
	}
}

// Review focus: a minified file, one long line holding many matches, costs
// a pass over it: each line's end is found once, not once per match.
func TestManyMatchesOnALongLineStayFast(t *testing.T) {
	text := bytes.Repeat([]byte("ab "), 10<<20) // 30 MB on one line, a match every 3 bytes
	start := time.Now()
	if ms, more := Find(text, NewQuery("ab"), 20000); len(ms) != 20000 || !more {
		t.Fatalf("%d matches, more %t; want 20000, true", len(ms), more)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("Find took %v: it should find the line's end once", d)
	}
}

// Lowering keeps every offset: the same length, and each character's first
// byte where it was.
func FuzzLower(f *testing.F) {
	for _, s := range []string{"Hello, World", "École ÉCOLE", "ΣΑΣ σας", "Привет МИР", "\u212A İ ẞ Ⱥ", "\xff\xfeA\x80", "日本語 ＡＢＣ", "é"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		got := lower(nil, []byte(text))
		if len(got) != len(text) {
			t.Fatalf("lower(%q) is %d bytes, want %d", text, len(got), len(text))
		}
		for i := range len(text) {
			if utf8.RuneStart(got[i]) != utf8.RuneStart(text[i]) {
				t.Fatalf("lower(%q) = %q: byte %d starts a character in one and not the other", text, got, i)
			}
		}
	})
}

func FuzzFind(f *testing.F) {
	f.Add("map Map\nMAP", "map")
	f.Add("aaaa", "aa")
	f.Add("École\r\nécole", "école")
	f.Add("\xff\u212A\xfeİ k", "k")
	f.Fuzz(func(t *testing.T, text, query string) {
		if !utf8.ValidString(query) {
			return // keys type valid text
		}
		q := NewQuery(query)
		ms, _ := Find([]byte(text), q, 0)
		lines := strings.Split(text, "\n")
		starts := make([]int, len(lines)) // each line's offset in text
		for i := 1; i < len(lines); i++ {
			starts[i] = starts[i-1] + len(lines[i-1]) + 1
		}
		end := 0 // where the match before ended
		for _, m := range ms {
			at := starts[m.Line] + m.Column
			if at < end {
				t.Fatalf("match %+v starts before the one before ended", m)
			}
			end = at + m.End - m.Start
			got := []byte(text[at:end])
			if q.fold {
				got = lower(nil, got)
			}
			if !bytes.Equal(got, q.text) || m.Text[m.Start:m.End] != text[at:end] {
				t.Fatalf("match %+v holds %q, not the query %q", m, text[at:end], query)
			}
			if line := strings.TrimSuffix(lines[m.Line], "\r"); len(line) <= longLine && m.Line < len(lines)-1 && m.Text != line {
				t.Fatalf("match %+v: Text %q, want the line %q", m, m.Text, line)
			}
		}
		hay := []byte(text)
		if q.fold {
			hay = lower(nil, hay)
		}
		if want := countIn(hay, q.text); len(ms) != want {
			t.Fatalf("%d matches, a plain scan finds %d", len(ms), want)
		}
	})
}

// countIn counts the places q is found in hay, none overlapping: none for
// a nil q.
func countIn(hay, q []byte) int {
	if q == nil {
		return 0
	}
	return bytes.Count(hay, q)
}

// tree is a folder tree for the Search tests.
func tree() fstest.MapFS {
	return fstest.MapFS{
		"A.txt":         {Data: []byte("needle A\n")},
		"b.txt":         {Data: []byte("needle b\n")},
		"dir/c.txt":     {Data: []byte("x\nneedle c\n")},
		"dir/sub/d.txt": {Data: []byte("needle d\n")},
		"zdir/e.txt":    {Data: []byte("needle e\n")},
	}
}

// search runs Search on fsys for query with opts, and returns where each
// match was found, as "path:line", in the order found got them.
func search(t *testing.T, fsys fs.FS, query string, opts Options) ([]string, bool, error) {
	t.Helper()
	var got []string
	more, err := Search(context.Background(), fsys, NewQuery(query), opts, func(ms []Match) {
		if len(ms) == 0 {
			t.Error("found was called with no matches")
		}
		for _, m := range ms {
			got = append(got, fmt.Sprintf("%s:%d", m.Path, m.Line))
		}
	})
	return got, more, err
}

// foldersFirst orders folders before files, then by name.
func foldersFirst(a, b fs.DirEntry) int {
	if a.IsDir() != b.IsDir() {
		if a.IsDir() {
			return -1
		}
		return 1
	}
	return strings.Compare(a.Name(), b.Name())
}

func TestSearchOrder(t *testing.T) {
	for _, c := range []struct {
		name string
		opts Options
		want []string
	}{
		{"by name", Options{}, []string{"A.txt:0", "b.txt:0", "dir/c.txt:1", "dir/sub/d.txt:0", "zdir/e.txt:0"}},
		{"by Order", Options{Order: foldersFirst}, []string{"dir/sub/d.txt:0", "dir/c.txt:1", "zdir/e.txt:0", "A.txt:0", "b.txt:0"}},
	} {
		got, _, err := search(t, tree(), "needle", c.opts)
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%s: found %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

func TestSkipLeavesOutAFolderAndAllItHolds(t *testing.T) {
	var asked []string
	got, _, _ := search(t, tree(), "needle", Options{Skip: func(p string, isDir bool) bool {
		asked = append(asked, fmt.Sprintf("%s %t", p, isDir))
		return p == "dir" || p == "b.txt"
	}})
	if want := []string{"A.txt:0", "zdir/e.txt:0"}; !slices.Equal(got, want) {
		t.Errorf("found %q, want %q", got, want)
	}
	if !slices.Contains(asked, "dir true") || slices.ContainsFunc(asked, func(a string) bool { return strings.HasPrefix(a, "dir/") }) {
		t.Errorf("Skip was asked %q: about dir, a folder, and nothing in it", asked)
	}
}

// tripFS is fsys, failing the test when a file of trip is opened.
type tripFS struct {
	t    *testing.T
	fsys fs.FS
	trip []string
}

func (f tripFS) Open(name string) (fs.File, error) {
	if slices.Contains(f.trip, name) {
		f.t.Errorf("%s was opened", name)
	}
	return f.fsys.Open(name)
}

func TestOnlyRegularFilesAreRead(t *testing.T) {
	fsys := tripFS{t, fstest.MapFS{
		"a.txt":   {Data: []byte("needle")},
		"link":    {Data: []byte("a.txt"), Mode: fs.ModeSymlink},
		"dirlink": {Data: []byte("dir"), Mode: fs.ModeSymlink},
		"pipe":    {Mode: fs.ModeNamedPipe},
	}, []string{"link", "dirlink", "pipe"}}
	if got, _, _ := search(t, fsys, "needle", Options{}); !slices.Equal(got, []string{"a.txt:0"}) {
		t.Errorf("found %q, want a.txt alone", got)
	}
}

func TestABinaryFileIsLeftOut(t *testing.T) {
	fsys := fstest.MapFS{
		"bin":  {Data: []byte("needle\x00")},
		"late": {Data: append(bytes.Repeat([]byte("x"), 9000), "\x00 needle"...)}, // a NUL past the first 8000 bytes: text
	}
	if got, _, _ := search(t, fsys, "needle", Options{}); !slices.Equal(got, []string{"late:0"}) {
		t.Errorf("found %q, want late alone", got)
	}
}

// shrunkFS is a MapFS whose folder listings say each file is 1 byte: as if
// the files grew after they were listed.
type shrunkFS struct{ fstest.MapFS }

func (f shrunkFS) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := f.MapFS.ReadDir(name)
	for i, e := range entries {
		entries[i] = shrunkEntry{e}
	}
	return entries, err
}

type shrunkEntry struct{ fs.DirEntry }

func (e shrunkEntry) Info() (fs.FileInfo, error) {
	info, err := e.DirEntry.Info()
	return oneByte{info}, err
}

type oneByte struct{ fs.FileInfo }

func (oneByte) Size() int64 { return 1 }

func TestAFileOverMaxSizeIsLeftOut(t *testing.T) {
	files := fstest.MapFS{"big": {Data: []byte("needle needle")}, "small": {Data: []byte("needle")}}
	got, _, _ := search(t, tripFS{t, files, []string{"big"}}, "needle", Options{MaxSize: 10})
	if !slices.Equal(got, []string{"small:0"}) {
		t.Errorf("found %q, want small alone, big never read", got)
	}
	if got, _, _ := search(t, shrunkFS{files}, "needle", Options{MaxSize: 10}); !slices.Equal(got, []string{"small:0"}) {
		t.Errorf("a file grown past MaxSize since it was listed: found %q, want small alone", got)
	}
}

// failFS is fsys, failing to open the files and folders of fail.
type failFS struct {
	fsys fs.FS
	fail []string
}

func (f failFS) Open(name string) (fs.File, error) {
	if slices.Contains(f.fail, name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return f.fsys.Open(name)
}

func TestWhatCantBeReadIsLeftOut(t *testing.T) {
	got, _, err := search(t, failFS{tree(), []string{"b.txt", "dir"}}, "needle", Options{})
	if want := []string{"A.txt:0", "zdir/e.txt:0"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("found %q, %v; want %q", got, err, want)
	}
	if _, _, err := search(t, failFS{tree(), []string{"."}}, "needle", Options{}); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("an unreadable root: %v, want its error", err)
	}
}

func TestContentsAreSearchedInPlaceOfFiles(t *testing.T) {
	opts := Options{Contents: map[string][]byte{"b.txt": []byte("a pin\n"), "gone.txt": []byte("pin")}}
	if got, _, _ := search(t, tree(), "pin", opts); !slices.Equal(got, []string{"b.txt:0"}) {
		t.Errorf("found %q, want b.txt's contents, and gone.txt, not in the tree, never", got)
	}
	if got, _, _ := search(t, tree(), "needle b", opts); len(got) != 0 {
		t.Errorf("found %q: b.txt's text on disk should not be searched", got)
	}
}

func TestSearchStopsPastItsLimit(t *testing.T) {
	for _, c := range []struct {
		limit    int
		n        int
		wantMore bool
	}{{3, 3, true}, {5, 5, false}, {0, 5, false}} {
		got, more, err := search(t, tree(), "needle", Options{Limit: c.limit})
		if len(got) != c.n || more != c.wantMore || err != nil {
			t.Errorf("limit %d: %d matches, more %t, %v; want %d, %t", c.limit, len(got), more, err, c.n, c.wantMore)
		}
	}
}

func TestACancelledSearchStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var got []string
	_, err := Search(ctx, tree(), NewQuery("needle"), Options{}, func(ms []Match) {
		got = append(got, ms[0].Path)
		cancel()
	})
	if !errors.Is(err, context.Canceled) || !slices.Equal(got, []string{"A.txt"}) {
		t.Errorf("found %q, %v; want A.txt, then the cancel's error", got, err)
	}
}

func TestFoundGetsItsOwnSliceAndNoTextIsWritten(t *testing.T) {
	text := []byte("Needle NEEDLE needle")
	before := slices.Clone(text)
	var batches, copies [][]Match
	_, _ = Search(context.Background(), tree(), NewQuery("needle"), Options{Contents: map[string][]byte{"b.txt": text}}, func(ms []Match) {
		batches, copies = append(batches, ms), append(copies, slices.Clone(ms))
	})
	for i := range batches {
		if !slices.Equal(batches[i], copies[i]) {
			t.Errorf("batch %d changed after found got it", i)
		}
	}
	if Find(text, NewQuery("needle"), 0); !bytes.Equal(text, before) {
		t.Errorf("the text became %q: no search may write to it", text)
	}
}
