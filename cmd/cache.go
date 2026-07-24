package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Inspect and clear the index metadata cache",
	Long: `cache shows where gopip stores the package metadata it has fetched, how much
of it there is, and clears it.

gopip caches what it reads from a package index so a second resolve does little
or no network work. Entries are kept per index, so a private index and the
public one never answer for each other. Nothing in the cache affects which
versions are chosen: it only saves fetching the same metadata again.`,
}

var cacheDirCmd = &cobra.Command{
	Use:   "dir",
	Short: "Print the cache directory",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := pypi.CacheRoot()
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), root)
		return nil
	},
}

var cacheInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show what the cache is holding",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := pypi.CacheRoot()
		if err != nil {
			return err
		}
		caches, err := pypi.ListCaches(root)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "cache root: %s\n", root)
		if len(caches) == 0 {
			fmt.Fprintln(out, "the cache is empty")
			return nil
		}

		var packages, entries int
		var bytes int64
		for _, c := range caches {
			fmt.Fprintf(out, "\n%s\n  %d package(s), %d entrie(s), %s\n",
				filepath.Base(c.Dir), c.Packages, c.Entries, humanBytes(c.Bytes))
			packages += c.Packages
			entries += c.Entries
			bytes += c.Bytes
		}
		fmt.Fprintf(out, "\ntotal: %d package(s), %d entrie(s), %s\n", packages, entries, humanBytes(bytes))
		return nil
	},
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Delete everything the cache is holding",
	Long: `clear deletes the cached index metadata. Nothing is lost that cannot be
fetched again, so the only cost of clearing is that the next resolve is a cold
one.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := pypi.CacheRoot()
		if err != nil {
			return err
		}
		caches, err := pypi.ListCaches(root)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		if len(caches) == 0 {
			fmt.Fprintln(out, "the cache is already empty")
			return nil
		}

		var entries int
		var bytes int64
		for _, c := range caches {
			entries += c.Entries
			bytes += c.Bytes
		}
		if err := os.RemoveAll(root); err != nil {
			return err
		}
		fmt.Fprintf(out, "removed %d entrie(s), %s, from %s\n", entries, humanBytes(bytes), root)
		return nil
	},
}

// humanBytes renders a size the way a person reads it.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit && exp < 3 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

func init() {
	cacheCmd.AddCommand(cacheDirCmd, cacheInfoCmd, cacheClearCmd)
	rootCmd.AddCommand(cacheCmd)
}
