package resolve

import (
	"context"
	"testing"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

func solveForPython(t *testing.T, m *pypi.MemSource, py string, roots ...string) (map[string]string, error) {
	t.Helper()
	var reqs []*requirement.Requirement
	for _, s := range roots {
		req, err := requirement.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		reqs = append(reqs, req)
	}
	env := requirement.CurrentEnvironment(py)
	r := New(m, WithEnvironment(env), WithPythonVersion(version.MustParse(py)))
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

func TestMarkerFiltersDependency(t *testing.T) {
	m := pypi.NewMemSource()
	// a depends on legacy only on Python before 3.10.
	m.AddPackage("a", "1.0.0", `legacy>=1.0; python_version < "3.10"`)
	m.AddPackage("legacy", "1.0.0")

	// On Python 3.12 the dependency does not apply.
	got, err := solveForPython(t, m, "3.12", "a")
	if err != nil {
		t.Fatal(err)
	}
	assertSolution(t, got, map[string]string{"a": "1.0.0"})

	// On Python 3.9 it does apply and legacy is pulled in.
	got, err = solveForPython(t, m, "3.9", "a")
	if err != nil {
		t.Fatal(err)
	}
	assertSolution(t, got, map[string]string{"a": "1.0.0", "legacy": "1.0.0"})
}

func TestRequiresPythonSkipsVersion(t *testing.T) {
	m := pypi.NewMemSource()
	// a 2.0 requires a newer Python than the target, so 1.0 is chosen instead.
	m.Add(&pypi.ReleaseInfo{Name: "a", Version: version.MustParse("2.0.0"), RequiresPython: ">=3.13"})
	m.Add(&pypi.ReleaseInfo{Name: "a", Version: version.MustParse("1.0.0"), RequiresPython: ">=3.8"})

	got, err := solveForPython(t, m, "3.12", "a")
	if err != nil {
		t.Fatal(err)
	}
	assertSolution(t, got, map[string]string{"a": "1.0.0"})
}

func TestDeeperBacktracking(t *testing.T) {
	m := pypi.NewMemSource()
	// foo's newest versions each demand a bar that forces a shared baz conflict,
	// so the resolver must walk foo down to a version that fits.
	m.AddPackage("foo", "1.3.0", "bar>=2.0")
	m.AddPackage("foo", "1.2.0", "bar>=2.0")
	m.AddPackage("foo", "1.1.0", "bar>=1.0")
	m.AddPackage("bar", "2.0.0", "baz>=2.0")
	m.AddPackage("bar", "1.0.0", "baz<2.0")
	m.AddPackage("baz", "1.0.0")
	m.AddPackage("baz", "1.5.0")

	got, err := solve(t, m, "foo", "baz<2.0")
	if err != nil {
		t.Fatal(err)
	}
	// baz is pinned below 2.0, so bar 2.0 (needs baz>=2.0) is impossible, which
	// rules out foo 1.3 and 1.2. foo 1.1 with bar 1.0 and baz 1.5 works.
	assertSolution(t, got, map[string]string{"foo": "1.1.0", "bar": "1.0.0", "baz": "1.5.0"})
}
