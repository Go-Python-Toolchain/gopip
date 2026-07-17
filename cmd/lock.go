package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/Go-Python-Toolchain/gopip/internal/lockfile"
	"github.com/spf13/cobra"
)

var lockOpts struct {
	reqFiles []string
	python   string
	indexURL string
	output   string
}

var lockCmd = &cobra.Command{
	Use:   "lock [requirements...]",
	Short: "Resolve requirements and write a deterministic gpt.lock",
	Long: `lock resolves the requirements and writes gpt.lock, a deterministic JSON
lockfile that pins every package and records the dependency graph. The same
requirements produce a byte-identical lockfile on any machine.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sol, err := resolveInputs(context.Background(), resolveOptions{
			args:     args,
			reqFiles: lockOpts.reqFiles,
			python:   lockOpts.python,
			indexURL: lockOpts.indexURL,
		})
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
	f.StringArrayVarP(&lockOpts.reqFiles, "requirement", "r", nil, "requirements file to read (repeatable)")
	f.StringVar(&lockOpts.python, "python", "", "target Python version, for example 3.12")
	f.StringVar(&lockOpts.indexURL, "index-url", "", "JSON index base URL (default the public index or GOPIP_INDEX_URL)")
	f.StringVarP(&lockOpts.output, "output", "o", "gpt.lock", "path to write the lockfile")
	rootCmd.AddCommand(lockCmd)
}
