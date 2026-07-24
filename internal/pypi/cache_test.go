package pypi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// countingSource is a stand-in index that records how often it was asked, which
// is how these tests tell a cache hit from a fetch.
type countingSource struct {
	mu       sync.Mutex
	versions int
	releases int
	fail     error
}

func (c *countingSource) Versions(_ context.Context, name string) ([]*version.Version, error) {
	c.mu.Lock()
	c.versions++
	err := c.fail
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return []*version.Version{version.MustParse("1.0"), version.MustParse("2.0")}, nil
}

func (c *countingSource) Release(_ context.Context, name string, v *version.Version) (*ReleaseInfo, error) {
	c.mu.Lock()
	c.releases++
	err := c.fail
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	dep, err := requirement.Parse("other>=1.0; python_version >= '3.8'")
	if err != nil {
		return nil, err
	}
	return &ReleaseInfo{
		Name:           name,
		Version:        v,
		RequiresPython: ">=3.9",
		RequiresDist:   []*requirement.Requirement{dep},
		Yanked:         true,
	}, nil
}

func (c *countingSource) counts() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.versions, c.releases
}

// A second resolve should ask the index nothing it already knows.
func TestCachedSourceServesRepeatLookups(t *testing.T) {
	inner := &countingSource{}
	c := NewCachedSource(inner, t.TempDir())
	ctx := context.Background()
	v := version.MustParse("2.0")

	for i := 0; i < 3; i++ {
		if _, err := c.Versions(ctx, "Sample-Pkg"); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Release(ctx, "Sample-Pkg", v); err != nil {
			t.Fatal(err)
		}
	}

	gotV, gotR := inner.counts()
	if gotV != 1 || gotR != 1 {
		t.Fatalf("index was asked %d times for versions and %d for releases, want 1 and 1", gotV, gotR)
	}
	stats := c.Stats()
	if stats.Hits() != 4 || stats.Requests() != 2 {
		t.Fatalf("stats = %+v, want 4 hits and 2 requests", stats)
	}
}

