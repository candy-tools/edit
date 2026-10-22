package treesitter

import (
	"maps"
	"math"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/candy-tools/edit/libs/syntax"
)

// parse parses text in the language Detect finds for a file called name,
// and closes the tree when the test ends.
func parse(t testing.TB, name, text string) syntax.Tree {
	t.Helper()
	first, _, _ := strings.Cut(text, "\n")
	lang, ok := Detect(name, first)
	if !ok {
		t.Fatalf("Detect(%q) knows no language", name)
	}
	tree, err := lang.Parse([]byte(text))
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	t.Cleanup(tree.Close)
	return tree
}

// allSpans returns the spans of every line of text, which tree was parsed
// from.
func allSpans(tree syntax.Tree, text string) [][]syntax.Span {
	return tree.Spans([]byte(text), 0, strings.Count(text, "\n")+1, 0, math.MaxInt)
}

// captureAt returns the capture of the span on off in spans, "" for none.
func captureAt(spans []syntax.Span, off int) string {
	for _, s := range spans {
		if s.Start <= off && off < s.End {
			return s.Capture
		}
	}
	return ""
}

// captureOf returns the capture that colors all of tok, found in text where
// its context ctx first occurs: "" when none does, and "mixed" when tok's
// bytes don't all have the same.
func captureOf(spans [][]syntax.Span, text, ctx, tok string) string {
	i, j := strings.Index(text, ctx), strings.Index(ctx, tok)
	if i < 0 || j < 0 {
		return "not in the text"
	}
	off := i + j
	line := strings.Count(text[:off], "\n")
	col := off - strings.LastIndex(text[:off], "\n") - 1
	name := captureAt(spans[line], col)
	for k := col + 1; k < col+len(tok); k++ {
		if captureAt(spans[line], k) != name {
			return "mixed"
		}
	}
	return name
}

// item is a token of a text, found where its context ctx first occurs
// (tok "" is ctx itself), and the capture that must color it.
type item struct{ ctx, tok, want string }

