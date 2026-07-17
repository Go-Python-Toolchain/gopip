package lockfile

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/resolve"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

func resolveSolution(t *testing.T, m *pypi.MemSource, roots ...string) *resolve.Solution {
	t.Helper()
	var reqs []*requirement.Requirement
	for _, s := range roots {
		req, err := requirement.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		reqs = append(reqs, req)
	}
	env := requirement.CurrentEnvironment("3.12")
	r := resolve.New(m, resolve.WithEnvironment(env), resolve.WithPythonVersion(version.MustParse("3.12")))
	sol, err := r.Resolve(context.Background(), reqs)
	if err != nil {
		t.Fatal(err)
	}
	return sol
}

func diamond(t *testing.T) *pypi.MemSource {
	m := pypi.NewMemSource()
	if err := m.AddPackage("a", "1.0.0", "c>=1.0"); err != nil {
		t.Fatal(err)
	}
	if err := m.AddPackage("b", "1.0.0", "c<2.0"); err != nil {
		t.Fatal(err)
	}
	m.AddPackage("c", "1.0.0")
	m.AddPackage("c", "1.5.0")
	m.AddPackage("c", "2.0.0")
	return m
}

func TestBuildAndStructure(t *testing.T) {
	m := pypi.NewMemSource()
	m.AddPackage("a", "1.0.0", "b>=1.0")
	m.AddPackage("b", "1.0.0")

	lock := Build(resolveSolution(t, m, "a"))
	if lock.Version != 1 {
		t.Fatalf("version = %d", lock.Version)
	}
	if len(lock.Roots) != 1 || lock.Roots[0] != "a" {
		t.Fatalf("roots = %v", lock.Roots)
	}
	if len(lock.Packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(lock.Packages))
	}
	// Packages are sorted by name.
	if lock.Packages[0].Name != "a" || lock.Packages[1].Name != "b" {
		t.Fatalf("packages not sorted: %v", lock.Packages)
	}
	if len(lock.Packages[0].Dependencies) != 1 || lock.Packages[0].Dependencies[0] != "b" {
		t.Fatalf("a dependencies = %v", lock.Packages[0].Dependencies)
	}
}

func TestMarshalDeterministic(t *testing.T) {
	// Two independent resolutions of the same graph produce byte-identical locks.
	// Nothing in the lock depends on the host, so this is also OS-independent.
	first, err := Build(resolveSolution(t, diamond(t), "a", "b")).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, err := Build(resolveSolution(t, diamond(t), "a", "b")).Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, again) {
			t.Fatalf("lock not deterministic on run %d:\n--- first ---\n%s\n--- again ---\n%s", i, first, again)
		}
	}
}

func TestMarshalNoHostContent(t *testing.T) {
	out, err := Build(resolveSolution(t, diamond(t), "a", "b")).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"linux", "darwin", "win32", "posix", "x86_64", "python_version"} {
		if bytes.Contains(bytes.ToLower(out), []byte(forbidden)) {
			t.Fatalf("lock contains host-specific content %q:\n%s", forbidden, out)
		}
	}
	// The resolved versions are present and pinned.
	if !bytes.Contains(out, []byte(`"version": "1.5.0"`)) {
		t.Fatalf("expected c to be pinned to 1.5.0:\n%s", out)
	}
}

func TestRoundTrip(t *testing.T) {
	out, err := Build(resolveSolution(t, diamond(t), "a", "b")).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	again, err := parsed.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, again) {
		t.Fatalf("round trip changed the lock:\n%s\nvs\n%s", out, again)
	}
}

func TestExplainTree(t *testing.T) {
	out := Explain(resolveSolution(t, diamond(t), "a", "b"))
	// Both a and b pull in c 1.5.0.
	if !strings.Contains(out, "a 1.0.0\n  c 1.5.0") {
		t.Fatalf("explain missing a subtree:\n%s", out)
	}
	if !strings.Contains(out, "b 1.0.0\n  c 1.5.0") {
		t.Fatalf("explain missing b subtree:\n%s", out)
	}
}

func TestExplainHandlesCycle(t *testing.T) {
	sol := &resolve.Solution{
		Packages: map[string]*version.Version{
			"a": version.MustParse("1.0.0"),
			"b": version.MustParse("1.0.0"),
		},
		Roots: []string{"a"},
		Edges: map[string][]string{"a": {"b"}, "b": {"a"}},
	}
	out := Explain(sol)
	if !strings.Contains(out, "(cycle)") {
		t.Fatalf("expected a cycle marker:\n%s", out)
	}
}
