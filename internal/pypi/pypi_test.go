package pypi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// fakeIndex serves a small canned JSON index for tests.
func fakeIndex(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/sample/json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"releases": {"1.0": [], "1.1": [], "2.0a1": [], "bogus": []}}`)
	})
	mux.HandleFunc("/sample/1.1/json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"info": {"name": "sample", "requires_python": ">=3.8",
			"requires_dist": ["requests>=2.0", "click; python_version < \"3.11\"", "!!bad!!"],
			"yanked": false}}`)
	})
	mux.HandleFunc("/missing/json", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	return httptest.NewServer(mux)
}

func TestClientVersions(t *testing.T) {
	srv := fakeIndex(t)
	defer srv.Close()
	c := NewClient(WithBaseURL(srv.URL))

	versions, err := c.Versions(context.Background(), "sample")
	if err != nil {
		t.Fatal(err)
	}
	// Three parseable versions, ascending; the bogus one is skipped.
	want := []string{"1.0", "1.1", "2.0a1"}
	if len(versions) != len(want) {
		t.Fatalf("got %d versions %v, want %d", len(versions), versions, len(want))
	}
	for i, w := range want {
		if versions[i].String() != w {
			t.Errorf("version %d = %q, want %q", i, versions[i], w)
		}
	}
}

func TestClientRelease(t *testing.T) {
	srv := fakeIndex(t)
	defer srv.Close()
	c := NewClient(WithBaseURL(srv.URL))

	info, err := c.Release(context.Background(), "sample", version.MustParse("1.1"))
	if err != nil {
		t.Fatal(err)
	}
	if info.RequiresPython != ">=3.8" {
		t.Errorf("requires_python = %q", info.RequiresPython)
	}
	// Two of the three dependencies parse; the malformed one is dropped.
	if len(info.RequiresDist) != 2 {
		t.Fatalf("got %d dependencies, want 2: %v", len(info.RequiresDist), info.RequiresDist)
	}
	if info.RequiresDist[0].Name != "requests" {
		t.Errorf("first dependency = %q", info.RequiresDist[0].Name)
	}
	if info.RequiresDist[1].Marker == nil {
		t.Error("expected the second dependency to carry a marker")
	}
}

func TestClientNotFound(t *testing.T) {
	srv := fakeIndex(t)
	defer srv.Close()
	c := NewClient(WithBaseURL(srv.URL))

	if _, err := c.Versions(context.Background(), "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestBackoffThenSuccess(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 2 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"releases": {"1.0": []}}`)
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithMinBackoff(time.Millisecond))
	versions, err := c.Versions(context.Background(), "flaky")
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("got %d versions, want 1", len(versions))
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls (2 failures then success), got %d", calls)
	}
}

func TestRetriesExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithMinBackoff(time.Millisecond), WithMaxRetries(2))
	if _, err := c.Versions(context.Background(), "down"); err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
}

func TestFetchReleasesConcurrent(t *testing.T) {
	var maxConcurrent, current int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&current, 1)
		for {
			m := atomic.LoadInt32(&maxConcurrent)
			if c <= m || atomic.CompareAndSwapInt32(&maxConcurrent, m, c) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&current, -1)
		fmt.Fprint(w, `{"info": {"name": "pkg"}}`)
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	var refs []Ref
	for i := 0; i < 50; i++ {
		refs = append(refs, Ref{Name: "pkg", Version: version.MustParse("1.0")})
	}

	results := c.FetchReleases(context.Background(), refs, 8)
	if len(results) != 50 {
		t.Fatalf("got %d results, want 50", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Fatalf("unexpected error: %v", r.Err)
		}
	}
	if maxConcurrent > 8 {
		t.Fatalf("concurrency limit exceeded: saw %d in flight, limit was 8", maxConcurrent)
	}
	if maxConcurrent < 2 {
		t.Fatalf("expected real concurrency, peak was only %d", maxConcurrent)
	}
}

func TestContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := NewClient(WithBaseURL(srv.URL), WithMinBackoff(50*time.Millisecond))
	if _, err := c.Versions(ctx, "x"); err == nil {
		t.Fatal("expected a cancellation error")
	}
}

