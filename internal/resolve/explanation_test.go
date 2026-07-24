package resolve_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/resolve"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// resolveExpectingFailure resolves and requires the resolution to fail, giving
// back the error so its explanation can be read.
func resolveExpectingFailure(t *testing.T, source pypi.Source, specs ...string) error {
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
	if err == nil {
		t.Fatalf("resolving %v succeeded, expected a conflict: %v", specs, names(sol))
	}
	return err
}

// conflictIndex has a package whose dependency contradicts a direct
// requirement, which is the ordinary way a resolve fails in practice.
func conflictIndex(t *testing.T) *pypi.MemSource {
	t.Helper()
	m := pypi.NewMemSource()
	add := func(name, ver string, deps ...string) {
		if err := m.AddPackage(name, ver, deps...); err != nil {
			t.Fatal(err)
		}
	}
	add("app", "2.0", "lib>=3.0")
	add("lib", "3.0", "core>=2.0")
	add("lib", "1.0", "core>=1.0")
	add("core", "1.0")
	add("core", "2.0")
	return m
}

// The old message named a single constraint, which was the last thing the
// solver touched rather than the reason for the failure. An explanation has to
// name every requirement involved in the contradiction, including the one in the
// middle that the reader never wrote.
func TestFailureNamesTheWholeConflict(t *testing.T) {
	err := resolveExpectingFailure(t, conflictIndex(t), "app==2.0", "core<2.0")

	msg := err.Error()
	for _, want := range []string{
		"the root project depends on app ==2.0",
		"the root project depends on core <2.0",
		"lib 3.0 depends on core >=2.0",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the explanation does not mention %q:\n%s", want, msg)
		}
	}
	if !strings.Contains(msg, "cannot all be satisfied") {
		t.Errorf("the explanation does not say what the problem is:\n%s", msg)
	}
}

// The explanation is available structurally, not only as a formatted string, so
// it can be reported however a caller needs.
func TestExplainReturnsTheFacts(t *testing.T) {
	err := resolveExpectingFailure(t, conflictIndex(t), "app==2.0", "core<2.0")

	var re *resolve.ResolutionError
	if !asResolutionError(err, &re) {
		t.Fatalf("error is %T, want a *resolve.ResolutionError", err)
	}
	facts := re.Explain()
	if len(facts) < 3 {
		t.Fatalf("explanation has %d facts, want at least 3: %v", len(facts), facts)
	}
	for _, f := range facts {
		if strings.Contains(f, "$root") {
			t.Errorf("explanation leaks an internal name: %q", f)
		}
	}
	// Every fact should be stated once. A derivation visits the same requirement
	// repeatedly, and repeating it back is noise.
	seen := map[string]bool{}
	for _, f := range facts {
		if seen[f] {
			t.Errorf("fact repeated: %q", f)
		}
		seen[f] = true
	}
}

// A requirement no version satisfies is its own kind of failure and should say
// so rather than blaming something else.
func TestFailureOnAnImpossibleConstraint(t *testing.T) {
	err := resolveExpectingFailure(t, conflictIndex(t), "core>=99")

	msg := err.Error()
	if !strings.Contains(msg, "core") {
		t.Errorf("the explanation does not mention core:\n%s", msg)
	}
	if !strings.Contains(msg, "no matching version") {
		t.Errorf("the explanation does not say the constraint matches nothing:\n%s", msg)
	}
}

// A single-fact failure stays on one line. Ceremony around one sentence makes it
// harder to read, not easier.
func TestSingleFactFailureStaysShort(t *testing.T) {
	err := resolveExpectingFailure(t, conflictIndex(t), "core>=99")

	if strings.Contains(err.Error(), "\n") {
		t.Errorf("a one-fact failure should be one line:\n%s", err)
	}
}

func asResolutionError(err error, target **resolve.ResolutionError) bool {
	re, ok := err.(*resolve.ResolutionError)
	if ok {
		*target = re
	}
	return ok
}