// colored has a text in each language, and one item for each thing the
// spec's What gets colored lists.
var colored = []struct {
	file, text string
	items      []item
}{
	{"main.go", "package main\n\nimport \"fmt\"\n\n// Doc.\ntype T struct{ n int }\n\nfunc (t *T) Get() int { return t.n }\n\n" +
		"func helper(x []int) int { return len(x) }\n\nfunc main() {\n\tx := []int{1}\n" +
		"\tfmt.Println(helper(x), nil, true, iota, 1.5, 'r', \"a\\tb\")\n}\n", []item{
		{"package", "", "keyword"},
		{"func main", "func", "keyword"},
		{"n int", "int", "type.builtin"},
		{"true", "", "boolean"},
		{"nil", "", "constant.builtin"},
		{"iota", "", "constant.builtin"},
		{"len(x)", "len", "function.builtin"},
		{"type T", "T", "type"},
		{"func helper", "helper", "function"},
		{"helper(x),", "helper", "function"},
		{") Get(", "Get", "function.method"},
		{".Println", "Println", "function.method"},
		{"t.n", "n", "property"},
		{`"fmt"`, "", "string"},
		{"'r'", "", "character"},
		{`\t`, "", "string.escape"},
		{"1.5", "", "number"},
		{"// Doc.", "", "comment"},
	}},
	{"go.mod", "// deps\nmodule example.com/m\n\ngo 1.26\n\nrequire (\n\tgithub.com/a/b v1.3.10\n)\n\nreplace example.com/a => ../a\n", []item{
		{"module", "", "keyword"},
		{"go 1.26", "go", "keyword"},
		{"require", "", "keyword"},
		{"replace", "", "keyword"},
		{"v1.3.10", "", "number"},
		{"1.26", "", "number"},
		{"// deps", "", "comment"},
	}},
	{"config.yaml", "# settings\nname: \"edit\"\ncount: 3\nratio: 1.5\non: true\noff: null\nplain: text\nbase: &base\n  a: 1\n" +
		"copy: *base\ntagged: !custom value\nlist:\n  - one\nblock: |\n  line one\n", []item{
		{"name:", "name", "property"},
		{`"edit"`, "", "string"},
		{"text", "", "string"},
		{"count: 3", "3", "number"},
		{"1.5", "", "number"},
		{"true", "", "boolean"},
		{"null", "", "constant.builtin"},
		{"# settings", "", "comment"},
		{"&base", "base", "label"},
		{"*base", "base", "label"},
		{"!custom", "", "type"},
		{"- one", "-", "punctuation.delimiter"},
		{"name:", ":", "punctuation.delimiter"},
		{"line one", "", "string"},
	}},
	{"package.json", `{"name": "edit", "count": 3, "ok": true, "no": false, "none": null}` + "\n", []item{
		{`"name"`, "", "property"},
		{`"edit"`, "", "string"},
		{"3", "", "number"},
		{"true", "", "boolean"},
		{"false", "", "boolean"},
		{"null", "", "constant.builtin"},
	}},
	{"README.md", "# Title\n\nSome *italic*, **strong**, `code` and [link](https://x.y).\n\n> quoted\n\n    indented\n\n- item\n\n" +
		"```go\nfunc f() {}\n```\n\n```rust\nfn main() {}\n```\n", []item{
		{"# Title", "", "markup.heading"},
		{"*italic*", "", "markup.italic"},
		{"**strong**", "", "markup.strong"},
		{"`code`", "", "markup.raw"},
		{"[link]", "link", "markup.link.label"},
		{"https://x.y", "", "markup.link.url"},
		{"> quoted", "", "markup.quote"},
		{"- item", "-", "markup.list"},
		{"    indented", "indented", "markup.raw"},
		{"func f", "func", "keyword"},
		{"fn main() {}", "", "markup.raw"},
	}},
	{"setup", "#!/bin/sh\n# setup\nname=\"hi $HOME\"\nfiles=$(ls -la)\nif [ -n \"${name}\" ]; then echo \"$name\"; fi\ncat <<END\nbody\nEND\n", []item{
		{"if", "", "keyword"},
		{"then", "", "keyword"},
		{"ls", "", "function"},
		{"echo", "", "function.builtin"},
		{"$HOME", "", "string.special"},
		{"${name}", "", "string.special"},
		{"$(", "", "string.special"},
		{`"hi `, "", "string"},
		{"-la", "", "constant"},
		{"body", "", "string"},
		{"# setup", "", "comment"},
	}},
	{"Makefile", "# Build.\nCC ?= gcc\n.PHONY: test\n\ntest: build\n\t@$(CC) -o out main.c\n\nifeq ($(OS),Windows)\nEXE := .exe\nendif\ninclude other.mk\n", []item{
		{"test: build", "test", "function"},
		{".PHONY", "", "function.builtin"},
		{"CC ?=", "CC", "variable"},
		{"$(CC)", "", "string.special"},
		{"ifeq", "", "keyword"},
		{"endif", "", "keyword"},
		{"include", "", "keyword"},
		{"@$(CC)", "@", "punctuation.special"},
		{"# Build.", "", "comment"},
	}},
	{"app.js", "// app\nconst re = /ab+c/g;\nlet n = 0x1f;\nclass Greeter {\n  greet() { return `hi ${this.name}`; }\n}\n" +
		"function main() {\n  console.log(new Greeter().greet(), 'x');\n}\nmain();\n", []item{
		{"const", "", "keyword"},
		{"function main", "main", "function"},
		{"main();", "main", "function"},
		{"greet()", "greet", "function.method"},
		{".log", "log", "function.method"},
		{"this.name", "name", "property"},
		{"'x'", "", "string"},
		{"`hi ", "", "string"},
		{"/ab+c/g", "", "string.special"},
		{"0x1f", "", "number"},
		{"console", "", "variable.builtin"},
		{"// app", "", "comment"},
	}},
	{"point.py", "\"\"\"Doc.\"\"\"\nimport os\n\n@dataclass\nclass Point:\n    def norm(self):\n        # note\n        return abs(self.x) * 1.5\n\n" +
		"p = Point()\ns = f\"hi {p}\\t\"\nnorm(p)\n", []item{
		{"def", "", "keyword"},
		{"class Point", "class", "keyword"},
		{"abs", "", "function.builtin"},
		{"self", "", "variable.builtin"},
		{"def norm", "norm", "function"},
		{"norm(p)", "norm", "function"},
		{"class Point", "Point", "type"},
		{"Point()", "Point", "constructor"},
		{"@dataclass", "", "attribute"},
		{`"""Doc."""`, "", "string"},
		{`f"hi `, "", "string"},
		{`\t`, "", "string.escape"},
		{"1.5", "", "number"},
		{"# note", "", "comment"},
	}},
	{"A.java", "@Override\npublic class A {\n    // note\n    private int n = 0x10;\n    public String greet(String s) {\n" +
		"        var t = \"\"\"\n            block\n            \"\"\";\n        return s.trim() + 'c' + \"\\t\" + null + true + 1.5;\n    }\n}\n", []item{
		{"public", "", "keyword"},
		{"class", "", "keyword"},
		{"String greet", "String", "type"},
		{"int n", "int", "type.builtin"},
		{"@Override", "", "attribute"},
		{"greet(", "greet", "function.method"},
		{".trim", "trim", "function.method"},
		{`"""`, "", "string"},
		{`\t`, "", "string.escape"},
		{"0x10", "", "number"},
		{"1.5", "", "number"},
		{"// note", "", "comment"},
	}},
}

