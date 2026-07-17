package resolve

import (
	"context"
	"testing"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

func solve(t *testing.T, m *pypi.MemSource, roots ...string) (map[string]string, error) {
	t.Helper()
	var reqs []*requirement.Requirement
	for _, s := range roots {
		req, err := requirement.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		reqs = append(reqs, req)
	}
	r := New(m, WithPythonVersion(version.MustParse("3.12")))
	sol, err := r.Resolve(context.Background(), reqs)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for pkg, v := range sol.Packages {
		out[pkg] = v.String()
	}
	return out, nil
}

func assertSolution(t *testing.T, got map[string]string, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("solution has %d packages %v, want %d %v", len(got), got, len(want), want)
	}
	for pkg, ver := range want {
		if got[pkg] != ver {
			t.Errorf("package %s = %q, want %q (full: %v)", pkg, got[pkg], ver, got)
		}
	}
}

func TestResolveSingle(t *testing.T) {
	m := pypi.NewMemSource()
	m.AddPackage("a", "1.0.0")
	got, err := solve(t, m, "a")
	if err != nil {
		t.Fatal(err)
	}
	assertSolution(t, got, map[string]string{"a": "1.0.0"})
}

func TestResolveTransitive(t *testing.T) {
	m := pypi.NewMemSource()
	m.AddPackage("a", "1.0.0", "b>=1.0")
	m.AddPackage("b", "1.0.0")
	got, err := solve(t, m, "a")
	if err != nil {
		t.Fatal(err)
	}
	assertSolution(t, got, map[string]string{"a": "1.0.0", "b": "1.0.0"})
}

func TestResolveAnyVersionDependency(t *testing.T) {
	m := pypi.NewMemSource()
	m.AddPackage("a", "1.0.0", "b")
	m.AddPackage("b", "1.0.0")
	m.AddPackage("b", "2.0.0")
	got, err := solve(t, m, "a")
	if err != nil {
		t.Fatal(err)
	}
	// b has no constraint, so the highest version is chosen.
	assertSolution(t, got, map[string]string{"a": "1.0.0", "b": "2.0.0"})
}

func TestResolveHighestVersion(t *testing.T) {
	m := pypi.NewMemSource()
	m.AddPackage("a", "1.0.0")
	m.AddPackage("a", "1.5.0")
	m.AddPackage("a", "2.0.0")
	got, err := solve(t, m, "a>=1.0")
	if err != nil {
		t.Fatal(err)
	}
	assertSolution(t, got, map[string]string{"a": "2.0.0"})
}

func TestResolveDiamond(t *testing.T) {
	m := pypi.NewMemSource()
	m.AddPackage("a", "1.0.0", "c>=1.0")
	m.AddPackage("b", "1.0.0", "c<2.0")
	m.AddPackage("c", "1.0.0")
	m.AddPackage("c", "1.5.0")
	m.AddPackage("c", "2.0.0")
	got, err := solve(t, m, "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	// c must satisfy >=1.0 and <2.0; the highest such version is 1.5.0.
	assertSolution(t, got, map[string]string{"a": "1.0.0", "b": "1.0.0", "c": "1.5.0"})
}

func TestResolveBacktracking(t *testing.T) {
	m := pypi.NewMemSource()
	// a 2.0 needs b>=2.0 which does not exist, so the resolver must back off to
	// a 1.0 which accepts any b.
	m.AddPackage("a", "2.0.0", "b>=2.0")
	m.AddPackage("a", "1.0.0", "b")
	m.AddPackage("b", "1.0.0")
	got, err := solve(t, m, "a")
	if err != nil {
		t.Fatal(err)
	}
	assertSolution(t, got, map[string]string{"a": "1.0.0", "b": "1.0.0"})
}

func TestResolveConflict(t *testing.T) {
	m := pypi.NewMemSource()
	m.AddPackage("a", "1.0.0", "c>=2.0")
	m.AddPackage("b", "1.0.0", "c<2.0")
	m.AddPackage("c", "1.0.0")
	m.AddPackage("c", "2.0.0")
	if got, err := solve(t, m, "a", "b"); err == nil {
		t.Fatalf("expected a resolution error, got solution %v", got)
	}
}

func TestResolveMissingPackage(t *testing.T) {
	m := pypi.NewMemSource()
	m.AddPackage("a", "1.0.0", "ghost>=1.0")
	if got, err := solve(t, m, "a"); err == nil {
		t.Fatalf("expected a resolution error for a missing dependency, got %v", got)
	}
}

func TestResolveDeterministic(t *testing.T) {
	build := func() *pypi.MemSource {
		m := pypi.NewMemSource()
		m.AddPackage("a", "1.0.0", "c>=1.0")
		m.AddPackage("b", "1.0.0", "c<2.0")
		m.AddPackage("c", "1.0.0")
		m.AddPackage("c", "1.5.0")
		m.AddPackage("c", "2.0.0")
		return m
	}
	first, err := solve(t, build(), "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, err := solve(t, build(), "a", "b")
		if err != nil {
			t.Fatal(err)
		}
		assertSolution(t, again, first)
	}
}
