package resolve_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/resolve"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// countingTransport measures a resolve's actual index traffic. Counting at the
// source interface would be misleading, because a lookup can be answered from
// metadata the index already sent; only the transport knows what really went
// over the network. It also records how many requests were in flight at once,
// which is the difference between fetching in parallel and merely intending to.
type countingTransport struct {
	inner http.RoundTripper

	mu       sync.Mutex
	requests int
	inFlight int
	peak     int
	busy     time.Duration
}

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.requests++
	t.inFlight++
	if t.inFlight > t.peak {
		t.peak = t.inFlight
	}
	t.mu.Unlock()

	start := time.Now()
	resp, err := t.inner.RoundTrip(req)
	elapsed := time.Since(start)

	t.mu.Lock()
	t.inFlight--
	t.busy += elapsed
	t.mu.Unlock()
	return resp, err
}

func (t *countingTransport) snapshot() (requests, peak int, busy time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.requests, t.peak, t.busy
}

// TestFetchProfile measures the index traffic of a real resolve, per benchmark
// project. It reaches the network, so it is skipped by default:
//
//	GOPIP_NETWORK_TESTS=1 go test -run TestFetchProfile -v ./internal/resolve/
//
// The request count is the durable figure: unlike wall time it does not move
// with the network, so it is what a change to the fetch strategy should be
// judged on. Peak concurrency says how much of the waiting was overlapped, and
// the ratio of time spent inside requests to wall time says the same thing from
// the other side.
func TestFetchProfile(t *testing.T) {
	if os.Getenv("GOPIP_NETWORK_TESTS") == "" {
		t.Skip("set GOPIP_NETWORK_TESTS=1 to run tests that reach pypi.org")
	}

	env := fixtureEnvironment()
	py := version.MustParse(fixturePython)

	fmt.Printf("\n%-14s %9s %8s %9s %6s %9s\n",
		"project", "packages", "wall", "requests", "peak", "in requests")
	for _, name := range benchmarkProjects {
		roots := loadProject(t, name)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

		counter := &countingTransport{inner: http.DefaultTransport}
		client := pypi.NewClient(pypi.WithHTTPClient(&http.Client{
			Timeout:   30 * time.Second,
			Transport: counter,
		}))

		r := resolve.New(client, resolve.WithEnvironment(env), resolve.WithPythonVersion(py))
		start := time.Now()
		sol, err := r.Resolve(ctx, roots)
		wall := time.Since(start)
		cancel()
		if err != nil {
			t.Fatalf("resolving %s: %v", name, err)
		}

		requests, peak, busy := counter.snapshot()
		fmt.Printf("%-14s %9d %7.2fs %9d %6d %8.2fs\n",
			name, len(sol.Packages), wall.Seconds(), requests, peak, busy.Seconds())
	}
}
