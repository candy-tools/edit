package gitignore

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// checkIgnored fails the test unless m says of each path what want says,
// whether git ignores it, a path ending in "/" a folder.
func checkIgnored(t *testing.T, m *Matcher, want map[string]bool) {
	t.Helper()
	for p, w := range want {
		name, isDir := strings.CutSuffix(p, "/")
		if got := m.Ignored(name, isDir); got != w {
			t.Errorf("Ignored(%q, %t) = %t, want %t", name, isDir, got, w)
		}
	}
}

// gitignore returns a tree holding one .gitignore, at its root.
func gitignore(text string) fstest.MapFS {
	return fstest.MapFS{".gitignore": {Data: []byte(text)}}
}

func TestLinesThatHoldNoPattern(t *testing.T) {
	m := New(gitignore("\n# a.txt\n   \n\\#b.txt\n"), ".")
	checkIgnored(t, m, map[string]bool{"a.txt": false, "# a.txt": false, "#b.txt": true})
}

func TestTrailingSpaces(t *testing.T) {
	m := New(gitignore("a.txt  \nb\\ \n"), ".")
	checkIgnored(t, m, map[string]bool{"a.txt": true, "a.txt  ": false, "b ": true, "b": false})
}

func TestNegation(t *testing.T) {
	m := New(gitignore("*.log\n!keep.log\n\\!bang\n"), ".")
	checkIgnored(t, m, map[string]bool{"a.log": true, "keep.log": false, "x/keep.log": false, "!bang": true, "bang": false})
}

func TestNamesAnchorsAndFolders(t *testing.T) {
	m := New(gitignore("*.tmp\n/dist\ndocs/superpowers/\nbuild/\n"), ".")
	checkIgnored(t, m, map[string]bool{
		"a.tmp": true, "x/y/b.tmp": true, // a name, at any depth
		"dist/": true, "dist": true, "x/dist/": false, // anchored to the root
		"docs/superpowers/": true, "docs/superpowers": false, "x/docs/superpowers/": false, // anchored by its middle slash, folders only
		"build/": true, "x/build/": true, "build": false, // folders only, at any depth
	})
}

func TestDoubleStars(t *testing.T) {
	m := New(gitignore("**/cache\nlogs/**\na/**/b\n"), ".")
	checkIgnored(t, m, map[string]bool{
		"cache/": true, "x/y/cache": true,
		"logs/": false, "logs/x": true, "logs/x/y/": true,
		"a/b": true, "a/x/b": true, "a/x/y/b": true, "a/x": false, "b": false,
	})
}

func TestTheDeeperFileAndTheLaterLineDecide(t *testing.T) {
	m := New(fstest.MapFS{
		".gitignore":     {Data: []byte("*.gen\n!keep.gen\n")},
		"app/.gitignore": {Data: []byte("!*.gen\nkeep.gen\n/local\n")},
	}, ".")
	checkIgnored(t, m, map[string]bool{
		"x.gen": true, "keep.gen": false, // the root's lines: the later wins
		"app/x.gen": false, "app/keep.gen": true, // app's file overrides the root's
		"app/local/": true, "local/": false, "app/x/local/": false, // anchored to app
	})
}

func TestClasses(t *testing.T) {
	m := New(gitignore("[ab].txt\nx[!0-9]\ny[^0-9]\n*.[[:digit:]]\n[\n"), ".")
	checkIgnored(t, m, map[string]bool{
		"a.txt": true, "c.txt": false,
		"xa": true, "x1": false, "ya": true, "y1": false,
		"a.d]": false, "a.1": false, // a POSIX class: the pattern is dropped
		"[": false, // a pattern path.Match can't read
	})
}

func TestCRLFAndAByteOrderMark(t *testing.T) {
	m := New(gitignore("\xef\xbb\xbf*.log\r\nbuild/\r\n"), ".")
	checkIgnored(t, m, map[string]bool{"a.log": true, "build/": true, "build": false})
}

func TestTheExcludeFile(t *testing.T) {
	m := New(fstest.MapFS{
		".git/info/exclude": {Data: []byte("*.secret\n*.tmp\n")},
		".gitignore":        {Data: []byte("!keep.tmp\n")},
	}, ".")
	checkIgnored(t, m, map[string]bool{"a.secret": true, "x/a.tmp": true, "keep.tmp": false})
}

// countingFS counts how often each file of fsys is opened. It has Open
// alone, so fs.ReadFile and fs.Stat go through it.
type countingFS struct {
	fsys  fs.FS
	opens map[string]int
}

func (c countingFS) Open(name string) (fs.File, error) {
	c.opens[name]++
	return c.fsys.Open(name)
}

func TestAWorktreeHasNoExcludeFile(t *testing.T) {
	c := countingFS{fstest.MapFS{".git": {Data: []byte("gitdir: /elsewhere\n")}, ".gitignore": {Data: []byte("*.log\n")}}, map[string]int{}}
	if !New(c, ".").Ignored("a.log", false) {
		t.Error("a.log should be ignored")
	}
	if c.opens[".git/info/exclude"] != 0 {
		t.Error("with .git a file, the exclude file should not be read")
	}
}

