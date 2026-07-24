package resolve_test

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/resolve"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// capturedVersionsPerPackage is how many of the newest versions of each package
// have their release metadata recorded. The full version list is always
// recorded, so the resolver's search space matches the real index; this window
// only bounds how deep into a package's history the captured details go. It is
// wide enough that backtracking has real alternatives to explore.
const capturedVersionsPerPackage = 20

// recordingSource wraps a live source and notes which packages were touched, so
// the capture knows the exact closure the resolver walks.
type recordingSource struct {
	inner pypi.Source

	mu       sync.Mutex
	packages map[string]string // canonical name to the name as requested
	versions int
	releases int
}

func newRecordingSource(inner pypi.Source) *recordingSource {
	return &recordingSource{inner: inner, packages: map[string]string{}}
}

func (r *recordingSource) note(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.packages[name] = name
}

func (r *recordingSource) Versions(ctx context.Context, name string) ([]*version.Version, error) {
	r.note(name)
	r.mu.Lock()
	r.versions++
	r.mu.Unlock()
	return r.inner.Versions(ctx, name)
}

func (r *recordingSource) Release(ctx context.Context, name string, v *version.Version) (*pypi.ReleaseInfo, error) {
	r.note(name)
	r.mu.Lock()
	r.releases++
	r.mu.Unlock()
	return r.inner.Release(ctx, name, v)
}

func (r *recordingSource) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.packages))
	for _, n := range r.packages {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// TestCaptureSnapshot records the index metadata the benchmark projects need
// into testdata/snapshot.json. It reaches the network and rewrites a committed
// fixture, so it only runs on request:
//
//	GOPIP_CAPTURE=1 go test -run TestCaptureSnapshot ./internal/resolve/
//
// Re-run it when the fixture needs to be refreshed or widened, then re-record
// the reference locks with GOPIP_RECORD_REFERENCE=1.
func TestCaptureSnapshot(t *testing.T) {
	if os.Getenv("GOPIP_CAPTURE") == "" {
		t.Skip("set GOPIP_CAPTURE=1 to capture a fresh index snapshot")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	client := pypi.NewClient()
	rec := newRecordingSource(client)
	env := fixtureEnvironment()
	py := version.MustParse(fixturePython)

	// Walk every benchmark project once against the live index to learn the
	// closure of packages involved.
	for _, name := range benchmarkProjects {
		roots := loadProject(t, name)
		r := resolve.New(rec, resolve.WithEnvironment(env), resolve.WithPythonVersion(py))
		sol, err := r.Resolve(ctx, roots)
		if err != nil {
			t.Fatalf("resolving %s against the live index: %v", name, err)
		}
		t.Logf("%s: %d packages", name, len(sol.Packages))
	}

	names := rec.names()
	t.Logf("closure is %d packages, learned in %d version and %d release requests",
		len(names), rec.versions, rec.releases)

	// Record each package's full version list, then the newest releases of each,
	// fetched concurrently so a capture takes seconds rather than minutes.
	snap := pypi.NewSnapshot(pypi.DefaultBaseURL)
	var refs []pypi.Ref
	for _, name := range names {
		versions, err := client.Versions(ctx, name)
		if err != nil {
			t.Fatalf("versions of %s: %v", name, err)
		}
		snap.SetVersions(name, versions)

		start := 0
		if len(versions) > capturedVersionsPerPackage {
			start = len(versions) - capturedVersionsPerPackage
		}
		for _, v := range versions[start:] {
			refs = append(refs, pypi.Ref{Name: name, Version: v})
		}
	}

	results := client.FetchReleases(ctx, refs, 16)
	var failed int
	for _, res := range results {
		if res.Err != nil {
			// A release can be missing from the JSON API even when it is listed.
			// Skipping it is honest: the snapshot then behaves like the index does.
			failed++
			continue
		}
		snap.AddRelease(res.Info)
	}
	t.Logf("captured %d releases across %d packages, %d unavailable", len(results)-failed, len(names), failed)

	if err := snap.Save(snapshotPath); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("wrote %s (%.1f KB)\n", snapshotPath, float64(info.Size())/1024)
}