func TestWhatGetsColored(t *testing.T) {
	for _, c := range colored {
		spans := allSpans(parse(t, c.file, c.text), c.text)
		for _, it := range c.items {
			tok := it.tok
			if tok == "" {
				tok = it.ctx
			}
			if got := captureOf(spans, c.text, it.ctx, tok); got != it.want {
				t.Errorf("%s: %q in %q is %q, want %q", c.file, tok, it.ctx, got, it.want)
			}
		}
	}
}

// Every line's spans are sorted, without overlaps or empty spans, and
// capture names of the vocabulary.
func TestSpansAreSortedWithoutOverlaps(t *testing.T) {
	known := map[string]bool{}
	for _, c := range syntax.Captures() {
		known[c] = c != "none"
	}
	for _, c := range colored {
		for i, line := range allSpans(parse(t, c.file, c.text), c.text) {
			for k, s := range line {
				if s.Start >= s.End || (k > 0 && line[k-1].End > s.Start) || !known[s.Capture] {
					t.Errorf("%s, line %d: spans %+v", c.file, i, line)
					break
				}
			}
		}
	}
}

func TestACaptureAcrossLinesIsASpanOnEach(t *testing.T) {
	text := "package p\n\nvar s = `a\nb`\n\n/* c\nd */\n"
	got := allSpans(parse(t, "a.go", text), text)
	want := map[int][]syntax.Span{
		2: {{Start: 0, End: 3, Capture: "keyword"}, {Start: 4, End: 5, Capture: "variable"}, {Start: 6, End: 7, Capture: "operator"}, {Start: 8, End: math.MaxInt, Capture: "string"}},
		3: {{Start: 0, End: 2, Capture: "string"}},
		5: {{Start: 0, End: math.MaxInt, Capture: "comment"}},
		6: {{Start: 0, End: 4, Capture: "comment"}},
	}
	for line, w := range want {
		if !reflect.DeepEqual(got[line], w) {
			t.Errorf("line %d: %+v, want %+v", line, got[line], w)
		}
	}
}

// A fenced block in one of the languages owns its code: the code takes
// that language's colors, and its plain text none of the block's.
func TestInjectedCodeWinsOverItsBlock(t *testing.T) {
	text := "```go\nfunc f() {}\n```\n"
	got := allSpans(parse(t, "README.md", text), text)
	want := []syntax.Span{
		{Start: 0, End: 4, Capture: "keyword"},
		{Start: 5, End: 6, Capture: "function"},
		{Start: 6, End: 8, Capture: "punctuation.bracket"},
		{Start: 9, End: 11, Capture: "punctuation.bracket"},
	}
	if !reflect.DeepEqual(got[1], want) {
		t.Errorf("the code: %+v, want %+v", got[1], want)
	}
	if captureAt(got[0], 0) != "markup.raw" || captureAt(got[2], 0) != "markup.raw" {
		t.Errorf("the fences: %+v and %+v, want markup.raw", got[0], got[2])
	}
}

// Review focus: the colors on screen move with an edit at once, and a
// predicate reads the text as it is now: len stays a built-in after a
// keystroke before it, before any reparse.
func TestAnEditMovesTheSpansBeforeAReparse(t *testing.T) {
	text := "package p\n\nvar n = len(x)\n"
	tree := parse(t, "a.go", text)
	tree.Edit(syntax.Edit{StartByte: 11, OldEndByte: 11, NewEndByte: 13,
		Start: syntax.Point{Line: 2}, OldEnd: syntax.Point{Line: 2}, NewEnd: syntax.Point{Line: 2, Column: 2}})
	edited := "package p\n\nzzvar n = len(x)\n"
	spans := tree.Spans([]byte(edited), 2, 3, 0, math.MaxInt)[0]
	for _, c := range []struct {
		col  int
		want string
	}{{0, ""}, {2, "keyword"}, {10, "function.builtin"}, {12, "function.builtin"}} {
		if got := captureAt(spans, c.col); got != c.want {
			t.Errorf("column %d of %q: %q, want %q", c.col, "zzvar n = len(x)", got, c.want)
		}
	}
}

