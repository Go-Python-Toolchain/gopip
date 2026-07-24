package resolve

import (
	"sort"
	"strings"

	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
)

// An extra is a package's optional feature: `flask[async]` means flask plus the
// dependencies flask publishes under its `async` extra. The resolver treats each
// extra as a package in its own right, named `flask[async]`, whose universe is
// flask's versions and which depends on flask at the same version plus whatever
// that extra requires.
//
// The obvious alternative, tracking a set of active extras per package and
// widening its dependencies as extras are requested, does not survive
// backtracking. Dependency incompatibilities are permanent facts, so once
// "flask 3.0 requires greenlet" is recorded because something wanted
// flask[async], it keeps forcing greenlet even if the package that asked for the
// extra is backjumped out of the solution. Modelling the extra as its own
// package keeps the fact conditional on the extra actually being selected, which
// is what makes it true. It also costs the solver nothing: `flask[async]` is an
// ordinary package to every part of the algorithm.
//
// Package names cannot contain a bracket, so the encoding is unambiguous.

// extraPkg names the resolver package standing for one extra of a package.
func extraPkg(base, extra string) string {
	return base + "[" + requirement.CanonicalizeName(extra) + "]"
}

// splitPkg separates a resolver package name into its base package and, if it
// stands for an extra, that extra's name.
func splitPkg(pkg string) (base, extra string) {
	open := strings.IndexByte(pkg, '[')
	if open < 0 || !strings.HasSuffix(pkg, "]") {
		return pkg, ""
	}
	return pkg[:open], pkg[open+1 : len(pkg)-1]
}

// isExtraPkg reports whether a resolver package stands for an extra.
func isExtraPkg(pkg string) bool {
	_, extra := splitPkg(pkg)
	return extra != ""
}

// extrasOf returns a requirement's extras, canonicalized, sorted, and without
// duplicates, so `flask[Async,async]` and `flask[async]` mean the same thing.
func extrasOf(dep *requirement.Requirement) []string {
	if len(dep.Extras) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(dep.Extras))
	for _, e := range dep.Extras {
		c := requirement.CanonicalizeName(e)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