// latestIndex serves a package document that carries its latest release's
// metadata, the way a real index does, and counts what is asked for.
func latestIndex(t *testing.T, hits *int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/sample/json", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		fmt.Fprint(w, `{"info": {"name": "sample", "version": "2.0", "requires_python": ">=3.9",
			"requires_dist": ["requests>=2.0"], "yanked": false},
			"releases": {"1.0": [], "2.0": []}}`)
	})
	mux.HandleFunc("/sample/1.0/json", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		fmt.Fprint(w, `{"info": {"name": "sample", "version": "1.0", "requires_python": ">=3.7",
			"requires_dist": ["requests>=1.0"], "yanked": true}}`)
	})
	return httptest.NewServer(mux)
}

// A package document already describes its latest release, and that is the
// version the resolver asks about for most packages. Asking the index again for
// something it just sent is a wasted round trip.
func TestClientReusesLatestReleaseMetadata(t *testing.T) {
	var hits int32
	srv := latestIndex(t, &hits)
	defer srv.Close()
	c := NewClient(WithBaseURL(srv.URL))
	ctx := context.Background()

	if _, err := c.Versions(ctx, "sample"); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("listing versions took %d requests, want 1", got)
	}

	info, err := c.Release(ctx, "sample", version.MustParse("2.0"))
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("the latest release cost %d more request(s), want 0", got-1)
	}
	if info.Name != "sample" || info.Version.String() != "2.0" {
		t.Errorf("identity = %s %s", info.Name, info.Version)
	}
	if info.RequiresPython != ">=3.9" {
		t.Errorf("requires-python = %q", info.RequiresPython)
	}
	if len(info.RequiresDist) != 1 || info.RequiresDist[0].Name != "requests" {
		t.Fatalf("requires-dist = %v", info.RequiresDist)
	}
}

// Only the latest release is described by the package document, so any other
// version must still be fetched, with its own distinct metadata.
func TestClientStillFetchesOlderReleases(t *testing.T) {
	var hits int32
	srv := latestIndex(t, &hits)
	defer srv.Close()
	c := NewClient(WithBaseURL(srv.URL))
	ctx := context.Background()

	if _, err := c.Versions(ctx, "sample"); err != nil {
		t.Fatal(err)
	}
	info, err := c.Release(ctx, "sample", version.MustParse("1.0"))
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("an older release took %d requests in total, want 2", got)
	}
	if info.RequiresPython != ">=3.7" || !info.Yanked {
		t.Errorf("older release metadata was not used: requires-python %q, yanked %v",
			info.RequiresPython, info.Yanked)
	}
	if len(info.RequiresDist) != 1 || info.RequiresDist[0].String() != "requests>=1.0" {
		t.Fatalf("requires-dist = %v", info.RequiresDist)
	}
}

// TestLatestMetadataMatchesTheReleaseDocument checks the assumption the reuse
// rests on against the real index: that a package document's info block says the
// same thing about the latest release as that release's own document. It is
// guarded because it reaches the network.
func TestLatestMetadataMatchesTheReleaseDocument(t *testing.T) {
	if os.Getenv("GOPIP_NETWORK_TESTS") == "" {
		t.Skip("set GOPIP_NETWORK_TESTS=1 to run tests that reach pypi.org")
	}

	ctx := context.Background()
	for _, name := range []string{"requests", "flask", "django", "numpy", "rich", "httpx"} {
		t.Run(name, func(t *testing.T) {
			reusing := NewClient()
			versions, err := reusing.Versions(ctx, name)
			if err != nil {
				t.Fatal(err)
			}
			if len(versions) == 0 {
				t.Fatal("no versions")
			}
			latest := reusing.cachedLatest(name, versions[len(versions)-1])
			if latest == nil {
				// The newest version by ordering is a pre-release the index does not
				// consider latest, which is a legitimate case, not a failure.
				t.Skipf("the newest version of %s is not the one the index calls latest", name)
			}

			// A separate client has nothing stored, so this really does fetch.
			direct, err := NewClient().Release(ctx, name, latest.Version)
			if err != nil {
				t.Fatal(err)
			}
			if latest.RequiresPython != direct.RequiresPython {
				t.Errorf("requires-python: reused %q, fetched %q", latest.RequiresPython, direct.RequiresPython)
			}
			if latest.Yanked != direct.Yanked {
				t.Errorf("yanked: reused %v, fetched %v", latest.Yanked, direct.Yanked)
			}
			if len(latest.RequiresDist) != len(direct.RequiresDist) {
				t.Fatalf("requires-dist: reused %d entries, fetched %d", len(latest.RequiresDist), len(direct.RequiresDist))
			}
			for i := range latest.RequiresDist {
				if latest.RequiresDist[i].String() != direct.RequiresDist[i].String() {
					t.Errorf("requires-dist %d: reused %q, fetched %q",
						i, latest.RequiresDist[i], direct.RequiresDist[i])
				}
			}
		})
	}
}
