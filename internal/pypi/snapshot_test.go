package pypi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

func mustVersions(t *testing.T, raw ...string) []*version.Version {
	t.Helper()
	out := make([]*version.Version, 0, len(raw))
	for _, r := range raw {
		v, err := version.Parse(r)
		if err != nil {
			t.Fatalf("parsing %q: %v", r, err)
		}
		out = append(out, v)
	}
	return out
}

func sampleSnapshot(t *testing.T) *Snapshot {
	t.Helper()
	s := NewSnapshot(DefaultBaseURL)
	s.SetVersions("Sample-Pkg", mustVersions(t, "1.0", "1.1", "2.0"))

	dep, err := requirement.Parse("other>=1.0; python_version >= '3.8'")
	if err != nil {
		t.Fatal(err)
	}
	s.AddRelease(&ReleaseInfo{
		Name:           "Sample-Pkg",
		Version:        version.MustParse("2.0"),
		RequiresPython: ">=3.9",
		RequiresDist:   []*requirement.Requirement{dep},
		Yanked:         true,
	})
	return s
}

// A snapshot reports the full version list even for versions whose details were
// not captured, because the resolver's search space depends on the whole list.
func TestSnapshotVersionsCoverUncapturedReleases(t *testing.T) {
	s := sampleSnapshot(t)
	versions, err := s.Versions(context.Background(), "sample_pkg")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 {
		t.Fatalf("versions = %v, want 3", versions)
	}
	if versions[0].String() != "1.0" || versions[2].String() != "2.0" {
		t.Fatalf("versions not ascending: %v", versions)
	}
}

// An uncaptured version is reported distinctly from a package the index does not
// have, so a narrow capture is diagnosable rather than silently wrong.
func TestSnapshotDistinguishesMissingFromUncaptured(t *testing.T) {
	s := sampleSnapshot(t)

	if _, err := s.Release(context.Background(), "nope", version.MustParse("1.0")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown package error = %v, want ErrNotFound", err)
	}
	_, err := s.Release(context.Background(), "sample-pkg", version.MustParse("1.1"))
	if !errors.Is(err, ErrNotCaptured) {
		t.Fatalf("uncaptured version error = %v, want ErrNotCaptured", err)
	}
	if _, err := s.Versions(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown package versions error = %v, want ErrNotFound", err)
	}
}

// Saving and loading preserves the metadata the resolver reads, including the
// yanked flag and the parsed requirements.
func TestSnapshotRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := sampleSnapshot(t).Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}

	info, err := loaded.Release(context.Background(), "Sample-Pkg", version.MustParse("2.0"))
	if err != nil {
		t.Fatal(err)
	}
	if info.RequiresPython != ">=3.9" {
		t.Errorf("requires-python = %q", info.RequiresPython)
	}
	if !info.Yanked {
		t.Error("yanked flag was lost")
	}
	if len(info.RequiresDist) != 1 || info.RequiresDist[0].Name != "other" {
		t.Fatalf("requires-dist = %v", info.RequiresDist)
	}
	if info.RequiresDist[0].Marker == nil {
		t.Error("dependency marker was lost")
	}
}

// The same snapshot always serializes to the same bytes, so a committed fixture
// only changes when the captured data actually changes.
func TestSnapshotSaveIsStable(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.json")
	second := filepath.Join(dir, "b.json")

	if err := sampleSnapshot(t).Save(first); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Save(second); err != nil {
		t.Fatal(err)
	}

	a, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Error("saving a loaded snapshot produced different bytes")
	}
}
