package resolve

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// genGraph builds a random dependency graph that is satisfiable by construction:
// every package has a 1.0.0 release whose dependencies are all of the form
// >=1.0.0, so the assignment that puts every reachable package at 1.0.0 is
// always a valid solution. Higher releases carry a mix of satisfiable and
// impossible constraints, which forces the resolver to do real work and back
// off from versions that cannot be used.
func genGraph(rng *rand.Rand) (*pypi.MemSource, []*requirement.Requirement) {
	m := pypi.NewMemSource()
	n := 4 + rng.Intn(8) // 4 to 11 packages
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("p%d", i)
	}

	others := func(self int) []string {
		var out []string
		for j := 0; j < n; j++ {
			if j != self && rng.Intn(2) == 0 {
				out = append(out, names[j])
			}
		}
		return out
	}

	for i := 0; i < n; i++ {
		versions := 1 + rng.Intn(4) // 1 to 4 releases
		for v := 1; v <= versions; v++ {
			ver := fmt.Sprintf("%d.0.0", v)
			var deps []string
			if v == 1 {
				// The safe baseline: every dependency is satisfiable.
				for _, o := range others(i) {
					deps = append(deps, o+">=1.0.0")
				}
			} else if rng.Intn(2) == 0 {
				// An impossible constraint makes this release unusable.
				deps = append(deps, names[rng.Intn(n)]+">=99.0.0")
			} else {
				for _, o := range others(i) {
					deps = append(deps, o+">=1.0.0")
				}
			}
			if err := m.AddPackage(names[i], ver, deps...); err != nil {
				panic(err)
			}
		}
	}

	// Roots are a non-empty subset of packages.
	var roots []*requirement.Requirement
	for {
		for i := 0; i < n; i++ {
			if rng.Intn(3) == 0 {
				req, _ := requirement.Parse(names[i] + ">=1.0.0")
				roots = append(roots, req)
			}
		}
		if len(roots) > 0 {
			break
		}
	}
	return m, roots
}

func pinned(sol *Solution) string {
	parts := make([]string, 0, len(sol.Packages))
	for name, v := range sol.Packages {
		parts = append(parts, name+"=="+v.String())
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func TestResolveManyRandomGraphs(t *testing.T) {
	const graphs = 1000
	env := requirement.CurrentEnvironment("3.12")
	py := version.MustParse("3.12")
	ctx := context.Background()

	contradictions := 0
	nonDeterministic := 0
	withBacktracking := 0
	totalPackages := 0

	for i := 0; i < graphs; i++ {
		rng := rand.New(rand.NewSource(int64(i)))
		m, roots := genGraph(rng)

		sol, err := New(m, WithEnvironment(env), WithPythonVersion(py)).Resolve(ctx, roots)
		if err != nil {
			// The graph is satisfiable by construction, so a failure is a bug.
			contradictions++
			t.Errorf("graph %d: satisfiable graph failed to resolve: %v", i, err)
			continue
		}
		if err := Verify(ctx, m, env, roots, sol); err != nil {
			contradictions++
			t.Errorf("graph %d: solution does not satisfy the constraints: %v", i, err)
			continue
		}

		// Determinism: a second independent resolution must match.
		sol2, err := New(m, WithEnvironment(env), WithPythonVersion(py)).Resolve(ctx, roots)
		if err != nil || pinned(sol) != pinned(sol2) {
			nonDeterministic++
			t.Errorf("graph %d: resolution not deterministic", i)
		}

		totalPackages += len(sol.Packages)
		for _, v := range sol.Packages {
			if v.String() != "1.0.0" {
				withBacktracking++
				break
			}
		}
	}

	if contradictions != 0 {
		t.Fatalf("%d of %d graphs had contradictions", contradictions, graphs)
	}
	t.Logf("resolved %d random graphs, 0 contradictions, %d non-deterministic, "+
		"%d needed a version above the baseline, average %.1f packages per solution",
		graphs, nonDeterministic, withBacktracking, float64(totalPackages)/float64(graphs))
}

func TestUnsatisfiableIsDetected(t *testing.T) {
	// Two roots demand contradictory versions of a shared dependency.
	m := pypi.NewMemSource()
	m.AddPackage("a", "1.0.0", "shared>=2.0")
	m.AddPackage("b", "1.0.0", "shared<2.0")
	m.AddPackage("shared", "1.0.0")
	m.AddPackage("shared", "2.0.0")

	reqA, _ := requirement.Parse("a")
	reqB, _ := requirement.Parse("b")
	_, err := New(m, WithPythonVersion(version.MustParse("3.12"))).Resolve(
		context.Background(), []*requirement.Requirement{reqA, reqB})
	if err == nil {
		t.Fatal("expected the contradictory requirements to be reported as unsatisfiable")
	}
}
