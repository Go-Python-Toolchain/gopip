package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/resolve"
	pyver "github.com/Go-Python-Toolchain/gopip/internal/version"
	"github.com/spf13/pflag"
)

// resolveOptions gathers the inputs shared by the resolving commands.
type resolveOptions struct {
	args     []string // requirement strings given on the command line
	reqFiles []string // requirements files given with -r
	python   string   // target Python version, empty means detect
	indexURL string   // JSON index base, empty means the default or GOPIP_INDEX_URL
	refresh  bool     // ignore cached metadata and fetch it again
	offline  bool     // resolve from cached metadata only
	noCache  bool     // do not read or write the cache at all
}

// addResolveFlags registers the flags every resolving command shares, so the
// four of them cannot drift apart.
func addResolveFlags(f *pflag.FlagSet, o *resolveOptions) {
	f.StringArrayVarP(&o.reqFiles, "requirement", "r", nil, "requirements file to read (repeatable)")
	f.StringVar(&o.python, "python", "", "target Python version, for example 3.12")
	f.StringVar(&o.indexURL, "index-url", "", "JSON index base URL (default the public index or GOPIP_INDEX_URL)")
	f.BoolVar(&o.refresh, "refresh", false, "ignore cached index metadata and fetch it again")
	f.BoolVar(&o.offline, "offline", false, "resolve from cached index metadata only, never reaching the network")
	f.BoolVar(&o.noCache, "no-cache", false, "neither read nor write the metadata cache")
}

// gatherRequirements collects requirements from command line arguments and from
// requirements files.
func gatherRequirements(o resolveOptions) ([]*requirement.Requirement, error) {
	var reqs []*requirement.Requirement
	for _, a := range o.args {
		req, err := requirement.Parse(a)
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, req)
	}
	for _, f := range o.reqFiles {
		fileReqs, err := loadRequirementsFile(f, map[string]bool{})
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, fileReqs...)
	}
	if len(reqs) == 0 {
		return nil, fmt.Errorf("no requirements given: pass them as arguments or with -r")
	}
	return reqs, nil
}

// loadRequirementsFile reads a requirements file and follows its -r and -c
// includes, relative to the file's directory, guarding against cycles.
func loadRequirementsFile(path string, seen map[string]bool) ([]*requirement.Requirement, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if seen[abs] {
		return nil, nil
	}
	seen[abs] = true

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rf, err := requirement.ParseRequirementsFile(string(data))
	if err != nil {
		return nil, err
	}

	reqs := append([]*requirement.Requirement(nil), rf.Requirements...)
	dir := filepath.Dir(path)
	includes := append(append([]string(nil), rf.Includes...), rf.Constraints...)
	for _, inc := range includes {
		incPath := inc
		if !filepath.IsAbs(incPath) {
			incPath = filepath.Join(dir, inc)
		}
		sub, err := loadRequirementsFile(incPath, seen)
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, sub...)
	}
	return reqs, nil
}

var pyVersionRe = regexp.MustCompile(`(\d+\.\d+)`)

// targetPython returns the explicit version, or the detected local interpreter
// version, or a sensible default.
func targetPython(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := detectPython(); v != "" {
		return v
	}
	return "3.12"
}

func detectPython() string {
	for _, exe := range []string{"python3", "python"} {
		out, err := exec.Command(exe, "--version").CombinedOutput()
		if err == nil {
			if m := pyVersionRe.FindString(string(out)); m != "" {
				return m
			}
		}
	}
	return ""
}

// jsonIndexBase returns the JSON index base URL to use, from the flag or the
// GOPIP_INDEX_URL environment variable.
func jsonIndexBase(flagVal string) string {
	v := flagVal
	if v == "" {
		v = os.Getenv("GOPIP_INDEX_URL")
	}
	return strings.TrimRight(v, "/")
}

// indexSource builds the metadata source for a run: the index client, wrapped
// in the on-disk cache unless the cache was turned off. A cache directory that
// cannot be determined is not fatal, since resolving without a cache is still
// correct, only slower.
func indexSource(o resolveOptions) (pypi.Source, *pypi.CachedSource, error) {
	base := jsonIndexBase(o.indexURL)

	var clientOpts []pypi.Option
	if base != "" {
		clientOpts = append(clientOpts, pypi.WithBaseURL(base))
	}
	client := pypi.NewClient(clientOpts...)

	if o.noCache {
		if o.offline {
			return nil, nil, fmt.Errorf("--offline needs the cache, so it cannot be used with --no-cache")
		}
		return client, nil, nil
	}

	root, err := pypi.CacheRoot()
	if err != nil {
		if o.offline {
			return nil, nil, fmt.Errorf("--offline needs a cache directory: %w", err)
		}
		return client, nil, nil
	}

	mode := pypi.CacheNormal
	switch {
	case o.offline && o.refresh:
		return nil, nil, fmt.Errorf("--offline and --refresh ask for opposite things")
	case o.offline:
		mode = pypi.CacheOffline
	case o.refresh:
		mode = pypi.CacheRefresh
	}

	cached := pypi.NewCachedSource(client, pypi.CacheDir(root, base), pypi.WithCacheMode(mode))
	return cached, cached, nil
}

// resolveInputs runs the resolver over the gathered requirements.
func resolveInputs(ctx context.Context, o resolveOptions) (*resolve.Solution, error) {
	reqs, err := gatherRequirements(o)
	if err != nil {
		return nil, err
	}

	py := targetPython(o.python)
	pyVer, err := pyver.Parse(py)
	if err != nil {
		return nil, fmt.Errorf("invalid python version %q: %w", py, err)
	}
	env := requirement.CurrentEnvironment(py)

	source, _, err := indexSource(o)
	if err != nil {
		return nil, err
	}

	r := resolve.New(source, resolve.WithEnvironment(env), resolve.WithPythonVersion(pyVer))
	return r.Resolve(ctx, reqs)
}

// pinnedRequirements returns the resolved packages as sorted name==version
// strings, the form pip consumes.
func pinnedRequirements(sol *resolve.Solution) []string {
	names := make([]string, 0, len(sol.Packages))
	for n := range sol.Packages {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, fmt.Sprintf("%s==%s", n, sol.Packages[n].String()))
	}
	return out
}
