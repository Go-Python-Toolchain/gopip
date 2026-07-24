package resolve_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/resolve"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// errFlaky stands in for the index being briefly unreachable.
var errFlaky = errors.New("connection reset by peer")

// faultySource fails the metadata lookup for one specific release and answers
// everything else normally, which is what a transient index failure looks like
// from inside a resolve.
type faultySource struct {
	inner pypi.Source

	failName    string
	failVersion string
	failWith    error
}

func (f *faultySource) Versions(ctx context.Context, name string) ([]*version.Version, error) {
	return f.inner.Versions(ctx, name)
}

func (f *faultySource) Release(ctx context.Context, name string, v *version.Version) (*pypi.ReleaseInfo, error) {
	if requirement.CanonicalizeName(name) == f.failName && v.String() == f.failVersion {
		return nil, f.failWith
	}
	return f.inner.Release(ctx, name, v)
}

func newFaultySource(t *testing.T, name, ver string, err error) *faultySource {
	t.Helper()
	inner := pypi.NewMemSource()
	for _, v := range []string{"1.0", "2.0", "3.0"} {
		if err := inner.AddPackage("sample", v); err != nil {
			t.Fatal(err)
		}
	}
	return &faultySource{inner: inner, failName: name, failVersion: ver, failWith: err}
}

func resolveWith(source pypi.Source, spec string) (*resolve.Solution, error) {
	req, err := requirement.Parse(spec)
	if err != nil {
		return nil, err
	}
	r := resolve.New(source,
		resolve.WithEnvironment(fixtureEnvironment()),
		resolve.WithPythonVersion(version.MustParse(fixturePython)))
	return r.Resolve(context.Background(), []*requirement.Requirement{req})
}

// Not knowing whether a version can be used is not the same as knowing it
// cannot. If the index cannot be read, the resolve has to say so: quietly
// dropping the version it failed on and settling for an older one produces a
// lockfile that looks like a decision and is really a network error.
func TestFailedMetadataLookupStopsTheResolve(t *testing.T) {
	source := newFaultySource(t, "sample", "3.0", errFlaky)

	sol, err := resolveWith(source, "sample")
	if err == nil {
		t.Fatalf("resolve succeeded with sample %s despite the index failing", sol.Packages["sample"])
	}
	if !errors.Is(err, errFlaky) {
		t.Fatalf("error = %v, want it to wrap the underlying failure", err)
	}
	if !strings.Contains(err.Error(), "sample") || !strings.Contains(err.Error(), "3.0") {
		t.Errorf("error %q does not say which release could not be read", err)
	}
}

// A version the index lists but has no metadata for is a different case: the
// release is not really there, so skipping it is correct and the resolve should
// carry on with the next one down.
func TestMissingReleaseIsSkipped(t *testing.T) {
	source := newFaultySource(t, "sample", "3.0", pypi.ErrNotFound)

	sol, err := resolveWith(source, "sample")
	if err != nil {
		t.Fatalf("a release the index does not have should be skipped, not fatal: %v", err)
	}
	if got := sol.Packages["sample"].String(); got != "2.0" {
		t.Fatalf("selected sample %s, want 2.0", got)
	}
}

// cancellingSource cancels the context part way through a resolve, standing in
// for a user pressing ctrl-c or a deadline expiring.
type cancellingSource struct {
	inner  pypi.Source
	cancel context.CancelFunc
	after  int32
	calls  int32
}

func (c *cancellingSource) trip(ctx context.Context) error {
	if atomic.AddInt32(&c.calls, 1) >= c.after {
		c.cancel()
	}
	return ctx.Err()
}

func (c *cancellingSource) Versions(ctx context.Context, name string) ([]*version.Version, error) {
	if err := c.trip(ctx); err != nil {
		return nil, err
	}
	return c.inner.Versions(ctx, name)
}

func (c *cancellingSource) Release(ctx context.Context, name string, v *version.Version) (*pypi.ReleaseInfo, error) {
	if err := c.trip(ctx); err != nil {
		return nil, err
	}
	return c.inner.Release(ctx, name, v)
}

// Candidate selection used to look up release metadata with a background
// context, so a cancelled resolve kept working through the slowest path it has.
// Cancellation has to reach every fetch.
func TestCancellationReachesCandidateSelection(t *testing.T) {
	inner := pypi.NewMemSource()
	for _, v := range []string{"1.0", "2.0", "3.0"} {
		if err := inner.AddPackage("sample", v); err != nil {
			t.Fatal(err)
		}
	}

	// Trip on the second lookup, which is the release-metadata fetch inside
	// candidate selection rather than the version listing before it. A resolve
	// that only honored cancellation on the listing would still pass here by
	// accident, so the count is what makes this test sharp.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &cancellingSource{inner: inner, cancel: cancel, after: 2}

	req, err := requirement.Parse("sample")
	if err != nil {
		t.Fatal(err)
	}
	r := resolve.New(source,
		resolve.WithEnvironment(fixtureEnvironment()),
		resolve.WithPythonVersion(version.MustParse(fixturePython)))

	if _, err := r.Resolve(ctx, []*requirement.Requirement{req}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
