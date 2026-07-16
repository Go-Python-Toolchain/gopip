package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gopip",
	Short: "A fast, deterministic dependency resolver for Python",
	Long: `gopip resolves Python dependencies with a pure Go solver and writes a
deterministic lockfile. It does not replace pip. It computes what to install,
quickly and reproducibly, and leaves the installation to pip.

gopip reads existing standards, so it does not ask you to change your project.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command and exits non-zero on failure.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "gopip:", err)
		os.Exit(1)
	}
}