func TestAnEditMovesInjectedSpansToo(t *testing.T) {
	text := "```go\nfunc f() {}\n```\n"
	tree := parse(t, "README.md", text)
	tree.Edit(syntax.Edit{StartByte: 0, OldEndByte: 0, NewEndByte: 1,
		NewEnd: syntax.Point{Line: 1}})
	edited := "\n" + text
	if got := captureAt(tree.Spans([]byte(edited), 2, 3, 0, math.MaxInt)[0], 0); got != "keyword" {
		t.Errorf("after a line break above the block, func is %q, want keyword", got)
	}
}

func TestAParseStopsAtTheTimeout(t *testing.T) {
	defer func(old uint64) { timeout = old }(timeout)
	timeout = 1
	for name, text := range map[string]string{
		"a.go":      "package p\n" + strings.Repeat("func f() { x := []int{1, 2, 3}; _ = x }\n", 20000),
		"README.md": strings.Repeat("Some *text* and `code`.\n\n", 20000),
	} {
		lang, _ := Detect(name, "")
		if tree, err := lang.Parse([]byte(text)); err == nil {
			tree.Close()
			t.Errorf("%s: a parse past the timeout gave a tree", name)
		}
	}
}

func TestDetect(t *testing.T) {
	for name, want := range map[string]string{
		"main.go": "go", "go.mod": "gomod", "a.yaml": "yaml", "a.yml": "yaml",
		"a.json": "json", "package.json": "json", "a.geojson": "json",
		"README.md": "markdown", "a.markdown": "markdown",
		"a.sh": "bash", "a.bash": "bash", "a.zsh": "bash", ".bashrc": "bash", ".profile": "bash",
		"Makefile": "make", "makefile": "make", "GNUmakefile": "make", "a.mk": "make", "a.mak": "make",
		"a.js": "javascript", "a.mjs": "javascript", "a.cjs": "javascript", "a.jsx": "javascript",
		"a.py": "python", "a.pyi": "python", "a.pyw": "python", "A.java": "java",
	} {
		if l, ok := Detect(name, ""); !ok || l.Name() != want {
			t.Errorf("Detect(%q) = %v, %t; want %s", name, l, ok, want)
		}
	}
	for first, want := range map[string]string{
		"#!/bin/sh": "bash", "#!/usr/bin/env bash": "bash", "#!/usr/bin/env node": "javascript",
		"#!/usr/bin/python3": "python", "#!/usr/bin/env python3": "python",
	} {
		if l, ok := Detect("script", first); !ok || l.Name() != want {
			t.Errorf("Detect with first line %q = %v, %t; want %s", first, l, ok, want)
		}
	}
	for _, name := range []string{"x.ts", "Dockerfile", "go.sum", "notes.txt", "script"} {
		if l, ok := Detect(name, "hello"); ok {
			t.Errorf("Detect(%q) = %s, want no language", name, l.Name())
		}
	}
}

func TestLanguages(t *testing.T) {
	want := []string{"bash", "go", "gomod", "java", "javascript", "json", "make", "markdown", "python", "yaml"}
	if got := Languages(); !slices.Equal(got, want) {
		t.Errorf("Languages() = %q, want %q", got, want)
	}
}

// Every language loads in this build, every query compiles, and every
// capture is a name of the vocabulary or a predicate's, which starts with
// "_". This fails when the build-tag list in the Makefile misses a grammar.
func TestEveryQueryCompilesAndUsesTheVocabulary(t *testing.T) {
	known := map[string]bool{}
	for _, c := range syntax.Captures() {
		known[c] = true
	}
	for _, l := range append(slices.Collect(maps.Values(detectable)), inline) {
		if err := l.load(); err != nil {
			t.Errorf("%s: %v", l.name, err)
			continue
		}
		for _, name := range l.query.CaptureNames() {
			if !known[name] && !strings.HasPrefix(name, "_") {
				t.Errorf("%s captures %q, which is not in syntax.Captures", l.name, name)
			}
		}
	}
}

// Review focus: texts are parsed on background goroutines, several at
// once, and their trees used and freed on another: run with -race.
func TestTreesParsedElsewhere(t *testing.T) {
	trees := make([]syntax.Tree, len(colored))
	var wg sync.WaitGroup
	for i, c := range colored {
		wg.Go(func() { trees[i] = parse(t, c.file, c.text) })
	}
	wg.Wait()
	for i, tree := range trees {
		tree.Edit(syntax.Edit{StartByte: 0, OldEndByte: 0, NewEndByte: 1, NewEnd: syntax.Point{Column: 1}})
		tree.Spans([]byte(" "+colored[i].text), 0, 3, 0, math.MaxInt)
		tree.Close()
	}
}

