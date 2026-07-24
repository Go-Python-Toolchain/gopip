package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Go-Python-Toolchain/gopip/internal/resolve"
	"github.com/spf13/cobra"
)

var installOpts struct {
	resolveOptions
	pythonExec    string
	dryRun        bool
	requireHashes bool
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

		opts := installOpts.resolveOptions
		opts.args = reqArgs
		sol, err := resolveInputs(context.Background(), opts)
		if err != nil {
			return err
		}

		exe := installOpts.pythonExec
		if exe == "" {
			exe = "python3"
		}
		out := cmd.OutOrStdout()

		if installOpts.requireHashes {
			return installWithHashes(cmd, exe, sol, pipExtra)
		}

		pipArgs := pipInstallArgs(pinnedRequirements(sol), pipExtra)
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

// installWithHashes installs from a generated requirements file carrying every
// pinned package and its artifact digests, which is the only way to give pip
// hashes. The file is temporary: it is derived from the resolution and would go
// stale the moment anything changed, so it is not something to leave in a
// project.
func installWithHashes(cmd *cobra.Command, exe string, sol *resolve.Solution, pipExtra []string) error {
	body, err := hashedRequirements(sol)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()

	if installOpts.dryRun {
		fmt.Fprint(out, body)
		fmt.Fprintf(out, "%s -m pip install --require-hashes -r <the requirements above>", exe)
		if len(pipExtra) > 0 {
			fmt.Fprintf(out, " %s", strings.Join(pipExtra, " "))
		}
		fmt.Fprintln(out)
		return nil
	}

	f, err := os.CreateTemp("", "gopip-requirements-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	args := append([]string{"-m", "pip", "install", "--require-hashes", "-r", f.Name()}, pipExtra...)
	fmt.Fprintf(out, "installing %d package(s) with %s, verifying hashes\n", len(sol.Packages), exe)
	c := exec.Command(exe, args...)
	c.Stdout = out
	c.Stderr = cmd.ErrOrStderr()
	c.Stdin = os.Stdin
	return c.Run()
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
	addResolveFlags(f, &installOpts.resolveOptions)
	f.StringVar(&installOpts.pythonExec, "python-exec", "python3", "interpreter whose pip performs the install")
	f.BoolVar(&installOpts.dryRun, "dry-run", false, "print the pip command instead of running it")
	f.BoolVar(&installOpts.requireHashes, "require-hashes", false, "verify each downloaded artifact against the digests the index published")
	rootCmd.AddCommand(installCmd)
}
