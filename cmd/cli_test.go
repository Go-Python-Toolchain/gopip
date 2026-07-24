package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/resolve"
	pyver "github.com/Go-Python-Toolchain/gopip/internal/version"
)

func TestPinnedRequirements(t *testing.T) {
	sol := &resolve.Solution{
		Packages: map[string]*pyver.Version{
			"flask":    pyver.MustParse("2.3.0"),
			"requests": pyver.MustParse("2.31.0"),
			"click":    pyver.MustParse("8.1.3"),
		},
	}
	got := pinnedRequirements(sol)
	want := []string{"click==8.1.3", "flask==2.3.0", "requests==2.31.0"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPipInstallArgs(t *testing.T) {
	got := pipInstallArgs([]string{"a==1.0", "b==2.0"}, []string{"--no-deps"})
	want := []string{"-m", "pip", "install", "a==1.0", "b==2.0", "--no-deps"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestJSONIndexBase(t *testing.T) {
	if got := jsonIndexBase("https://example.com/pypi/"); got != "https://example.com/pypi" {
		t.Fatalf("flag value: got %q", got)
	}
	t.Setenv("GOPIP_INDEX_URL", "https://env.example.com/pypi/")
	if got := jsonIndexBase(""); got != "https://env.example.com/pypi" {
		t.Fatalf("env value: got %q", got)
	}
}

func TestTargetPythonExplicit(t *testing.T) {
	if got := targetPython("3.11"); got != "3.11" {
		t.Fatalf("got %q", got)
	}
}

func TestGatherRequirementsFromArgs(t *testing.T) {
	reqs, err := gatherRequirements(resolveOptions{args: []string{"requests>=2.0", "flask"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 2 || reqs[0].Name != "requests" || reqs[1].Name != "flask" {
		t.Fatalf("unexpected requirements: %v", reqs)
	}
}

func TestGatherRequirementsEmpty(t *testing.T) {
	if _, err := gatherRequirements(resolveOptions{}); err == nil {
		t.Fatal("expected an error when no requirements are given")
	}
}

func TestLoadRequirementsFileWithInclude(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "requirements.txt")
	extra := filepath.Join(dir, "extra.txt")
	if err := os.WriteFile(extra, []byte("click>=8.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base, []byte("# base\nrequests>=2.0\n-r extra.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reqs, err := loadRequirementsFile(base, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, r := range reqs {
		names[r.Name] = true
	}
	if !names["requests"] || !names["click"] {
		t.Fatalf("expected requests and click from the include, got %v", names)
	}
}

// The cache flags are the one place a user can ask gopip for two opposite
// things, so contradictions must be refused rather than silently ranked.
func TestIndexSourceRejectsContradictoryCacheFlags(t *testing.T) {
	cases := []struct {
		name string
		opts resolveOptions
		want string
	}{
		{"offline with refresh", resolveOptions{offline: true, refresh: true}, "opposite"},
		{"offline with no cache", resolveOptions{offline: true, noCache: true}, "--no-cache"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := indexSource(c.opts)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

// Without --no-cache a resolve should be cached, and with it the cache is out
// of the picture entirely.
func TestIndexSourceCachesUnlessTurnedOff(t *testing.T) {
	source, cached, err := indexSource(resolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cached == nil || source != pypi.Source(cached) {
		t.Fatal("resolving should go through the cache by default")
	}

	source, cached, err = indexSource(resolveOptions{noCache: true})
	if err != nil {
		t.Fatal(err)
	}
	if cached != nil {
		t.Fatal("--no-cache still built a cache")
	}
	if _, ok := source.(*pypi.Client); !ok {
		t.Fatalf("--no-cache should resolve straight from the index client, got %T", source)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:         "0 B",
		512:       "512 B",
		1024:      "1.0 KB",
		69_530:    "67.9 KB",
		5_242_880: "5.0 MB",
	}
	for n, want := range cases {
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

// What gopip hands to pip has to be what it resolved, extras included. Handing
// over the bare package would install a different set from the one in the lock.
func TestPinnedRequirementsCarryExtras(t *testing.T) {
	sol := &resolve.Solution{
		Packages: map[string]*pyver.Version{
			"flask":   pyver.MustParse("3.1.3"),
			"asgiref": pyver.MustParse("3.12.1"),
			"uvicorn": pyver.MustParse("0.51.0"),
		},
		Extras: map[string][]string{
			"flask":   {"async"},
			"uvicorn": {"standard", "extra"},
		},
	}
	got := pinnedRequirements(sol)
	want := []string{"asgiref==3.12.1", "flask[async]==3.1.3", "uvicorn[extra,standard]==0.51.0"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// pip only accepts hashes inside a requirements file, so this rendering is the
// interface between a resolution and a verified install.
func TestHashedRequirements(t *testing.T) {
	sol := &resolve.Solution{
		Packages: map[string]*pyver.Version{
			"flask": pyver.MustParse("3.1.3"),
			"mdurl": pyver.MustParse("0.1.2"),
		},
		Extras: map[string][]string{"flask": {"async"}},
		Hashes: map[string][]string{
			"flask": {"sha256:aaa", "sha256:bbb"},
			"mdurl": {"sha256:ccc"},
		},
	}

	got, err := hashedRequirements(sol)
	if err != nil {
		t.Fatal(err)
	}
	want := "flask[async]==3.1.3 \\\n" +
		"    --hash=sha256:aaa \\\n" +
		"    --hash=sha256:bbb\n" +
		"mdurl==0.1.2 \\\n" +
		"    --hash=sha256:ccc\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// Verifying hashes is only meaningful if every package has some. Saying which
// one does not is more use than letting pip refuse the whole file.
func TestHashedRequirementsRefusesAPackageWithoutDigests(t *testing.T) {
	sol := &resolve.Solution{
		Packages: map[string]*pyver.Version{
			"flask": pyver.MustParse("3.1.3"),
			"mdurl": pyver.MustParse("0.1.2"),
		},
		Hashes: map[string][]string{"flask": {"sha256:aaa"}},
	}

	_, err := hashedRequirements(sol)
	if err == nil {
		t.Fatal("expected an error for the package with no digests")
	}
	if !strings.Contains(err.Error(), "mdurl") {
		t.Errorf("error %q does not name the package that is missing digests", err)
	}
}
