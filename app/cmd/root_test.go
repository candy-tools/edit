package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/candy-tools/trazo/app/tui"
	"github.com/candy-tools/trazo/libs/keymap"
	"github.com/charmbracelet/colorprofile"
)

// stubTUI swaps the TUI launcher for fn for the duration of the test.
func stubTUI(t *testing.T, fn func(root string, keys *keymap.Map, theme tui.Theme, colors colorprofile.Profile, mouse bool) error) {
	t.Helper()
	orig := runTUI
	runTUI = fn
	t.Cleanup(func() { runTUI = orig })
}

// noUserConfig points trazo's default config file at a path in an empty
// temporary directory, so a developer's own config file is never read or
// written, on any OS, and returns that path.
func noUserConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trazo", "config.yaml")
	stubConfigPath(t, func() (string, error) { return path, nil })
	return path
}

// noConfigDir makes trazo find no config directory, as when $HOME is unset.
func noConfigDir(t *testing.T) {
	t.Helper()
	stubConfigPath(t, func() (string, error) { return "", errors.New("no config directory") })
}

// stubConfigPath swaps the lookup of the default config path for fn for the
// duration of the test.
func stubConfigPath(t *testing.T, fn func() (string, error)) {
	t.Helper()
	orig := defaultConfigPath
	defaultConfigPath = fn
	t.Cleanup(func() { defaultConfigPath = orig })
}

// writeFile writes text to the file at path, creating its directory.
func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// started is what the TUI started with.
type started struct {
	keys   *keymap.Map
	theme  tui.Theme
	colors colorprofile.Profile
	mouse  bool
}

// execute runs trazo with args, the TUI stubbed, and returns what the TUI
// started with, nil if it didn't start, and the command's error.
func execute(t *testing.T, args ...string) (*started, error) {
	t.Helper()
	var s *started
	stubTUI(t, func(_ string, keys *keymap.Map, theme tui.Theme, colors colorprofile.Profile, mouse bool) error {
		s = &started{keys: keys, theme: theme, colors: colors, mouse: mouse}
		return nil
	})
	c := newRootCommand()
	c.SetArgs(args)
	err := c.Execute()
	return s, err
}

// terminal makes trazo see a terminal of color profile p until the test
// ends: a test's own output is no terminal.
func terminal(t *testing.T, p colorprofile.Profile) {
	t.Helper()
	orig := terminalProfile
	terminalProfile = func() colorprofile.Profile { return p }
	t.Cleanup(func() { terminalProfile = orig })
}

// sidebarDown is the action the config tests rebind.
var sidebarDown = keymap.ID{Scope: "sidebar", Name: "down"}

func TestRootArgs(t *testing.T) {
	c := newRootCommand()
	if err := c.Args(c, nil); err != nil {
		t.Errorf("no argument should be valid (opens the current directory): %v", err)
	}
	if err := c.Args(c, []string{"a.txt"}); err == nil {
		t.Error("expected an error with an argument")
	}
}

func TestRootMeta(t *testing.T) {
	c := newRootCommand()
	if c.Use == "" || c.Short == "" {
		t.Error("the root command should set Use and Short")
	}
}

