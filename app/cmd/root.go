// Package cmd is the command-line entry point: it parses `edit` and launches
// the interactive TUI on the current directory.
package cmd

import (
	"fmt"
	"os"

	"github.com/candy-tools/edit/app/metainfo"
	"github.com/candy-tools/edit/app/tui"
	"github.com/spf13/cobra"
)

// runTUI starts the interactive editor on a workspace directory. It is a
// package variable so tests can stub it and exercise the command wiring
// without a terminal.
var runTUI = tui.Run

// Execute is the entry point for the command line.
func Execute() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "edit",
		Short:         "A terminal text editor",
		Long:          "edit is a keyboard-driven text editor for the terminal: a file sidebar\non the left and the open files as tabs on the right.\n\nIt opens the current directory as the workspace.",
		Version:       metainfo.Version,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(*cobra.Command, []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			return runTUI(root)
		},
	}
	cmd.AddCommand(versionCmd())
	return cmd
}
