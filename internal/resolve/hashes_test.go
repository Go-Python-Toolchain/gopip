package resolve_test

import (
	"strings"
	"testing"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
)

// hashIndex is a small index whose releases publish artifacts, so the path from
// index metadata to a solution's digests can be checked without a network.
func hashIndex(t *testing.T) *pypi.MemSource {
	t.Helper()
	m := pypi.NewMemSource()
	if err := m.AddPackage("sample", "1.0", "dep"); err != nil {
		t.Fatal(err)
	}
	if err := m.AddPackage("dep", "1.0"); err != nil {
		t.Fatal(err)
	}
	if err := m.AddFiles("sample", "1.0",
		pypi.FileInfo{Filename: "sample-1.0-py3-none-any.whl", SHA256: "bbb"},
		pypi.FileInfo{Filename: "sample-1.0.tar.gz", SHA256: "aaa"},
	); err != nil {
		t.Fatal(err)
	}
	return m
}

// A resolution carries the digests of the artifacts published for each version
// it chose, which is what lets the lockfile pin what will be installed rather
// than only which version.
func TestSolutionCarriesArtifactDigests(t *testing.T) {
	sol := resolveSpecs(t, hashIndex(t), "sample")

	got := sol.Hashes["sample"]
	if len(got) != 2 {
		t.Fatalf("hashes = %v, want two", got)
	}
	if got[0] != "sha256:aaa" || got[1] != "sha256:bbb" {
		t.Fatalf("hashes = %v, want them sorted", got)
	}
	if _, ok := sol.Hashes["dep"]; ok {
		t.Errorf("dep has hashes %v, but the index published no artifacts for it", sol.Hashes["dep"])
	}
}

// Every artifact of a release is recorded, not just the one this machine would
// install, so a lock stays usable on another platform.
func TestAllArtifactsOfAReleaseAreRecorded(t *testing.T) {
	sol := resolveSpecs(t, hashIndex(t), "sample")

	joined := strings.Join(sol.Hashes["sample"], " ")
	for _, want := range []string{"sha256:aaa", "sha256:bbb"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%s is missing from %q", want, joined)
		}
	}
}
