// Package cmd is the command-line entry point: it parses `edit`, reads the
// config file and launches the interactive TUI on the current directory.
package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/candy-tools/edit/app/config"
	"github.com/candy-tools/edit/app/metainfo"
	"github.com/candy-tools/edit/app/tui"
	"github.com/candy-tools/edit/libs/keymap"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
)

// runTUI starts the interactive editor on a workspace directory. It is a
// package variable so tests can stub it and exercise the command wiring
// without a terminal.
var runTUI = tui.Run

// defaultConfigPath is where edit looks for its config file without
// --config. It is a package variable so tests can point it at a temporary
// file, whatever the OS's config directory.
var defaultConfigPath = config.DefaultPath

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
		Use:           "edit",
		Short:         "A terminal text editor",
		Long:          "edit is a keyboard-driven text editor for the terminal: a file sidebar\non the left and the open files as tabs on the right.\n\nIt opens the current directory as the workspace.",
		Version:       metainfo.Version,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(*cobra.Command, []string) error {
			keys, theme, err := loadSettings(configPath)
			if err != nil {
				return err
			}
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			return runTUI(root, keys, theme)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", configUsage())
	cmd.AddCommand(versionCmd())
	return cmd
}

// configUsage is the help of the --config flag, which names the file edit
// reads without it.
func configUsage() string {
	where := "edit/config.yaml in the user config directory"
	if path, err := defaultConfigPath(); err == nil {
		where = path
	}
	return "read the settings from the config file at `path` (default " + where + ")"
}

// loadSettings builds the keymap and the theme from the config file at path,
// or at the default location when path is "". There, a missing file, or no
// config directory at all, means the defaults; a missing file named with
// --config is an error, so a typo in its path can't go unnoticed. Its errors
// start "config <path>: ", and an invalid keymap's list its problems, one
// per line, indented. A theme that doesn't exist is an error on any
// terminal, but on one without colors, as NO_COLOR makes any terminal, the
// theme is none.
func loadSettings(path string) (*keymap.Map, tui.Theme, error) {
	cfg, path, err := loadConfig(path)
	if err != nil {
		return nil, tui.Theme{}, err
	}
	keys, err := tui.NewKeymap(cfg.Shortcuts)
	if err != nil {
		problems := strings.ReplaceAll(err.Error(), "\n", "\n  ")
		return nil, tui.Theme{}, fmt.Errorf("config %s: invalid shortcuts:\n  %s", path, problems)
	}
	theme, err := tui.NewTheme(cfg.Theme)
	if err != nil {
		return nil, tui.Theme{}, fmt.Errorf("config %s: %w", path, err)
	}
	if lipgloss.ColorProfile() == termenv.Ascii {
		theme, err = tui.NewTheme("none")
	}
	return keys, theme, err
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
