package lockfile

import (
	"sort"
	"strings"

	"github.com/Go-Python-Toolchain/gopip/internal/resolve"
)

// Explain renders the resolved dependency tree as indented text, starting from
// the direct requirements. A package that reappears on the current path is
// marked as a cycle rather than expanded, so the output is always finite.
func Explain(sol *resolve.Solution) string {
	var b strings.Builder

	roots := append([]string(nil), sol.Roots...)
	sort.Strings(roots)

	for _, root := range roots {
		explainNode(&b, sol, root, 0, map[string]bool{})
	}
	return b.String()
}

func explainNode(b *strings.Builder, sol *resolve.Solution, name string, depth int, path map[string]bool) {
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString(name)
	if v, ok := sol.Packages[name]; ok {
		b.WriteString(" ")
		b.WriteString(v.String())
	}
	if path[name] {
		b.WriteString(" (cycle)\n")
		return
	}
	b.WriteString("\n")

	deps := append([]string(nil), sol.Edges[name]...)
	sort.Strings(deps)
	if len(deps) == 0 {
		return
	}

	path[name] = true
	for _, dep := range deps {
		explainNode(b, sol, dep, depth+1, path)
	}
	delete(path, name)
}
