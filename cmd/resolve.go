package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var resolveOpts struct {
	reqFiles []string
	python   string
	indexURL string
}

var resolveCmd = &cobra.Command{
	Use:   "resolve [requirements...]",
	Short: "Resolve requirements and print the pinned versions",
	Long: `resolve computes a consistent set of package versions for the given
requirements and prints them as name==version lines, the form pip understands.

Requirements may be given as arguments or read from files with -r.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sol, err := resolveInputs(context.Background(), resolveOptions{
			args:     args,
			reqFiles: resolveOpts.reqFiles,
			python:   resolveOpts.python,
			indexURL: resolveOpts.indexURL,
		})
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
	f := resolveCmd.Flags()
	f.StringArrayVarP(&resolveOpts.reqFiles, "requirement", "r", nil, "requirements file to read (repeatable)")
	f.StringVar(&resolveOpts.python, "python", "", "target Python version, for example 3.12")
	f.StringVar(&resolveOpts.indexURL, "index-url", "", "JSON index base URL (default the public index or GOPIP_INDEX_URL)")
	rootCmd.AddCommand(resolveCmd)
}
