// Package cmd is the command-line entry point: it parses `trazo`, reads the
// config file and launches the interactive TUI on the current directory.
package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/candy-tools/trazo/app/config"
	"github.com/candy-tools/trazo/app/metainfo"
	"github.com/candy-tools/trazo/app/tui"
	"github.com/candy-tools/trazo/libs/keymap"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

// runTUI starts the interactive editor on a workspace directory. It is a
// package variable so tests can stub it and exercise the command wiring
// without a terminal.
var runTUI = tui.Run

// defaultConfigPath is where trazo looks for its config file without
// --config. It is a package variable so tests can point it at a temporary
// file, whatever the OS's config directory.
var defaultConfigPath = config.DefaultPath

// terminalProfile says which colors the terminal trazo draws on shows, as
// colorsOf reads them. It is a package variable so tests can fake a
// terminal.
var terminalProfile = func() colorprofile.Profile {
	return colorsOf(os.Stdout, os.Environ(), term.IsTerminal(os.Stdout.Fd()))
}

// colorsOf is the color profile of the terminal on out, whose environment is
// env, as trazo reads it: as colorprofile detects it, but without colors
// when NO_COLOR is set to anything, or CLICOLOR is 0 and CLICOLOR_FORCE
// isn't set, as termenv read them for Bubble Tea v1; and when tty says out
// is a terminal, with text attributes but no colors rather than nothing at
// all on one whose TERM is dumb or unset, so the cursor's reverse video
// still shows.
func colorsOf(out io.Writer, env []string, tty bool) colorprofile.Profile {
	p := colorprofile.Detect(out, env)
	vars := map[string]string{}
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			vars[k] = v
		}
	}
	forced := vars["CLICOLOR_FORCE"] != "" && vars["CLICOLOR_FORCE"] != "0"
	if p > colorprofile.ASCII && (vars["NO_COLOR"] != "" || vars["CLICOLOR"] == "0" && !forced) {
		p = colorprofile.ASCII
	}
	if p == colorprofile.NoTTY && tty {
		p = colorprofile.ASCII
	}
	return p
}

// Execute is the entry point for the command line.
func Execute() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:           "trazo",
		Short:         "A terminal text editor",
		Long:          "trazo is a keyboard-driven text editor for the terminal: a file sidebar\non the left and the open files as tabs on the right.\n\nIt opens the current directory as the workspace.",
		Version:       metainfo.Version,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(*cobra.Command, []string) error {
			colors := terminalProfile()
			keys, theme, mouse, err := loadSettings(configPath, colors)
			if err != nil {
				return err
			}
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			return runTUI(root, keys, theme, colors, mouse)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", configUsage())
	cmd.AddCommand(versionCmd())
	return cmd
}

// configUsage is the help of the --config flag, which names the file trazo
// reads without it.
func configUsage() string {
	where := "trazo/config.yaml in the user config directory"
	if path, err := defaultConfigPath(); err == nil {
		where = path
	}
	return "read the settings from the config file at `path` (default " + where + ")"
}

// loadSettings builds the keymap and the theme from the config file at path,
// or at the default location when path is "", and says whether it turns the
// mouse on. There, a missing file, or no config directory at all, means the
// defaults; a missing file named with --config is an error, so a typo in its
// path can't go unnoticed. Its errors start "config <path>: ", and an
// invalid keymap's list its problems, one per line, indented. A theme that
// doesn't exist is an error on any terminal, but on one without colors —
// colors, the terminal's profile, is ASCII or less, as NO_COLOR makes any
// terminal — the theme is none.
func loadSettings(path string, colors colorprofile.Profile) (keys *keymap.Map, theme tui.Theme, mouse bool, err error) {
	cfg, path, err := loadConfig(path)
	if err != nil {
		return nil, tui.Theme{}, false, err
	}
	keys, err = tui.NewKeymap(cfg.Shortcuts)
	if err != nil {
		problems := strings.ReplaceAll(err.Error(), "\n", "\n  ")
		return nil, tui.Theme{}, false, fmt.Errorf("config %s: invalid shortcuts:\n  %s", path, problems)
	}
	theme, err = tui.NewTheme(cfg.Theme)
	if err != nil {
		return nil, tui.Theme{}, false, fmt.Errorf("config %s: %w", path, err)
	}
	if colors <= colorprofile.ASCII {
		theme, err = tui.NewTheme("none")
	}
	return keys, theme, cfg.Mouse, err
}

// loadConfig reads the config file at path, or at the default location when
// path is "", and returns it and its path, as loadSettings describes.
func loadConfig(path string) (config.Config, string, error) {
	named := path != ""
	if !named {
		var err error
		if path, err = defaultConfigPath(); err != nil {
			return config.Config{}, "", nil
		}
	}
	cfg, err := config.Load(path)
	if err != nil && (named || !errors.Is(err, fs.ErrNotExist)) {
		return config.Config{}, path, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, path, nil
}