func TestRootOpensWorkingDir(t *testing.T) {
	noUserConfig(t)
	var got string
	stubTUI(t, func(root string, _ *keymap.Map, _ tui.Theme, _ colorprofile.Profile, _ bool) error {
		got = root
		return nil
	})
	c := newRootCommand()
	c.SetArgs([]string{})
	if err := c.Execute(); err != nil {
		t.Fatalf("root command failed: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if got != wd {
		t.Errorf("TUI started on %q, want the working directory %q", got, wd)
	}
}

func TestRootReturnsTUIError(t *testing.T) {
	noUserConfig(t)
	boom := errors.New("boom")
	stubTUI(t, func(string, *keymap.Map, tui.Theme, colorprofile.Profile, bool) error { return boom })
	c := newRootCommand()
	c.SetArgs([]string{})
	if err := c.Execute(); !errors.Is(err, boom) {
		t.Errorf("Execute() = %v, want the TUI's error", err)
	}
}

func TestRootWithoutAConfigFileUsesTheDefaults(t *testing.T) {
	terminal(t, colorprofile.ANSI256)
	noUserConfig(t)
	s, err := execute(t)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.keys.Keys(sidebarDown); !slices.Equal(got, []string{"down"}) {
		t.Errorf("sidebar.down: keys %q, want the default down", got)
	}
	if got := s.theme.Name(); got != "candy" {
		t.Errorf("theme %q, want the default candy", got)
	}
	if s.mouse {
		t.Error("the mouse is on, want it off by default")
	}
}

func TestRootWithoutAConfigDirectoryUsesTheDefaults(t *testing.T) {
	terminal(t, colorprofile.ANSI256)
	noConfigDir(t)
	s, err := execute(t)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.keys.Keys(sidebarDown); !slices.Equal(got, []string{"down"}) {
		t.Errorf("sidebar.down: keys %q, want the default down", got)
	}
	if got := s.theme.Name(); got != "candy" {
		t.Errorf("theme %q, want the default candy", got)
	}
}

func TestRootReadsTheConfigFile(t *testing.T) {
	terminal(t, colorprofile.ANSI256)
	writeFile(t, noUserConfig(t), "shortcuts:\n  sidebar:\n    down: [j, down]\ntheme: dracula\nmouse: true\n")
	s, err := execute(t)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.keys.Keys(sidebarDown); !slices.Equal(got, []string{"j", "down"}) {
		t.Errorf("sidebar.down: keys %q, want j, down from the config file", got)
	}
	if got := s.theme.Name(); got != "dracula" {
		t.Errorf("theme %q, want dracula from the config file", got)
	}
	if !s.mouse {
		t.Error("the mouse is off, want it on from the config file")
	}
}

// Review focus: on a terminal without colors, or with NO_COLOR set, trazo
// draws no colors whatever the config file says; nor when its output is no
// terminal, or TERM is dumb.
func TestRootWithoutColorsUsesTheThemeNone(t *testing.T) {
	for _, p := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.NoTTY} {
		terminal(t, p)
		writeFile(t, noUserConfig(t), "theme: dracula\n")
		s, err := execute(t)
		if err != nil {
			t.Fatal(err)
		}
		if got := s.theme.Name(); got != "none" {
			t.Errorf("%v: theme %q, want none on a terminal without colors", p, got)
		}
	}
}

// Review focus: the TUI draws on the colors the theme was picked for, so the
// renderer and the theme never disagree.
func TestTheTUIRunsOnTheColorsTheThemeWasPickedFor(t *testing.T) {
	for _, p := range []colorprofile.Profile{colorprofile.ANSI256, colorprofile.ASCII} {
		terminal(t, p)
		noUserConfig(t)
		s, err := execute(t)
		if err != nil {
			t.Fatal(err)
		}
		if s.colors != p {
			t.Errorf("on a %v terminal the TUI runs on %v", p, s.colors)
		}
	}
}