// A file is checked, then read, each an Open of countingFS, so the count to
// hold is not one open. Each .gitignore is read once, then cached: the second
// and third rounds open nothing more than the first did.
func TestEachGitignoreIsReadOnce(t *testing.T) {
	c := countingFS{fstest.MapFS{".gitignore": {Data: []byte("*.log\n")}, "app/.gitignore": {Data: []byte("*.tmp\n")}}, map[string]int{}}
	m := New(c, ".")
	round := func() {
		m.Ignored("app/a.log", false)
		m.Ignored("app/b.tmp", false)
		m.Ignored("c.txt", false)
	}
	round()
	first := maps.Clone(c.opens)
	round()
	round()
	for _, name := range []string{".gitignore", "app/.gitignore"} {
		if first[name] == 0 {
			t.Errorf("%s was not read in the first round", name)
		}
		if c.opens[name] != first[name] {
			t.Errorf("%s was opened %d times in the first round and %d by the third, want no more", name, first[name], c.opens[name])
		}
	}
}

// pipeFS is a tree whose .gitignore is a named pipe, which fails the test
// when it is opened, as a real pipe's open would block until a writer came,
// or read: MapFS's ReadFile, which it would promote, opens the file below
// Open. Its Stat, MapFS's, opens nothing.
type pipeFS struct {
	fstest.MapFS
	t *testing.T
}

func (p pipeFS) Open(name string) (fs.File, error) {
	if name == ".gitignore" {
		p.t.Errorf("the named pipe %s was opened", name)
	}
	return p.MapFS.Open(name)
}

func (p pipeFS) ReadFile(name string) ([]byte, error) {
	if name == ".gitignore" {
		p.t.Errorf("the named pipe %s was read", name)
	}
	return p.MapFS.ReadFile(name)
}

func TestAPipeIsNeverOpened(t *testing.T) {
	m := New(pipeFS{fstest.MapFS{".gitignore": {Mode: fs.ModeNamedPipe}, "app/.gitignore": {Data: []byte("*.log\n")}}, t}, ".")
	checkIgnored(t, m, map[string]bool{"app/a.log": true, "a.log": false})
}

func TestAFolderBelowTheRoot(t *testing.T) {
	m := New(fstest.MapFS{
		".gitignore":     {Data: []byte("node_modules/\n/top.txt\n")},
		"sub/.gitignore": {Data: []byte("*.out\n")},
	}, "sub")
	checkIgnored(t, m, map[string]bool{
		"node_modules/": true, "x/node_modules/": true, // the root's patterns count below it
		"top.txt": false, // anchored to the root, not to sub
		"a.out":   true,
	})
}

func TestAFolderInsideAnIgnoredOne(t *testing.T) {
	m := New(fstest.MapFS{
		".git/info/exclude":     {Data: []byte("*.secret\n")},
		".gitignore":            {Data: []byte("vendor/**\n*.log\n")},
		"vendor/lib/.gitignore": {Data: []byte("*.tmp\n")},
	}, "vendor/lib")
	checkIgnored(t, m, map[string]bool{
		"main.go": false, "a.log": false, "a.secret": false, // read as the root: what is above doesn't count
		"a.tmp": true,
	})
}

// write writes text to the file at name, making the folders it is in.
func write(t *testing.T, name, text string) {
	t.Helper()
	mkdir(t, filepath.Dir(name))
	if err := os.WriteFile(name, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mkdir makes the folder at name and the folders it is in.
func mkdir(t *testing.T, name string) {
	t.Helper()
	if err := os.MkdirAll(name, 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestAgreesWithGit checks the Matcher against git itself, when git is
// installed: a working tree of .gitignore files and paths, and, for each
// path, whether git check-ignore says git ignores it. No path is in an
// ignored folder, where the Matcher, made for walks, doesn't look.
func TestAgreesWithGit(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	env := append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "HOME="+root, "XDG_CONFIG_HOME="+root)
	git := func(args ...string) error {
		cmd := exec.Command(gitPath, args...)
		cmd.Dir, cmd.Env = root, env
		return cmd.Run()
	}
	if err := git("init", "-q"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	for name, text := range map[string]string{
		".gitignore":        "*.log\n!keep.log\n/dist\nbuild/\n**/cache\nlogs/**\na/**/b\n[ab].txt\nx[!0-9]\nspace\\ \n# comment\n",
		"app/.gitignore":    "!*.log\n/local\ndeep/*.tmp\n",
		".git/info/exclude": "*.secret\n",
	} {
		write(t, filepath.Join(root, name), text)
	}
	paths := []string{
		"a.log", "keep.log", "app/b.log", "dist/", "app/dist/", "build/", "x/build/", "cache/", "x/y/cache/",
		"logs/", "logs/z", "a/b", "a/x/b", "a.txt", "c.txt", "xa", "x1", "space ", "app/local/", "local/",
		"app/deep/z.tmp", "z.tmp", "s.secret", "app/s.secret",
	}
	for _, p := range paths {
		if full := filepath.Join(root, filepath.FromSlash(p)); strings.HasSuffix(p, "/") {
			mkdir(t, full)
		} else {
			write(t, full, "")
		}
	}
	m := New(os.DirFS(root), ".")
	for _, p := range paths {
		name, isDir := strings.CutSuffix(p, "/")
		err := git("check-ignore", "-q", "--no-index", "--", name)
		var exit *exec.ExitError
		if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
			t.Fatalf("git check-ignore %q: %v", name, err)
		}
		if got, gitSays := m.Ignored(name, isDir), err == nil; got != gitSays {
			t.Errorf("Ignored(%q, %t) = %t, git says %t", name, isDir, got, gitSays)
		}
	}
}
