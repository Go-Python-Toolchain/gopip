package cmd

import (
	"context"
	"fmt"

	"github.com/Go-Python-Toolchain/gopip/internal/lockfile"
	"github.com/spf13/cobra"
)

var explainOpts resolveOptions

var explainCmd = &cobra.Command{
	Use:   "explain [requirements...]",
	Short: "Resolve requirements and print the dependency tree",
	Long: `explain resolves the requirements and prints the resolved dependency tree,
so you can see why each package was chosen and how they relate.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := explainOpts
		opts.args = args
		sol, err := resolveInputs(context.Background(), opts)
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), lockfile.Explain(sol))
		return nil
	},
}

func init() {
	addResolveFlags(explainCmd.Flags(), &explainOpts)
	rootCmd.AddCommand(explainCmd)
}
