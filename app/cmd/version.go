package cmd

import (
	"fmt"
	"io"
	"runtime"

	"github.com/candy-tools/edit/app/metainfo"
	"github.com/spf13/cobra"
)

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			printVersion(cmd.OutOrStdout())
		},
	}
}

// printVersion writes the version and build metadata to w.
func printVersion(w io.Writer) {
	_, _ = fmt.Fprintf(w, "edit %s\n", metainfo.Version)
	_, _ = fmt.Fprintf(w, "  commit:   %s\n", metainfo.Commit)
	_, _ = fmt.Fprintf(w, "  built:    %s\n", metainfo.BuildTime)
	_, _ = fmt.Fprintf(w, "  compiler: %s\n", runtime.Version())
}
