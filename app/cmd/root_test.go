package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/candy-tools/edit/libs/keymap"
)

// stubTUI swaps the TUI launcher for fn for the duration of the test.
func stubTUI(t *testing.T, fn func(root string, keys *keymap.Map) error) {
	t.Helper()
	orig := runTUI
	runTUI = fn
	t.Cleanup(func() { runTUI = orig })
}

// noUserConfig points edit's default config file at a path in an empty
// temporary directory, so a developer's own config file is never read or
// written, on any OS, and returns that path.
func noUserConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "edit", "config.yaml")
	stubConfigPath(t, func() (string, error) { return path, nil })
	return path
}

// noConfigDir makes edit find no config directory, as when $HOME is unset.
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

// execute runs edit with args, the TUI stubbed, and returns the keymap the
// TUI started with, nil if it didn't start, and the command's error.
func execute(t *testing.T, args ...string) (*keymap.Map, error) {
	t.Helper()
	var keys *keymap.Map
	stubTUI(t, func(_ string, k *keymap.Map) error {
		keys = k
		return nil
	})
	c := newRootCommand()
	c.SetArgs(args)
	err := c.Execute()
	return keys, err
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
	stubTUI(t, func(root string, _ *keymap.Map) error {
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
	stubTUI(t, func(string, *keymap.Map) error { return boom })
	c := newRootCommand()
	c.SetArgs([]string{})
	if err := c.Execute(); !errors.Is(err, boom) {
		t.Errorf("Execute() = %v, want the TUI's error", err)
	}
}

func TestRootWithoutAConfigFileUsesTheDefaultKeys(t *testing.T) {
	noUserConfig(t)
	keys, err := execute(t)
	if err != nil {
		t.Fatal(err)
	}
	if got := keys.Keys(sidebarDown); !slices.Equal(got, []string{"down"}) {
		t.Errorf("sidebar.down: keys %q, want the default down", got)
	}
}

func TestRootWithoutAConfigDirectoryUsesTheDefaultKeys(t *testing.T) {
	noConfigDir(t)
	keys, err := execute(t)
	if err != nil {
		t.Fatal(err)
	}
	if got := keys.Keys(sidebarDown); !slices.Equal(got, []string{"down"}) {
		t.Errorf("sidebar.down: keys %q, want the default down", got)
	}
}

func TestRootReadsTheConfigFile(t *testing.T) {
	writeFile(t, noUserConfig(t), "shortcuts:\n  sidebar:\n    down: [j, down]\n")
	keys, err := execute(t)
	if err != nil {
		t.Fatal(err)
	}
	if got := keys.Keys(sidebarDown); !slices.Equal(got, []string{"j", "down"}) {
		t.Errorf("sidebar.down: keys %q, want j, down from the config file", got)
	}
}

func TestRootReadsTheFileTheConfigFlagNames(t *testing.T) {
	writeFile(t, noUserConfig(t), "shortcuts:\n  sidebar:\n    down: [k]\n")
	path := filepath.Join(t.TempDir(), "edit.yaml")
	writeFile(t, path, "shortcuts:\n  sidebar:\n    down: [j]\n")
	keys, err := execute(t, "--config", path)
	if err != nil {
		t.Fatal(err)
	}
	if got := keys.Keys(sidebarDown); !slices.Equal(got, []string{"j"}) {
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
	} {
		path := noUserConfig(t)
		var args []string
		if c.flag {
			path = filepath.Join(t.TempDir(), "edit.yaml")
			args = []string{"--config", path}
		}
		if c.text != "" {
			writeFile(t, path, c.text)
		}
		keys, err := execute(t, args...)
		if want := fmt.Sprintf(c.want, path); err == nil || err.Error() != want {
			t.Errorf("%s: Execute() = %v, want\n%s", c.name, err, want)
		}
		if keys != nil {
			t.Errorf("%s: the TUI started", c.name)
		}
	}
}

// Review focus: a config file at the default location that edit can't read
// stops it with the cause, rather than leaving it on the default keys.
func TestRootStopsOnAnUnreadableConfigFile(t *testing.T) {
	path := noUserConfig(t)
	if err := os.MkdirAll(path, 0o700); err != nil { // a folder where the file should be
		t.Fatal(err)
	}
	keys, err := execute(t)
	if want := "config " + path + ": is a directory"; err == nil || err.Error() != want {
		t.Errorf("Execute() = %v, want %q", err, want)
	}
	if keys != nil {
		t.Error("the TUI started")
	}
}

func TestRootStopsOnAConfigSyntaxError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "edit.yaml")
	writeFile(t, path, "shortcuts:\n  sidebar: [down\n")
	keys, err := execute(t, "--config", path)
	if want := "config " + path + ": yaml: line "; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("Execute() = %v, want an error starting %q", err, want)
	}
	if keys != nil {
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
	if got, want := configUsage(), "(default edit/config.yaml in the user config directory)"; !strings.HasSuffix(got, want) {
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
	keys, err := execute(t)
	if err != nil {
		t.Fatal(err)
	}
	if got := keys.Keys(sidebarDown); !slices.Equal(got, []string{"j"}) {
		t.Errorf("sidebar.down: keys %q, want j from %s", got, path)
	}
	if got := configUsage(); !strings.Contains(got, "(default "+path+")") {
		t.Errorf("--config's help = %q, want it to name %s", got, path)
	}
}
