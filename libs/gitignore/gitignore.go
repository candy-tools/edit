// Package gitignore tells which files and folders of a git working tree git
// ignores, as its .gitignore files and its .git/info/exclude say, in the
// pattern format gitignore(5) describes. It reads them from an fs.FS as it
// needs them.
//
// It is app-agnostic and depends only on the standard library; a depguard
// rule in .golangci.yaml keeps it from importing the app.
package gitignore

import (
	"bytes"
	"io/fs"
	"path"
	"strings"
)

// Matcher tells which paths under a folder of a working tree git ignores.
// It reads each .gitignore it needs once, the first time it is asked about a
// path below it. It is not safe for concurrent use.
type Matcher struct {
	fsys    fs.FS
	dir     string               // the folder the paths asked about are in, from the root; "." for the root
	top     string               // the highest folder whose .gitignore counts: the root, or dir when dir lies in an ignored folder
	exclude []pattern            // the patterns of .git/info/exclude; none when top isn't the root
	folders map[string][]pattern // each folder's .gitignore patterns, by the folder's path from the root
}

// pattern is one pattern of a .gitignore.
type pattern struct {
	segments []string // the pattern split at "/", its leading and trailing "/" dropped
	negate   bool     // written with "!": it re-includes what it matches
	dirOnly  bool     // written with a trailing "/": it matches only folders
	anchored bool     // written with a "/" at its start or in its middle: it matches the whole path from its folder on
}

// New returns a Matcher for the paths under dir, slash-separated, in fsys,
// whose root is the working tree's: the folder that holds .git. dir may be
// a folder below the root, as a workspace opened in a subfolder of a
// repository is; the .gitignore files of the folders from the root down to
// dir then count too. A fsys that is no working tree, with no .git, works
// the same, without an exclude file. When dir lies in a folder the patterns
// above it ignore, as a workspace opened inside node_modules does, dir is
// read as if it were the root: only the .gitignore files of dir and below
// count, and no exclude file.
func New(fsys fs.FS, dir string) *Matcher {
	m := &Matcher{fsys: fsys, dir: path.Clean(dir), top: ".", folders: map[string][]pattern{}}
	if fi, err := fs.Stat(fsys, ".git"); err == nil && fi.IsDir() {
		m.exclude = m.read(".git/info/exclude")
	}
	if m.dir != "." && m.inIgnored() {
		m.top, m.exclude = m.dir, nil
	}
	return m
}

// inIgnored reports whether dir, or a folder above it, is ignored by the
// patterns above it.
func (m *Matcher) inIgnored() bool {
	parts := strings.Split(m.dir, "/")
	for i := range parts {
		if m.decides(strings.Join(parts[:i+1], "/"), true) {
			return true
		}
	}
	return false
}

// Ignored reports whether git ignores the file or, when isDir is set, the
// folder at p, slash-separated and relative to dir: whether the pattern
// that decides it ignores it rather than negates. It doesn't look at the
// folders above p: it is for a walk that skips an ignored folder rather
// than look inside it, which is how git reads the patterns too — a file in
// an ignored folder can't be re-included.
func (m *Matcher) Ignored(p string, isDir bool) bool {
	return m.decides(path.Join(m.dir, p), isDir)
}

// decides reports whether the patterns ignore full, a path from the root:
// the last pattern that matches it decides, looking from the .gitignore of
// the folder that holds it up to top's, then in the exclude file.
func (m *Matcher) decides(full string, isDir bool) bool {
	for d := path.Dir(full); ; d = path.Dir(d) {
		rel := full
		if d != "." {
			rel = full[len(d)+1:]
		}
		if ignored, ok := last(m.patterns(d), rel, isDir); ok {
			return ignored
		}
		if d == m.top || d == "." {
			break
		}
	}
	ignored, _ := last(m.exclude, full, isDir)
	return ignored
}

// last returns whether the last of patterns that matches p, a path from
// their folder, ignores it, and false for ok when none matches.
func last(patterns []pattern, p string, isDir bool) (ignored, ok bool) {
	for i := len(patterns) - 1; i >= 0; i-- {
		if patterns[i].matches(p, isDir) {
			return !patterns[i].negate, true
		}
	}
	return false, false
}

