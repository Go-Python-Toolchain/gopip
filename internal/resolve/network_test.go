package resolve

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// TestResolveRealPackages resolves popular packages from the live index and
// verifies each solution. It is skipped by default so the normal test run stays
// offline. Enable it with GOPIP_NETWORK_TESTS=1.
func TestResolveRealPackages(t *testing.T) {
	if os.Getenv("GOPIP_NETWORK_TESTS") == "" {
		t.Skip("set GOPIP_NETWORK_TESTS=1 to run tests that reach pypi.org")
	}

	cases := []string{"requests", "flask", "click", "rich", "httpx", "pydantic"}
	env := requirement.CurrentEnvironment("3.12")
	py := version.MustParse("3.12")
	client := pypi.NewClient()

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			req, err := requirement.Parse(name)
			if err != nil {
				t.Fatal(err)
			}
			roots := []*requirement.Requirement{req}
			sol, err := New(client, WithEnvironment(env), WithPythonVersion(py)).Resolve(ctx, roots)
			if err != nil {
				t.Fatalf("resolving %s: %v", name, err)
			}
			if err := Verify(ctx, client, env, roots, sol); err != nil {
				t.Fatalf("solution for %s is invalid: %v", name, err)
			}
			t.Logf("%s resolved to %d packages", name, len(sol.Packages))
		})
	}
}
