package resolve_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
)

// The fixture is a frozen capture of real index metadata for the benchmark
// projects. Resolving against it is deterministic: the same inputs give the same
// lock on any machine, on any day, with no network. That is what makes it usable
// as the gate that later work must not move.
const (
	snapshotPath  = "testdata/snapshot.json"
	referenceDir  = "testdata/reference"
	projectsDir   = "../../examples/benchmark/projects"
	fixturePython = "3.12"
)

// benchmarkProjects are the same five requirement sets the published benchmarks
// use, so the offline gate and the timed comparison cover the same ground.
var benchmarkProjects = []string{
	"cli-tool",
	"web-api",
	"flask-app",
	"django-stack",
	"data-science",
}

// fixtureEnvironment is a pinned marker environment. It is deliberately not
// requirement.CurrentEnvironment, which reads the host platform: pinning it is
// what lets the reference locks be byte-identical on Linux, macOS, and Windows.
func fixtureEnvironment() requirement.Environment {
	return requirement.Environment{
		"python_version":                 "3.12",
		"python_full_version":            "3.12.0",
		"os_name":                        "posix",
		"sys_platform":                   "linux",
		"platform_system":                "Linux",
		"platform_machine":               "x86_64",
		"platform_release":               "",
		"platform_version":               "",
		"platform_python_implementation": "CPython",
		"implementation_name":            "cpython",
		"implementation_version":         "3.12.0",
		"extra":                          "",
	}
}

// loadProject reads one benchmark project's requirement set.
func loadProject(t *testing.T, name string) []*requirement.Requirement {
	t.Helper()
	path := filepath.Join(projectsDir, name+".txt")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("reading benchmark project %s: %v", name, err)
	}
	defer f.Close()

	var reqs []*requirement.Requirement
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		req, err := requirement.Parse(line)
		if err != nil {
			t.Fatalf("%s: parsing %q: %v", path, line, err)
		}
		reqs = append(reqs, req)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(reqs) == 0 {
		t.Fatalf("%s has no requirements", path)
	}
	return reqs
}
