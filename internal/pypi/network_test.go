package pypi

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestRealPyPI fetches live metadata from pypi.org. It is skipped by default so
// that the normal test run stays offline and deterministic. Enable it with
// GOPIP_NETWORK_TESTS=1.
func TestRealPyPI(t *testing.T) {
	if os.Getenv("GOPIP_NETWORK_TESTS") == "" {
		t.Skip("set GOPIP_NETWORK_TESTS=1 to run tests that reach pypi.org")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := NewClient()

	versions, err := c.Versions(ctx, "click")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) == 0 {
		t.Fatal("expected click to have released versions")
	}
	latest := versions[len(versions)-1]
	t.Logf("click has %d versions, latest %s", len(versions), latest)

	info, err := c.Release(ctx, "click", latest)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("click %s requires_python=%q with %d dependencies", latest, info.RequiresPython, len(info.RequiresDist))
}
