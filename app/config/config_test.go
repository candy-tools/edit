package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// shortcutMap is the shape of Config.Shortcuts: scope → action name → keys.
type shortcutMap = map[string]map[string][]string

// load writes text to a config file in a new temporary directory and loads
// it.
func load(t *testing.T, text string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestLoadAMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.yaml")
	_, err := Load(path)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load = %v, want an error satisfying fs.ErrNotExist", err)
	}
	if strings.Contains(err.Error(), "nope.yaml") {
		t.Errorf("error %q repeats the path, which the caller adds", err)
	}
}

// A file trazo can't read gives the cause alone, without the path.
func TestLoadAnUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs file permissions that root doesn't bypass")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("shortcuts:\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || err.Error() != "permission denied" {
		t.Errorf("Load = %v, want permission denied", err)
	}
	if _, err := Load(t.TempDir()); err == nil || err.Error() != "is a directory" {
		t.Errorf("Load of a directory = %v, want is a directory", err)
	}
}

func TestLoadNothingToRead(t *testing.T) {
	for _, text := range []string{
		"",
		"# only a comment\n",
		"~\n",
		"shortcuts:\n",
		"shortcuts: {}\n",
		"shortcuts:\n  # sidebar:\n  #   down: [down, j]\n",
		"---\n",
		"---\n---\n",
	} {
		cfg, err := load(t, text)
		if err != nil || cfg.Shortcuts != nil {
			t.Errorf("%q: Load = %v, %v; want no shortcuts", text, cfg.Shortcuts, err)
		}
	}
}

func TestLoad(t *testing.T) {
	for _, c := range []struct {
		name, text string
		want       shortcutMap
	}{
		{"the example in the README", `shortcuts:
  global:
    next-tab: alt+right        # a single key needs no brackets
    previous-tab: alt+left
  sidebar:
    down: [down, j]
    up: [up, k]
    collapse: [left, h]
    expand: [right, l]
  editor:
    down: [j, down]            # the first key is the one the footer shows
    up: [k, up]
`, shortcutMap{
			"global":  {"next-tab": {"alt+right"}, "previous-tab": {"alt+left"}},
			"sidebar": {"down": {"down", "j"}, "up": {"up", "k"}, "collapse": {"left", "h"}, "expand": {"right", "l"}},
			"editor":  {"down": {"j", "down"}, "up": {"k", "up"}},
		}},
		{"an empty list", "shortcuts:\n  global:\n    close-tab: []\n",
			shortcutMap{"global": {"close-tab": {}}}},
		{"an empty scope", "shortcuts:\n  sidebar:\n    # down: [down, j]\n",
			shortcutMap{"sidebar": {}}},
		{"keys as written", "shortcuts:\n  sidebar:\n    down: [1, \"#\", 'null', true, \" \"]\n",
			shortcutMap{"sidebar": {"down": {"1", "#", "null", "true", " "}}}},
		{"a list on several lines", "shortcuts:\n  sidebar:\n    down:\n      - down\n      - j\n",
			shortcutMap{"sidebar": {"down": {"down", "j"}}}},
		{"an alias to a list", "shortcuts:\n  sidebar:\n    down: &down [down, j]\n  editor:\n    down: *down\n",
			shortcutMap{"sidebar": {"down": {"down", "j"}}, "editor": {"down": {"down", "j"}}}},
		{"aliases to keys", "shortcuts:\n  sidebar:\n    up: &k k\n    down: [*k, j]\n",
			shortcutMap{"sidebar": {"up": {"k"}, "down": {"k", "j"}}}},
		{"an alias to a scope", "shortcuts:\n  sidebar: &nav\n    down: j\n  editor: *nav\n",
			shortcutMap{"sidebar": {"down": {"j"}}, "editor": {"down": {"j"}}}},
		{"document markers", "---\nshortcuts:\n  sidebar:\n    down: j\n...\n",
			shortcutMap{"sidebar": {"down": {"j"}}}},
		{"an empty document after it", "shortcuts:\n  sidebar:\n    down: j\n---\n# nothing more\n",
			shortcutMap{"sidebar": {"down": {"j"}}}},
	} {
		cfg, err := load(t, c.text)
		if err != nil || !reflect.DeepEqual(cfg.Shortcuts, c.want) {
			t.Errorf("%s: Load = %v, %v; want %v", c.name, cfg.Shortcuts, err, c.want)
		}
	}
}

// Review focus: a file saved with Windows line endings, or by an editor that
// writes a byte-order mark, loads like any other.
func TestLoadWindowsLineEndingsAndAByteOrderMark(t *testing.T) {
	want := shortcutMap{"sidebar": {"down": {"j", "down"}}}
	for _, text := range []string{
		"shortcuts:\r\n  sidebar:\r\n    down: [j, down]\r\n",
		"\xef\xbb\xbfshortcuts:\n  sidebar:\n    down: [j, down]\n",
	} {
		cfg, err := load(t, text)
		if err != nil || !reflect.DeepEqual(cfg.Shortcuts, want) {
			t.Errorf("%q: Load = %v, %v; want %v", text, cfg.Shortcuts, err, want)
		}
	}
}

