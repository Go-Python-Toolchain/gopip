package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/Go-Python-Toolchain/gopip/internal/lockfile"
	"github.com/spf13/cobra"
)

var lockOpts struct {
	resolveOptions
	output string
}

var lockCmd = &cobra.Command{
	Use:   "lock [requirements...]",
	Short: "Resolve requirements and write a deterministic gpt.lock",
	Long: `lock resolves the requirements and writes gpt.lock, a deterministic JSON
lockfile that pins every package and records the dependency graph. The same
requirements produce a byte-identical lockfile on any machine.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := lockOpts.resolveOptions
		opts.args = args
		sol, err := resolveInputs(context.Background(), opts)
		if err != nil {
			return err
		}
		data, err := lockfile.Build(sol).Marshal()
		if err != nil {
			return err
		}
		if err := os.WriteFile(lockOpts.output, data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "wrote %d package(s) to %s\n", len(sol.Packages), lockOpts.output)
		return nil
	},
}

func init() {
	f := lockCmd.Flags()
	addResolveFlags(f, &lockOpts.resolveOptions)
	f.StringVarP(&lockOpts.output, "output", "o", "gpt.lock", "path to write the lockfile")
	rootCmd.AddCommand(lockCmd)
}