// Review focus: colors are read as Bubble Tea v1 read them — no colors with
// NO_COLOR set to anything, or CLICOLOR=0 — and a real terminal whose TERM
// is dumb or unset still shows text attributes, the cursor's reverse video
// among them.
func TestColorsAreReadAsV1ReadThem(t *testing.T) {
	for _, c := range []struct {
		name string
		env  []string
		tty  bool
		want colorprofile.Profile
	}{
		{"a 256-color terminal", []string{"TERM=xterm-256color"}, true, colorprofile.ANSI256},
		{"NO_COLOR=1", []string{"TERM=xterm-256color", "NO_COLOR=1"}, true, colorprofile.ASCII},
		{"NO_COLOR set to anything", []string{"TERM=xterm-256color", "NO_COLOR=yes"}, true, colorprofile.ASCII},
		{"CLICOLOR=0", []string{"TERM=xterm-256color", "CLICOLOR=0"}, true, colorprofile.ASCII},
		{"CLICOLOR=0, but forced", []string{"TERM=xterm-256color", "CLICOLOR=0", "CLICOLOR_FORCE=1"}, true, colorprofile.ANSI256},
		{"TERM=dumb on a terminal", []string{"TERM=dumb"}, true, colorprofile.ASCII},
		{"no TERM on a terminal", nil, true, colorprofile.ASCII},
		{"no terminal", []string{"TERM=xterm-256color"}, false, colorprofile.NoTTY},
		{"no terminal, NO_COLOR set", []string{"TERM=xterm-256color", "NO_COLOR=yes"}, false, colorprofile.NoTTY},
	} {
		env := c.env
		if c.tty {
			env = append(slices.Clone(env), "TTY_FORCE=1") // colorprofile's way to see a terminal in a test
		}
		if got := colorsOf(io.Discard, env, c.tty); got != c.want {
			t.Errorf("%s: colors %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRootReadsTheFileTheConfigFlagNames(t *testing.T) {
	writeFile(t, noUserConfig(t), "shortcuts:\n  sidebar:\n    down: [k]\n")
	path := filepath.Join(t.TempDir(), "trazo.yaml")
	writeFile(t, path, "shortcuts:\n  sidebar:\n    down: [j]\n")
	s, err := execute(t, "--config", path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.keys.Keys(sidebarDown); !slices.Equal(got, []string{"j"}) {
		t.Errorf("sidebar.down: keys %q, want j from the file --config names", got)
	}
}

func TestRootStopsOnAConfigMistake(t *testing.T) {
	for _, c := range []struct {
		name string
		text string // the config file's text, "" for no file
		flag bool   // whether --config names the file, rather than it being at the default path
		want string // %s is the file's path
	}{
		{"a missing file named with --config", "", true, "config %s: no such file or directory"},
		{"the wrong shape", "shorcuts: {}\n", true, `config %s: unknown setting "shorcuts"`},
		{"an invalid keymap", "shortcuts:\n  global:\n    quit: []\n  sidebar:\n    donw: j\n", true,
			"config %s: invalid shortcuts:\n  global.quit: needs at least one key\n  sidebar.donw: unknown action"},
		{"a mistake in the file at the default path", "shortcuts: [j]\n", false,
			"config %s: shortcuts: want a mapping of scopes"},
		{"an unknown theme", "theme: monokai\n", false,
			`config %s: unknown theme "monokai": want candy, candy-light, dracula, gruvbox or none`},
		{"a theme that is no name", "theme: [dracula]\n", true, "config %s: theme: want a theme name"},
	} {
		path := noUserConfig(t)
		var args []string
		if c.flag {
			path = filepath.Join(t.TempDir(), "trazo.yaml")
			args = []string{"--config", path}
		}
		if c.text != "" {
			writeFile(t, path, c.text)
		}
		s, err := execute(t, args...)
		if want := fmt.Sprintf(c.want, path); err == nil || err.Error() != want {
			t.Errorf("%s: Execute() = %v, want\n%s", c.name, err, want)
		}
		if s != nil {
			t.Errorf("%s: the TUI started", c.name)
		}
	}
}

// Review focus: a config file at the default location that trazo can't read
// stops it with the cause, rather than leaving it on the default keys.
func TestRootStopsOnAnUnreadableConfigFile(t *testing.T) {
	path := noUserConfig(t)
	if err := os.MkdirAll(path, 0o700); err != nil { // a folder where the file should be
		t.Fatal(err)
	}
	s, err := execute(t)
	if want := "config " + path + ": is a directory"; err == nil || err.Error() != want {
		t.Errorf("Execute() = %v, want %q", err, want)
	}
	if s != nil {
		t.Error("the TUI started")
	}
}

func TestRootStopsOnAConfigSyntaxError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trazo.yaml")
	writeFile(t, path, "shortcuts:\n  sidebar: [down\n")
	s, err := execute(t, "--config", path)
	if want := "config " + path + ": yaml: line "; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("Execute() = %v, want an error starting %q", err, want)
	}
	if s != nil {
		t.Error("the TUI started")
	}
}

func TestRootHelpNamesTheDefaultConfigFile(t *testing.T) {
	path := noUserConfig(t)
	var out bytes.Buffer
	c := newRootCommand()
	c.SetOut(&out)
	c.SetArgs([]string{"--help"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--config path", "(default " + path + ")"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help should contain %q:\n%s", want, out.String())
		}
	}
}

func TestRootHelpWithoutAConfigDirectory(t *testing.T) {
	noConfigDir(t)
	if got, want := configUsage(), "(default trazo/config.yaml in the user config directory)"; !strings.HasSuffix(got, want) {
		t.Errorf("--config's help = %q, want it to end %q", got, want)
	}
}

// Review fix: noUserConfig keeps a developer's own config file out of the
// tests whatever the environment says, so it holds on every OS: Windows, for
// one, finds the config directory through %AppData%, not $HOME or
// $XDG_CONFIG_HOME.
func TestNoUserConfigHoldsWhateverTheEnvironmentSays(t *testing.T) {
	path := noUserConfig(t)
	writeFile(t, path, "shortcuts:\n  sidebar:\n    down: [j]\n")
	elsewhere := t.TempDir() // where the environment now puts the config directory
	for _, v := range []string{"HOME", "XDG_CONFIG_HOME", "AppData"} {
		t.Setenv(v, elsewhere)
	}
	s, err := execute(t)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.keys.Keys(sidebarDown); !slices.Equal(got, []string{"j"}) {
		t.Errorf("sidebar.down: keys %q, want j from %s", got, path)
	}
	if got := configUsage(); !strings.Contains(got, "(default "+path+")") {
		t.Errorf("--config's help = %q, want it to name %s", got, path)
	}
}