// Review fix: every parse of a language shares its grammar with the others,
// and with the queries of its trees. Two tabs of one language, or a Go tab
// beside a Markdown file's Go block, parse it on background goroutines while
// a tree of it is edited and drawn on the editor's: nothing a parse writes
// may go unguarded (run with -race).
func TestALanguageParsedAtOnceWhileATreeOfItIsDrawn(t *testing.T) {
	ins := syntax.Edit{NewEndByte: 1, NewEnd: syntax.Point{Column: 1}}
	del := syntax.Edit{OldEndByte: 1, OldEnd: syntax.Point{Column: 1}}
	for _, c := range colored {
		tree := parse(t, c.file, c.text)
		first, _, _ := strings.Cut(c.text, "\n")
		lang, _ := Detect(c.file, first)
		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				for range 5 {
					if other, err := lang.Parse([]byte(c.text)); err != nil {
						t.Errorf("%s: %v", c.file, err)
					} else {
						other.Close()
					}
				}
			})
		}
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		finished := func() bool {
			select {
			case <-done:
				return true
			default:
				return false
			}
		}
		for i, spaced := 0, false; i < 20 || !finished(); i, spaced = i+1, !spaced { // spaced: the text starts with a space
			edit, text := ins, " "+c.text
			if spaced {
				edit, text = del, c.text
			}
			tree.Edit(edit)
			tree.Spans([]byte(text), 0, 3, 0, math.MaxInt)
		}
	}
}

// Review fix: the editor asks for the spans of only the bytes its window
// shows of a line, so that a long line costs no more than a short one. On
// those bytes, they must be the whole line's.
func TestSpansOnTheBytesOfAWindow(t *testing.T) {
	for _, c := range colored {
		tree, src := parse(t, c.file, c.text), []byte(c.text)
		whole := allSpans(tree, c.text)
		for i, line := range strings.Split(c.text, "\n") {
			for lo := 0; lo <= len(line); lo += 5 {
				for _, hi := range []int{lo + 1, lo + 8} {
					got := tree.Spans(src, i, i+1, lo, hi)[0]
					for off := lo; off < min(hi, len(line)); off++ {
						if g, w := captureAt(got, off), captureAt(whole[i], off); g != w {
							t.Errorf("%s line %d, bytes %d to %d: byte %d is %q, want %q", c.file, i+1, lo, hi, off, g, w)
						}
					}
				}
			}
		}
	}
}

// Review fix: a line of a JSON-lines file holds a capture every two or
// three bytes, thousands in a record of a few KB, and its spans are
// resolved again for every frame. Resolving must take time about linear in
// a line's captures: quadratic, 10 lines of 10,000, as many as 20,000 bytes
// hold, under a capture as wide as a code block's, take seconds.
func TestResolvingLinesOfManyCapturesIsFast(t *testing.T) {
	const n = 10000
	segs := []segment{{start: 0, end: math.MaxInt, name: "none"}}
	for i := range n {
		segs = append(segs, segment{start: 2 * i, end: 2*i + 1, name: "string", rank: i + 1})
	}
	last := syntax.Span{Start: 2 * (n - 1), End: 2*n - 1, Capture: "string"}
	start := time.Now()
	for range 10 {
		if spans := resolve(segs); len(spans) != n || spans[n-1] != last {
			t.Fatalf("resolve gave %d spans; want %d, the last %v", len(spans), n, last)
		}
	}
	if d := time.Since(start); d > 250*time.Millisecond {
		t.Errorf("resolving 10 lines of %d captures took %v: it must take time about linear in them", n, d)
	}
}

// benchmark runs f on about 100 KB of each language's text in colored.
func benchmark(b *testing.B, f func(b *testing.B, name, text string)) {
	for _, c := range colored {
		text := strings.Repeat(c.text, 100<<10/len(c.text)+1)
		b.Run(c.file, func(b *testing.B) { f(b, c.file, text) })
	}
}

func BenchmarkParse(b *testing.B) {
	benchmark(b, func(b *testing.B, name, text string) {
		first, _, _ := strings.Cut(text, "\n")
		lang, _ := Detect(name, first)
		for b.Loop() {
			tree, err := lang.Parse([]byte(text))
			if err != nil {
				b.Fatal(err)
			}
			tree.Close()
		}
	})
}

func BenchmarkSpans(b *testing.B) {
	benchmark(b, func(b *testing.B, name, text string) {
		tree, src, mid := parse(b, name, text), []byte(text), strings.Count(text, "\n")/2
		for b.Loop() {
			tree.Spans(src, mid, mid+60, 0, math.MaxInt)
		}
	})
}