func TestLoadWrongShapes(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"- shortcuts\n", "want a mapping of settings"},
		{"shortcuts\n", "want a mapping of settings"},
		{"? [shortcuts]\n: {}\n", "want a mapping of settings"},
		{"shorcuts: {}\n", `unknown setting "shorcuts"`},
		{"shortcuts: [sidebar]\n", "shortcuts: want a mapping of scopes"},
		{"shortcuts: sidebar\n", "shortcuts: want a mapping of scopes"},
		{"shortcuts:\n  sidebar: [down]\n", "shortcuts.sidebar: want a mapping of actions"},
		{"shortcuts:\n  sidebar:\n    down:\n", "shortcuts.sidebar.down: want a key or a list of keys"},
		{"shortcuts:\n  sidebar:\n    down: ~\n", "shortcuts.sidebar.down: want a key or a list of keys"},
		{"shortcuts:\n  sidebar:\n    down: {j: k}\n", "shortcuts.sidebar.down: want a key or a list of keys"},
		{"shortcuts:\n  sidebar:\n    down: [j, ~]\n", "shortcuts.sidebar.down: want a key or a list of keys"},
		{"shortcuts:\n  sidebar:\n    down: [j, [k]]\n", "shortcuts.sidebar.down: want a key or a list of keys"},
		{"shortcuts: {}\nshortcuts: {}\n", "shortcuts: listed twice"},
		{"shortcuts:\n  sidebar: {}\n  sidebar: {}\n", "shortcuts.sidebar: listed twice"},
		{"shortcuts:\n  sidebar:\n    down: j\n    down: k\n", "shortcuts.sidebar.down: listed twice"},
	} {
		if _, err := load(t, c.text); err == nil || err.Error() != c.want {
			t.Errorf("%q: Load = %v, want %q", c.text, err, c.want)
		}
	}
}

func TestLoadASyntaxError(t *testing.T) {
	_, err := load(t, "shortcuts:\n  sidebar:\n    down: [down, j\n")
	if err == nil || !strings.HasPrefix(err.Error(), "yaml: line ") {
		t.Errorf("Load = %v, want the YAML parser's error with its line", err)
	}
}

// Review fix: trazo reads one document, so another one that holds anything,
// even a mistake, stops it rather than being ignored without a word.
func TestLoadAFileWithASecondDocument(t *testing.T) {
	for _, text := range []string{
		"shortcuts: {}\n---\nshortcuts:\n  sidebar:\n    down: j\n",
		"---\nshortcuts: {}\n...\n---\nx: 1\n",
		"shortcuts: {}\n---\n---\nshortcuts: {}\n", // after an empty one
	} {
		if _, err := load(t, text); err == nil || err.Error() != "want one document" {
			t.Errorf("%q: Load = %v, want %q", text, err, "want one document")
		}
	}
	_, err := load(t, "shortcuts: {}\n---\n[j\n")
	if err == nil || !strings.HasPrefix(err.Error(), "yaml: line ") {
		t.Errorf("a mistake in a second document: Load = %v, want the YAML parser's error", err)
	}
}

func TestDefaultPath(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("the config directory is found differently here")
	}
	home, xdg := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	want := filepath.Join(xdg, "trazo", "config.yaml")
	if runtime.GOOS == "darwin" {
		want = filepath.Join(home, "Library", "Application Support", "trazo", "config.yaml")
	}
	if got, err := DefaultPath(); err != nil || got != want {
		t.Errorf("DefaultPath = %q, %v; want %q", got, err, want)
	}
}

func TestDefaultPathWithoutAConfigDirectory(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("the config directory is found differently here")
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if got, err := DefaultPath(); err == nil {
		t.Errorf("DefaultPath = %q, want an error", got)
	}
}

func TestLoadTheTheme(t *testing.T) {
	for text, want := range map[string]string{
		"theme: dracula\n":                "dracula",
		"theme: \"candy-light\"\n":        "candy-light",
		"shortcuts: {}\ntheme: gruvbox\n": "gruvbox",
		"theme: 7\n":                      "7", // read as written: which themes exist is for the editor to say
	} {
		if cfg, err := load(t, text); err != nil || cfg.Theme != want {
			t.Errorf("%q: Load = %q, %v; want theme %q", text, cfg.Theme, err, want)
		}
	}
}

func TestLoadTheMouse(t *testing.T) {
	for text, want := range map[string]bool{
		"mouse: true\n":               true,
		"mouse: True\n":               true,
		"mouse: false\n":              false,
		"theme: dracula\n":            false, // off unless the file turns it on
		"mouse: true\ntheme: candy\n": true,
	} {
		if cfg, err := load(t, text); err != nil || cfg.Mouse != want {
			t.Errorf("%q: Load = %t, %v; want mouse %t", text, cfg.Mouse, err, want)
		}
	}
}

func TestLoadAMouseThatIsNeitherTrueNorFalse(t *testing.T) {
	for _, text := range []string{"mouse:\n", "mouse: ~\n", "mouse: yes\n", "mouse: 1\n", "mouse: \"true\"\n", "mouse: [true]\n"} {
		if _, err := load(t, text); err == nil || err.Error() != "mouse: want true or false" {
			t.Errorf("%q: Load = %v, want mouse: want true or false", text, err)
		}
	}
}

func TestLoadAThemeThatIsNoName(t *testing.T) {
	for _, text := range []string{"theme:\n", "theme: ~\n", "theme: [dracula]\n", "theme: {name: dracula}\n"} {
		if _, err := load(t, text); err == nil || err.Error() != "theme: want a theme name" {
			t.Errorf("%q: Load = %v, want theme: want a theme name", text, err)
		}
	}
	if _, err := load(t, "theme: dracula\nthemes: candy\n"); err == nil || err.Error() != `unknown setting "themes"` {
		t.Errorf("a misspelt setting next to the theme: Load = %v, want it unknown", err)
	}
}
