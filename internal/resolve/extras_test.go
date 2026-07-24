package resolve_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/resolve"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// resolveSpecs resolves a set of requirement strings against a source.
func resolveSpecs(t *testing.T, source pypi.Source, specs ...string) *resolve.Solution {
	t.Helper()
	var roots []*requirement.Requirement
	for _, s := range specs {
		req, err := requirement.Parse(s)
		if err != nil {
			t.Fatalf("parsing %q: %v", s, err)
		}
		roots = append(roots, req)
	}
	r := resolve.New(source,
		resolve.WithEnvironment(fixtureEnvironment()),
		resolve.WithPythonVersion(version.MustParse(fixturePython)))
	sol, err := r.Resolve(context.Background(), roots)
	if err != nil {
		t.Fatalf("resolving %v: %v", specs, err)
	}
	if err := resolve.Verify(context.Background(), source, fixtureEnvironment(), roots, sol); err != nil {
		t.Fatalf("solution for %v is invalid: %v", specs, err)
	}
	return sol
}

// names lists a solution's packages as sorted name==version strings.
func names(sol *resolve.Solution) []string {
	out := make([]string, 0, len(sol.Packages))
	for n, v := range sol.Packages {
		out = append(out, n+"=="+v.String())
	}
	sort.Strings(out)
	return out
}

func has(sol *resolve.Solution, name string) bool {
	_, ok := sol.Packages[name]
	return ok
}

func loadSnapshotSource(t *testing.T) *pypi.Snapshot {
	t.Helper()
	snap, err := pypi.LoadSnapshot(snapshotPath)
	if err != nil {
		t.Fatalf("loading the index snapshot: %v", err)
	}
	return snap
}

// The whole point of an extra: asking for it brings in dependencies that asking
// for the plain package does not. flask publishes asgiref under its async extra,
// so the two resolves must differ by exactly that.
func TestExtraPullsInItsDependencies(t *testing.T) {
	snap := loadSnapshotSource(t)

	plain := resolveSpecs(t, snap, "flask")
	if has(plain, "asgiref") {
		t.Fatalf("plain flask pulled in asgiref: %v", names(plain))
	}

	withExtra := resolveSpecs(t, snap, "flask[async]")
	if !has(withExtra, "asgiref") {
		t.Fatalf("flask[async] did not pull in asgiref: %v", names(withExtra))
	}
	if len(withExtra.Packages) != len(plain.Packages)+1 {
		t.Errorf("flask[async] resolved to %d packages and flask to %d, want exactly one more",
			len(withExtra.Packages), len(plain.Packages))
	}
	if got := withExtra.Extras["flask"]; len(got) != 1 || got[0] != "async" {
		t.Errorf("selected extras = %v, want [async]", got)
	}
	// An extra is not a package to install, so it must not appear as one.
	for name := range withExtra.Packages {
		if strings.ContainsAny(name, "[]") {
			t.Errorf("solution contains %q, which is an extra rather than a package", name)
		}
	}
}

// Asking for an extra a package does publish leaves its ordinary dependencies
// exactly as they were. Only the extra's own requirements are added.
func TestExtraDoesNotDisturbTheBaseResolution(t *testing.T) {
	snap := loadSnapshotSource(t)

	plain := resolveSpecs(t, snap, "flask")
	withExtra := resolveSpecs(t, snap, "flask[async]")

	for name, v := range plain.Packages {
		got, ok := withExtra.Packages[name]
		if !ok {
			t.Errorf("%s disappeared when the extra was requested", name)
			continue
		}
		if got.String() != v.String() {
			t.Errorf("%s moved from %s to %s when the extra was requested", name, v, got)
		}
	}
}

// memIndex builds a small controlled index for the cases that are hard to find
// in real published metadata.
func memIndex(t *testing.T) *pypi.MemSource {
	t.Helper()
	m := pypi.NewMemSource()
	add := func(name, ver string, deps ...string) {
		if err := m.AddPackage(name, ver, deps...); err != nil {
			t.Fatal(err)
		}
	}

	// lib publishes two extras, one of which depends on the other.
	add("lib", "1.0",
		`extradep>=1.0; extra == "fancy"`,
		`otherdep; extra == "plus"`,
		`lib[fancy]; extra == "plus"`)
	add("lib", "2.0",
		`extradep>=1.0; extra == "fancy"`,
		`otherdep; extra == "plus"`,
		`lib[fancy]; extra == "plus"`)
	add("extradep", "1.0")
	add("otherdep", "1.0")

	// pin exists at two versions so a conflict can be constructed over it.
	add("pin", "1.0")
	add("pin", "2.0")

	// app 2.0 wants the extra and an old pin; app 1.0 wants neither.
	add("app", "2.0", "lib[fancy]", "pin==1.0")
	add("app", "1.0", "lib")
	add("other", "1.0", "pin==2.0")
	return m
}