// patterns returns the patterns of the .gitignore of the folder dir, a path
// from the root, reading it the first time.
func (m *Matcher) patterns(dir string) []pattern {
	ps, ok := m.folders[dir]
	if !ok {
		ps = m.read(path.Join(dir, ".gitignore"))
		m.folders[dir] = ps
	}
	return ps
}

// bom is the UTF-8 byte order mark, which git drops from a file's start.
var bom = []byte("\xef\xbb\xbf")

// read reads the patterns of the file at name: none when it can't be read,
// or isn't a regular file, such as a named pipe, which is never opened. A
// "\r" before each "\n" is dropped, as git drops it.
func (m *Matcher) read(name string) []pattern {
	if fi, err := fs.Stat(m.fsys, name); err != nil || !fi.Mode().IsRegular() {
		return nil
	}
	data, err := fs.ReadFile(m.fsys, name)
	if err != nil {
		return nil
	}
	var ps []pattern
	for _, line := range strings.Split(string(bytes.TrimPrefix(data, bom)), "\n") {
		if p, ok := parse(strings.TrimSuffix(line, "\r")); ok {
			ps = append(ps, p)
		}
	}
	return ps
}

// parse reads line as a pattern, as gitignore(5) says, and reports false
// for a line that holds none — a blank line, a comment — and for a pattern
// that would never match: one path.Match can't read, or one with a POSIX
// class. A backslash escapes the character after it, as path.Match reads
// it: "\#" and "\!" start a pattern with "#" and "!".
func parse(line string) (pattern, bool) {
	line = trimTrailingSpaces(line)
	if line == "" || line[0] == '#' {
		return pattern{}, false
	}
	var p pattern
	if line[0] == '!' {
		p.negate, line = true, line[1:]
	}
	if rest, ok := strings.CutSuffix(line, "/"); ok {
		p.dirOnly, line = true, rest
	}
	p.anchored = strings.Contains(line, "/")
	line = strings.TrimPrefix(line, "/")
	if line == "" {
		return pattern{}, false
	}
	p.segments = strings.Split(line, "/")
	for i, s := range p.segments {
		s, ok := classes(s)
		if _, err := path.Match(s, ""); !ok || err != nil {
			return pattern{}, false
		}
		p.segments[i] = s
	}
	return p, true
}

// trimTrailingSpaces drops the spaces at line's end, but one escaped with a
// backslash and those before it.
func trimTrailingSpaces(line string) string {
	end := len(line)
	for end > 0 && line[end-1] == ' ' && (end < 2 || line[end-2] != '\\') {
		end--
	}
	return line[:end]
}

// classes returns segment with each bracket expression's "[!" written as
// path.Match's "[^", and false when one holds a POSIX class, such as
// [[:digit:]], which path.Match would read as a class of "[", ":", "d"… and
// a "]".
func classes(segment string) (string, bool) {
	b := []byte(segment)
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '\\':
			i++ // the escaped byte is itself
		case '[':
			j := i + 1
			if j < len(b) && (b[j] == '!' || b[j] == '^') {
				b[j] = '^'
				j++
			}
			if j < len(b) && b[j] == ']' {
				j++ // a "]" first in a class is itself
			}
			for ; j < len(b) && b[j] != ']'; j++ {
				switch {
				case b[j] == '\\':
					j++
				case b[j] == '[' && j+1 < len(b) && b[j+1] == ':':
					return "", false
				}
			}
			i = j
		}
	}
	return string(b), true
}

// matches reports whether p matches rel, a path from p's folder: a folder
// when isDir is set.
func (p pattern) matches(rel string, isDir bool) bool {
	if p.dirOnly && !isDir {
		return false
	}
	if !p.anchored {
		ok, _ := path.Match(p.segments[0], path.Base(rel))
		return ok
	}
	return segmentsMatch(p.segments, strings.Split(rel, "/"))
}

// segmentsMatch reports whether the segments of a pattern match those of a
// name, a "**" standing for any number of the name's: one or more at the
// pattern's end, none or more elsewhere.
func segmentsMatch(pattern, name []string) bool {
	switch {
	case len(pattern) == 0:
		return len(name) == 0
	case pattern[0] == "**" && len(pattern) == 1:
		return len(name) > 0
	case pattern[0] == "**":
		for i := range len(name) + 1 {
			if segmentsMatch(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	case len(name) == 0:
		return false
	}
	ok, _ := path.Match(pattern[0], name[0])
	return ok && segmentsMatch(pattern[1:], name[1:])
}
