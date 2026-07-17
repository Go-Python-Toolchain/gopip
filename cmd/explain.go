package cmd

import (
	"context"
	"fmt"

	"github.com/Go-Python-Toolchain/gopip/internal/lockfile"
	"github.com/spf13/cobra"
)

var explainOpts struct {
	reqFiles []string
	python   string
	indexURL string
}

var explainCmd = &cobra.Command{
	Use:   "explain [requirements...]",
	Short: "Resolve requirements and print the dependency tree",
	Long: `explain resolves the requirements and prints the resolved dependency tree,
so you can see why each package was chosen and how they relate.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sol, err := resolveInputs(context.Background(), resolveOptions{
			args:     args,
			reqFiles: explainOpts.reqFiles,
			python:   explainOpts.python,
			indexURL: explainOpts.indexURL,
		})
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), lockfile.Explain(sol))
		return nil
	},
}

func init() {
	f := explainCmd.Flags()
	f.StringArrayVarP(&explainOpts.reqFiles, "requirement", "r", nil, "requirements file to read (repeatable)")
	f.StringVar(&explainOpts.python, "python", "", "target Python version, for example 3.12")
	f.StringVar(&explainOpts.indexURL, "index-url", "", "JSON index base URL (default the public index or GOPIP_INDEX_URL)")
	rootCmd.AddCommand(explainCmd)
}
