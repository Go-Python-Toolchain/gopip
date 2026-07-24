package resolve_test

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/resolve"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// countingSource wraps a source and measures how many index requests a resolve
// makes and how long they take. It is the instrument behind the fetch profile:
// wall time on its own says a resolve is slow, but the split between request
// count and time per request says why.
type countingSource struct {
	inner pypi.Source

	versions   int64
	releases   int64
	versionsNs int64
	releasesNs int64
}

func (c *countingSource) Versions(ctx context.Context, name string) ([]*version.Version, error) {
	atomic.AddInt64(&c.versions, 1)
	start := time.Now()
	v, err := c.inner.Versions(ctx, name)
	atomic.AddInt64(&c.versionsNs, int64(time.Since(start)))
	return v, err
}

func (c *countingSource) Release(ctx context.Context, name string, v *version.Version) (*pypi.ReleaseInfo, error) {
	atomic.AddInt64(&c.releases, 1)
	start := time.Now()
	info, err := c.inner.Release(ctx, name, v)
	atomic.AddInt64(&c.releasesNs, int64(time.Since(start)))
	return info, err
}

// TestFetchProfile measures the index traffic of a real resolve, per benchmark
// project. It reaches the network, so it is skipped by default:
//
//	GOPIP_NETWORK_TESTS=1 go test -run TestFetchProfile -v ./internal/resolve/
//
// It prints a table of packages, wall time, and the request counts and time
// spent on each kind of request. This is how the fetch-strategy work is
// measured: run it before a change and after, and compare the rows.
//
// Time spent inside requests can exceed wall time once fetching runs
// concurrently. That gap is exactly the win, so it is worth reading as a ratio.
func TestFetchProfile(t *testing.T) {
	if os.Getenv("GOPIP_NETWORK_TESTS") == "" {
		t.Skip("set GOPIP_NETWORK_TESTS=1 to run tests that reach pypi.org")
	}

	env := fixtureEnvironment()
	py := version.MustParse(fixturePython)

	fmt.Printf("\n%-14s %9s %9s %9s %9s %9s %9s\n",
		"project", "packages", "wall", "versions", "vtime", "releases", "rtime")
	for _, name := range benchmarkProjects {
		roots := loadProject(t, name)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

		src := &countingSource{inner: pypi.NewClient()}
		r := resolve.New(src, resolve.WithEnvironment(env), resolve.WithPythonVersion(py))
		start := time.Now()
		sol, err := r.Resolve(ctx, roots)
		wall := time.Since(start)
		cancel()
		if err != nil {
			t.Fatalf("resolving %s: %v", name, err)
		}

		fmt.Printf("%-14s %9d %8.2fs %9d %8.2fs %9d %8.2fs\n",
			name, len(sol.Packages), wall.Seconds(),
			src.versions, time.Duration(src.versionsNs).Seconds(),
			src.releases, time.Duration(src.releasesNs).Seconds())
	}
}
