package resolve_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Go-Python-Toolchain/gopip/internal/lockfile"
	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/resolve"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// TestReferenceLocks is the gate every change to resolution has to pass. It
// resolves the five benchmark projects against the frozen index snapshot and
// requires the resulting lockfiles to match the committed references byte for
// byte. Caching, concurrency, and prefetching are all allowed to change how fast
// the answer arrives; none of them may change the answer.
//
// When a change is meant to alter resolution, for example excluding yanked
// releases or expanding extras, re-record the references in the same commit:
//
//	GOPIP_RECORD_REFERENCE=1 go test -run TestReferenceLocks ./internal/resolve/
//
// so the diff shows exactly which pins moved and why.
func TestReferenceLocks(t *testing.T) {
	snap, err := pypi.LoadSnapshot(snapshotPath)
	if err != nil {
		t.Fatalf("loading the index snapshot: %v", err)
	}
	record := os.Getenv("GOPIP_RECORD_REFERENCE") != ""
	env := fixtureEnvironment()
	py := version.MustParse(fixturePython)

	for _, name := range benchmarkProjects {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			roots := loadProject(t, name)

			r := resolve.New(snap, resolve.WithEnvironment(env), resolve.WithPythonVersion(py))
			sol, err := r.Resolve(ctx, roots)
			if err != nil {
				t.Fatalf("resolving %s: %v", name, err)
			}
			if err := resolve.Verify(ctx, snap, env, roots, sol); err != nil {
				t.Fatalf("solution for %s is invalid: %v", name, err)
			}
			got, err := lockfile.Build(sol).Marshal()
			if err != nil {
				t.Fatal(err)
			}

			path := filepath.Join(referenceDir, name+".lock")
			if record {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("recorded %s with %d packages", path, len(sol.Packages))
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading the reference lock: %v (record it with GOPIP_RECORD_REFERENCE=1)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s resolved to a different lock than the reference\n--- reference ---\n%s\n--- got ---\n%s",
					name, want, got)
			}
		})
	}
}

// TestReferenceLocksAreDeterministic resolves each project twice from a fresh
// resolver and requires identical bytes, which catches any dependence on map
// iteration order or on state left behind by a previous run.
func TestReferenceLocksAreDeterministic(t *testing.T) {
	snap, err := pypi.LoadSnapshot(snapshotPath)
	if err != nil {
		t.Fatalf("loading the index snapshot: %v", err)
	}
	env := fixtureEnvironment()
	py := version.MustParse(fixturePython)

	for _, name := range benchmarkProjects {
		t.Run(name, func(t *testing.T) {
			roots := loadProject(t, name)
			var first []byte
			for i := 0; i < 3; i++ {
				r := resolve.New(snap, resolve.WithEnvironment(env), resolve.WithPythonVersion(py))
				sol, err := r.Resolve(context.Background(), roots)
				if err != nil {
					t.Fatalf("run %d: %v", i, err)
				}
				data, err := lockfile.Build(sol).Marshal()
				if err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					first = data
					continue
				}
				if !bytes.Equal(data, first) {
					t.Fatalf("run %d differed from run 0", i)
				}
			}
		})
	}
}
