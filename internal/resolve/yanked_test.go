package resolve_test

import (
	"context"
	"testing"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/resolve"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// resolveOne resolves a single requirement against the frozen index snapshot.
func resolveOne(t *testing.T, spec string) map[string]*version.Version {
	t.Helper()
	snap, err := pypi.LoadSnapshot(snapshotPath)
	if err != nil {
		t.Fatalf("loading the index snapshot: %v", err)
	}
	req, err := requirement.Parse(spec)
	if err != nil {
		t.Fatal(err)
	}
	r := resolve.New(snap,
		resolve.WithEnvironment(fixtureEnvironment()),
		resolve.WithPythonVersion(version.MustParse(fixturePython)))
	sol, err := r.Resolve(context.Background(), []*requirement.Requirement{req})
	if err != nil {
		t.Fatalf("resolving %s: %v", spec, err)
	}
	return sol.Packages
}

// Yanking a release is a maintainer saying "do not pick this". The snapshot
// carries a real one: pandas 3.0.4 is yanked and sits between two releases that
// are not, so a constraint that would otherwise land on it is the test.
func TestYankedReleaseIsNotSelected(t *testing.T) {
	got := resolveOne(t, "pandas<=3.0.4")["pandas"]
	if got == nil {
		t.Fatal("pandas is missing from the solution")
	}
	if got.String() == "3.0.4" {
		t.Fatal("selected pandas 3.0.4, which is yanked")
	}
	if got.String() != "3.0.3" {
		t.Fatalf("selected pandas %s, want 3.0.3, the highest release that is not yanked", got)
	}
}

// Asking for a yanked release by name is not the same as stumbling onto it. A
// pin is a deliberate choice, and pip honors it, so gopip does too: otherwise a
// project pinned to a yanked version could not be locked at all.
func TestYankedReleaseIsSelectedWhenPinned(t *testing.T) {
	got := resolveOne(t, "pandas==3.0.4")["pandas"]
	if got == nil {
		t.Fatal("pandas is missing from the solution")
	}
	if got.String() != "3.0.4" {
		t.Fatalf("selected pandas %s, want the pinned 3.0.4", got)
	}
}

// Without a constraint the newest release wins, and the yanked one below it is
// simply never in the running.
func TestYankedReleaseDoesNotDisturbTheUsualChoice(t *testing.T) {
	got := resolveOne(t, "pandas")["pandas"]
	if got == nil {
		t.Fatal("pandas is missing from the solution")
	}
	if got.String() != "3.0.5" {
		t.Fatalf("selected pandas %s, want 3.0.5", got)
	}
}
