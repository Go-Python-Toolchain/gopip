package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var installOpts struct {
	reqFiles   []string
	python     string
	indexURL   string
	pythonExec string
	dryRun     bool
}

var installCmd = &cobra.Command{
	Use:   "install [requirements...] [-- pip args]",
	Short: "Resolve requirements and install them with pip",
	Long: `install resolves the requirements to exact versions and then hands the
installation to pip, so packages install exactly as pip would while gopip does
the resolving.

Anything after a bare -- is passed straight through to pip. The install uses the
pip of the interpreter given by --python-exec, which defaults to python3, so pip
honors its own settings such as PIP_INDEX_URL during the install step.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reqArgs := args
		var pipExtra []string
		if dash := cmd.ArgsLenAtDash(); dash >= 0 {
			reqArgs = args[:dash]
			pipExtra = args[dash:]
		}

		sol, err := resolveInputs(context.Background(), resolveOptions{
			args:     reqArgs,
			reqFiles: installOpts.reqFiles,
			python:   installOpts.python,
			indexURL: installOpts.indexURL,
		})
		if err != nil {
			return err
		}

		exe := installOpts.pythonExec
		if exe == "" {
			exe = "python3"
		}
		pipArgs := pipInstallArgs(pinnedRequirements(sol), pipExtra)

		out := cmd.OutOrStdout()
		if installOpts.dryRun {
			fmt.Fprintf(out, "%s %s\n", exe, strings.Join(pipArgs, " "))
			return nil
		}

		fmt.Fprintf(out, "installing %d package(s) with %s\n", len(sol.Packages), exe)
		c := exec.Command(exe, pipArgs...)
		c.Stdout = out
		c.Stderr = cmd.ErrOrStderr()
		c.Stdin = os.Stdin
		return c.Run()
	},
}

// pipInstallArgs builds the argument list for python -m pip install.
func pipInstallArgs(pinned, extra []string) []string {
	args := []string{"-m", "pip", "install"}
	args = append(args, pinned...)
	args = append(args, extra...)
	return args
}

func init() {
	f := installCmd.Flags()
	f.StringArrayVarP(&installOpts.reqFiles, "requirement", "r", nil, "requirements file to read (repeatable)")
	f.StringVar(&installOpts.python, "python", "", "target Python version, for example 3.12")
	f.StringVar(&installOpts.indexURL, "index-url", "", "JSON index base URL (default the public index or GOPIP_INDEX_URL)")
	f.StringVar(&installOpts.pythonExec, "python-exec", "python3", "interpreter whose pip performs the install")
	f.BoolVar(&installOpts.dryRun, "dry-run", false, "print the pip command instead of running it")
	rootCmd.AddCommand(installCmd)
}
