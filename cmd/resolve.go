package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var resolveOpts resolveOptions

var resolveCmd = &cobra.Command{
	Use:   "resolve [requirements...]",
	Short: "Resolve requirements and print the pinned versions",
	Long: `resolve computes a consistent set of package versions for the given
requirements and prints them as name==version lines, the form pip understands.

Requirements may be given as arguments or read from files with -r.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := resolveOpts
		opts.args = args
		sol, err := resolveInputs(context.Background(), opts)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		for _, line := range pinnedRequirements(sol) {
			fmt.Fprintln(out, line)
		}
		return nil
	},
}

func init() {
	addResolveFlags(resolveCmd.Flags(), &resolveOpts)
	rootCmd.AddCommand(resolveCmd)
}
