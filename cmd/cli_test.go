package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
