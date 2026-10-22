// Package treesitter is a syntax engine on tree-sitter grammars, run in pure
// Go by gotreesitter. It colors with its own highlight queries, embedded in
// the package, and colors Markdown's fenced code and inline markup through
// injections. It is app-agnostic.
//
// A build embeds the grammars that gotreesitter's build tags name, and every
// grammar of the languages Languages lists must be among them: see the
// GRAMMARS of edit's Makefile.
package treesitter

import (
	"embed"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/candy-tools/edit/libs/syntax"
	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// timeout is how long a parse may run before it stops, in microseconds: 2 s.
// A variable, so a test can make a parse stop.
var timeout uint64 = 2_000_000

// queries holds edit's highlight and injection queries, one folder per
// grammar.
//
//go:embed queries
var queries embed.FS

// The grammars edit colors with. Markdown's inline syntax only ever comes
// injected into Markdown.
var (
	golang     = &language{name: "go"}
	gomod      = &language{name: "gomod"}
	yaml       = &language{name: "yaml"}
	json       = &language{name: "json"}
	bash       = &language{name: "bash"}
	makefile   = &language{name: "make"}
	javascript = &language{name: "javascript"}
	python     = &language{name: "python"}
	java       = &language{name: "java"}
	inline     = &language{name: "markdown_inline"}
	markdown   = &language{name: "markdown", injects: map[string]injection{
		"markdown_inline": {inline, false},
		"go":              {golang, true},
		"golang":          {golang, true},
		"yaml":            {yaml, true},
		"yml":             {yaml, true},
		"json":            {json, true},
		"sh":              {bash, true},
		"shell":           {bash, true},
		"bash":            {bash, true},
		"make":            {makefile, true},
		"js":              {javascript, true},
		"javascript":      {javascript, true},
		"py":              {python, true},
		"python":          {python, true},
		"java":            {java, true},
	}}
)

// detectable are the languages a file can be in, by their grammar's name in
// gotreesitter's registry.
var detectable = map[string]*language{}

func init() {
	for _, l := range []*language{golang, gomod, yaml, json, markdown, bash, makefile, javascript, python, java} {
		detectable[l.name] = l
	}
}

// Detect returns the language of a file called name, whose first line is
// firstLine, and whether it knows it. gotreesitter's registry finds the
// language by the name or, when that finds none of the languages Languages
// lists, by a shebang on the first line.
func Detect(name, firstLine string) (syntax.Language, bool) {
	for _, e := range []*grammars.LangEntry{grammars.DetectLanguage(name), grammars.DetectLanguageByShebang(firstLine)} {
		if l, ok := detectable[entryName(e)]; ok {
			return l, true
		}
	}
	return nil, false
}

// entryName is the name of a registry entry, "" for none.
func entryName(e *grammars.LangEntry) string {
	if e == nil {
		return ""
	}
	return e.Name
}

// Languages returns the names of the languages Detect knows, sorted.
func Languages() []string {
	return slices.Sorted(maps.Keys(detectable))
}

// language is a grammar of gotreesitter's registry and edit's highlight
// query for it, loaded the first time it parses. A language can inject
// others into parts of its text, as its injection query says.
type language struct {
	name    string               // the grammar's name, and its queries' folder
	injects map[string]injection // the languages its injection query can name, by name; nil for none

	once   sync.Once
	err    error
	lang   *ts.Language
	query  *ts.Query
	tokens func(src []byte, lang *ts.Language) ts.TokenSource // the grammar's own token source; nil for none
	inject string                                             // the injection query
}

// injection is a language a host injects into parts of its text, and
// whether it owns those parts, so that the host's colors stop there, as a
// code block's language does, or adds to the host's colors, as Markdown's
// inline syntax does to a heading's.
type injection struct {
	lang *language
	owns bool
}

// Name returns the grammar's name.
func (l *language) Name() string { return l.name }

// load loads l's grammar and compiles its queries, and those of the
// languages it injects, once.
func (l *language) load() error {
	l.once.Do(func() { l.err = l.compile() })
	return l.err
}

func (l *language) compile() error {
	e := grammars.DetectLanguageByName(l.name)
	if e == nil || e.Language == nil {
		return fmt.Errorf("treesitter: the %s grammar is not in this build", l.name)
	}
	lang := e.Language()
	if lang == nil {
		return fmt.Errorf("treesitter: the %s grammar didn't load", l.name)
	}
	src, err := queries.ReadFile("queries/" + l.name + "/highlights.scm")
	if err != nil {
		return err
	}
	q, err := ts.NewQuery(string(src), lang)
	if err != nil {
		return fmt.Errorf("treesitter: %s highlights: %w", l.name, err)
	}
	l.lang, l.query, l.tokens = lang, q, e.TokenSourceFactory
	if l.injects == nil {
		return nil
	}
	inject, err := queries.ReadFile("queries/" + l.name + "/injections.scm")
	if err != nil {
		return err
	}
	l.inject = string(inject)
	for _, inj := range l.injects {
		if err := inj.lang.load(); err != nil {
			return err
		}
	}
	return nil
}

// Parse parses text: a tree, or an error and no tree, after a parse that
// stops at the timeout.
func (l *language) Parse(text []byte) (syntax.Tree, error) {
	if err := l.load(); err != nil {
		return nil, err
	}
	if l.injects != nil {
		return l.parseInjected(text)
	}
	p := ts.NewParser(l.lang)
	p.SetTimeoutMicros(timeout)
	var t *ts.Tree
	var err error
	if l.tokens != nil {
		t, err = p.ParseWithTokenSource(text, l.tokens(text, l.lang))
	} else {
		t, err = p.Parse(text)
	}
	if err != nil {
		return nil, err
	}
	if err := stopped(t); err != nil {
		t.Release()
		return nil, err
	}
	return &tree{layers: []layer{{lang: l, tree: t}}}, nil
}

// parseInjected parses text as l, then each part of it l injects a language
// into, each in a tree of its own. A fresh InjectionParser does it, since one
// frees the trees of its last parse when it parses again.
func (l *language) parseInjected(text []byte) (syntax.Tree, error) {
	ip := ts.NewInjectionParser()
	ip.SetTimeoutMicros(timeout)
	ip.RegisterLanguage(l.name, l.lang)
	for name, inj := range l.injects {
		ip.RegisterLanguage(name, inj.lang.lang)
	}
	if err := ip.RegisterInjectionQuery(l.name, l.inject); err != nil {
		return nil, err
	}
	res, err := ip.Parse(text, l.name)
	if err != nil {
		return nil, err
	}
	t := &tree{layers: []layer{{lang: l, tree: res.Tree}}}
	for _, r := range res.Injections {
		if inj, ok := l.injects[r.Language]; ok && r.Tree != nil {
			t.layers = append(t.layers, layer{lang: inj.lang, tree: r.Tree, injected: true, owns: inj.owns})
		}
	}
	if err := stopped(res.Tree); err != nil {
		t.Close()
		return nil, err
	}
	return t, nil
}

// stopped returns why the parse that built t stopped before the end of the
// text, at the timeout say, or nil when it didn't.
func stopped(t *ts.Tree) error {
	if !t.ParseStoppedEarly() {
		return nil
	}
	return fmt.Errorf("treesitter: the parse stopped early: %s", t.ParseStopReason())
}