// The cache must survive the process that wrote it, which is the entire point.
func TestCachedSourceSurvivesANewSource(t *testing.T) {
	dir := t.TempDir()
	inner := &countingSource{}
	ctx := context.Background()

	if _, err := NewCachedSource(inner, dir).Versions(ctx, "sample-pkg"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCachedSource(inner, dir).Versions(ctx, "sample-pkg"); err != nil {
		t.Fatal(err)
	}

	if gotV, _ := inner.counts(); gotV != 1 {
		t.Fatalf("index was asked %d times, want 1", gotV)
	}
}

// Cached metadata must come back the way it went in, including the yanked flag
// and the parsed dependency markers.
func TestCachedSourceRoundTripsMetadata(t *testing.T) {
	dir := t.TempDir()
	inner := &countingSource{}
	ctx := context.Background()
	v := version.MustParse("2.0")

	want, err := NewCachedSource(inner, dir).Release(ctx, "Sample-Pkg", v)
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewCachedSource(inner, dir).Release(ctx, "Sample-Pkg", v)
	if err != nil {
		t.Fatal(err)
	}

	if got.Name != want.Name || got.Version.String() != want.Version.String() {
		t.Errorf("identity = %s %s, want %s %s", got.Name, got.Version, want.Name, want.Version)
	}
	if got.RequiresPython != ">=3.9" {
		t.Errorf("requires-python = %q", got.RequiresPython)
	}
	if !got.Yanked {
		t.Error("yanked flag was lost")
	}
	if len(got.RequiresDist) != 1 || got.RequiresDist[0].Name != "other" {
		t.Fatalf("requires-dist = %v", got.RequiresDist)
	}
	if got.RequiresDist[0].Marker == nil {
		t.Error("dependency marker was lost")
	}
}

// A version list goes stale quickly, because someone may have published since.
// One release's metadata does not, because it is fixed at publication.
func TestCachedSourceExpiryDiffersByKind(t *testing.T) {
	dir := t.TempDir()
	inner := &countingSource{}
	ctx := context.Background()
	v := version.MustParse("2.0")

	clock := time.Now()
	c := NewCachedSource(inner, dir, WithVersionTTL(10*time.Minute), WithReleaseTTL(7*24*time.Hour))
	c.now = func() time.Time { return clock }

	if _, err := c.Versions(ctx, "sample-pkg"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Release(ctx, "sample-pkg", v); err != nil {
		t.Fatal(err)
	}

	clock = clock.Add(time.Hour)
	if _, err := c.Versions(ctx, "sample-pkg"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Release(ctx, "sample-pkg", v); err != nil {
		t.Fatal(err)
	}

	gotV, gotR := inner.counts()
	if gotV != 2 {
		t.Errorf("versions fetched %d times after an hour, want 2 (the list expired)", gotV)
	}
	if gotR != 1 {
		t.Errorf("release fetched %d times after an hour, want 1 (release metadata is still fresh)", gotR)
	}
}

// Refresh ignores what is stored and replaces it.
func TestCachedSourceRefreshBypassesTheCache(t *testing.T) {
	dir := t.TempDir()
	inner := &countingSource{}
	ctx := context.Background()

	if _, err := NewCachedSource(inner, dir).Versions(ctx, "sample-pkg"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCachedSource(inner, dir, WithCacheMode(CacheRefresh)).Versions(ctx, "sample-pkg"); err != nil {
		t.Fatal(err)
	}
	if gotV, _ := inner.counts(); gotV != 2 {
		t.Fatalf("index was asked %d times, want 2", gotV)
	}

	// The refreshed entry is stored, so a normal lookup after it is a hit.
	if _, err := NewCachedSource(inner, dir).Versions(ctx, "sample-pkg"); err != nil {
		t.Fatal(err)
	}
	if gotV, _ := inner.counts(); gotV != 2 {
		t.Fatalf("index was asked %d times after the refresh was stored, want 2", gotV)
	}
}

// Offline serves what is stored and refuses to reach the network for the rest,
// rather than quietly producing a resolve that used the network anyway.
func TestCachedSourceOfflineNeverFetches(t *testing.T) {
	dir := t.TempDir()
	inner := &countingSource{}
	ctx := context.Background()

	if _, err := NewCachedSource(inner, dir).Versions(ctx, "sample-pkg"); err != nil {
		t.Fatal(err)
	}
	offline := NewCachedSource(inner, dir, WithCacheMode(CacheOffline))

	if _, err := offline.Versions(ctx, "sample-pkg"); err != nil {
		t.Fatalf("a stored entry should be served offline: %v", err)
	}
	_, err := offline.Versions(ctx, "never-fetched")
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("offline miss error = %v, want ErrOffline", err)
	}
	if _, err := offline.Release(ctx, "sample-pkg", version.MustParse("2.0")); !errors.Is(err, ErrOffline) {
		t.Fatalf("offline release miss error = %v, want ErrOffline", err)
	}
	if gotV, gotR := inner.counts(); gotV != 1 || gotR != 0 {
		t.Fatalf("offline mode reached the index: %d version and %d release requests", gotV, gotR)
	}
}

// A damaged entry costs a fetch, never a wrong answer or a crash.
func TestCachedSourceRecoversFromDamagedEntries(t *testing.T) {
	dir := t.TempDir()
	inner := &countingSource{}
	ctx := context.Background()
	c := NewCachedSource(inner, dir)

	if _, err := c.Versions(ctx, "sample-pkg"); err != nil {
		t.Fatal(err)
	}
	path := c.versionsPath("sample-pkg")
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	versions, err := c.Versions(ctx, "sample-pkg")
	if err != nil {
		t.Fatalf("a damaged entry should be re-fetched, not fatal: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("versions = %v", versions)
	}
	if gotV, _ := inner.counts(); gotV != 2 {
		t.Fatalf("index was asked %d times, want 2", gotV)
	}
	// The damaged entry was replaced, so the next lookup is a hit again.
	if _, err := c.Versions(ctx, "sample-pkg"); err != nil {
		t.Fatal(err)
	}
	if gotV, _ := inner.counts(); gotV != 2 {
		t.Fatalf("the damaged entry was not replaced: %d requests", gotV)
	}
}

// A failed fetch must not leave anything behind that a later run would trust.
func TestCachedSourceDoesNotStoreFailures(t *testing.T) {
	dir := t.TempDir()
	inner := &countingSource{fail: ErrNotFound}
	c := NewCachedSource(inner, dir)

	if _, err := c.Versions(context.Background(), "sample-pkg"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(c.versionsPath("sample-pkg")); !os.IsNotExist(err) {
		t.Fatal("a failed fetch wrote a cache entry")
	}
}

// Two indexes must never answer for each other.
func TestCacheDirSeparatesIndexes(t *testing.T) {
	root := t.TempDir()
	public := CacheDir(root, DefaultBaseURL)
	private := CacheDir(root, "https://packages.example.com/pypi")
	trailing := CacheDir(root, DefaultBaseURL+"/")

	if public == private {
		t.Fatal("two different indexes share a cache directory")
	}
	if public != trailing {
		t.Fatal("a trailing slash changed the cache directory")
	}
	if filepath.Base(private)[:len("packages.example.com")] != "packages.example.com" {
		t.Errorf("cache directory %q is not readable", filepath.Base(private))
	}
	if CacheDir(root, "") != public {
		t.Error("an empty index URL should map to the default index")
	}
}

// Versions with characters that are awkward in a filename must still be
// storable, because PEP 440 allows them.
func TestCachedSourceHandlesAwkwardVersions(t *testing.T) {
	dir := t.TempDir()
	inner := &countingSource{}
	c := NewCachedSource(inner, dir)
	ctx := context.Background()

	for _, raw := range []string{"1!2.0", "1.0+local.build-2", "1.0rc1"} {
		v, err := version.Parse(raw)
		if err != nil {
			t.Fatalf("parsing %q: %v", raw, err)
		}
		if _, err := c.Release(ctx, "sample-pkg", v); err != nil {
			t.Fatalf("caching %s: %v", raw, err)
		}
		if _, err := c.Release(ctx, "sample-pkg", v); err != nil {
			t.Fatalf("reading back %s: %v", raw, err)
		}
	}
	if _, gotR := inner.counts(); gotR != 3 {
		t.Fatalf("index was asked %d times, want 3 (one per distinct version)", gotR)
	}
}

// Resolves run concurrently and share one cache directory, so writes must not
// tear and readers must never see a partial entry.
func TestCachedSourceIsConcurrencySafe(t *testing.T) {
	dir := t.TempDir()
	inner := &countingSource{}
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := NewCachedSource(inner, dir)
			for j := 0; j < 8; j++ {
				if _, err := c.Versions(ctx, "sample-pkg"); err != nil {
					errs <- err
					return
				}
				if _, err := c.Release(ctx, "sample-pkg", version.MustParse("2.0")); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	// Every temporary file must have been renamed into place or removed.
	entries, err := os.ReadDir(filepath.Join(dir, "sample-pkg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if len(e.Name()) >= 4 && e.Name()[:4] == ".tmp" {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
}