// Selecting an extra selects the package it belongs to, at the same version.
// Nothing else ties them together.
func TestExtraForcesTheBasePackageAtTheSameVersion(t *testing.T) {
	sol := resolveSpecs(t, memIndex(t), "lib[fancy]==1.0")

	if got := sol.Packages["lib"]; got == nil || got.String() != "1.0" {
		t.Fatalf("lib = %v, want 1.0", got)
	}
	if !has(sol, "extradep") {
		t.Fatalf("the extra's dependency is missing: %v", names(sol))
	}
}

// An extra that asks for another extra of the same package follows through.
func TestExtraChainWithinAPackage(t *testing.T) {
	sol := resolveSpecs(t, memIndex(t), "lib[plus]")

	for _, want := range []string{"otherdep", "extradep"} {
		if !has(sol, want) {
			t.Errorf("%s is missing: %v", want, names(sol))
		}
	}
	got := sol.Extras["lib"]
	sort.Strings(got)
	if strings.Join(got, ",") != "fancy,plus" {
		t.Errorf("selected extras = %v, want [fancy plus]", got)
	}
}

// This is why an extra is modelled as its own package rather than as a flag on
// the package it belongs to.
//
// Dependency incompatibilities are permanent facts: once recorded they are never
// withdrawn, which is what lets the solver learn from a dead end instead of
// re-exploring it. If asking for lib[fancy] were recorded as "lib 1.0 requires
// extradep", that fact would outlive the decision that caused it. Here app 2.0
// asks for lib[fancy] and then loses a fight with other over pin, so app 1.0 is
// chosen instead and nothing wants the extra any more. extradep must not be in
// the solution.
func TestExtraDependenciesDoNotSurviveBacktracking(t *testing.T) {
	sol := resolveSpecs(t, memIndex(t), "app", "other")

	if got := sol.Packages["app"]; got == nil || got.String() != "1.0" {
		t.Fatalf("app = %v, want 1.0 after backtracking off 2.0", got)
	}
	if got := sol.Packages["pin"]; got == nil || got.String() != "2.0" {
		t.Fatalf("pin = %v, want 2.0", got)
	}
	if !has(sol, "lib") {
		t.Fatalf("lib is missing: %v", names(sol))
	}
	if has(sol, "extradep") {
		t.Fatalf("extradep survived the backtrack that removed the only thing asking for it: %v", names(sol))
	}
	if got := sol.Extras["lib"]; len(got) != 0 {
		t.Errorf("lib still carries extras %v after the backtrack", got)
	}
}

// Several extras at once bring in all of them.
func TestSeveralExtrasAtOnce(t *testing.T) {
	sol := resolveSpecs(t, memIndex(t), "lib[fancy,plus]")

	for _, want := range []string{"extradep", "otherdep"} {
		if !has(sol, want) {
			t.Errorf("%s is missing: %v", want, names(sol))
		}
	}
}

// Extra names are normalized the same way package names are, so Fancy, FANCY,
// and fancy are one extra rather than three.
func TestExtraNamesAreCanonicalized(t *testing.T) {
	sol := resolveSpecs(t, memIndex(t), "lib[FANCY]")

	if !has(sol, "extradep") {
		t.Fatalf("an extra named in a different case was not matched: %v", names(sol))
	}
	if got := sol.Extras["lib"]; len(got) != 1 || got[0] != "fancy" {
		t.Errorf("selected extras = %v, want [fancy]", got)
	}
}

// A package can be asked for an extra it does not publish. pip warns and carries
// on, and so does gopip: the package still resolves, with nothing extra.
func TestUnknownExtraResolvesToThePlainPackage(t *testing.T) {
	sol := resolveSpecs(t, memIndex(t), "lib[nosuchextra]")

	if !has(sol, "lib") {
		t.Fatalf("lib is missing: %v", names(sol))
	}
	if has(sol, "extradep") || has(sol, "otherdep") {
		t.Fatalf("an unknown extra pulled something in: %v", names(sol))
	}
}

// Verify is the independent check on a solution, so it has to notice an extra
// that was asked for and not selected. Without this, a resolver that dropped
// extras silently would still be reported as producing valid solutions, which is
// exactly the failure this work fixes.
func TestVerifyRejectsAMissingExtra(t *testing.T) {
	source := memIndex(t)
	sol := resolveSpecs(t, source, "lib[fancy]")

	// Take the extra away, as a resolver that ignored extras would have done.
	delete(sol.Extras, "lib")
	delete(sol.Packages, "extradep")

	req, err := requirement.Parse("lib[fancy]")
	if err != nil {
		t.Fatal(err)
	}
	err = resolve.Verify(context.Background(), source, fixtureEnvironment(),
		[]*requirement.Requirement{req}, sol)
	if err == nil {
		t.Fatal("Verify accepted a solution that dropped a requested extra")
	}
	if !strings.Contains(err.Error(), "fancy") {
		t.Errorf("error %q does not name the missing extra", err)
	}
}
