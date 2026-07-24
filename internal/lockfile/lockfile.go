// Package lockfile turns a resolution into gpt.lock, a deterministic JSON
// lockfile, and renders the dependency tree. The lockfile is a pure function of
// the resolution: package and dependency lists are sorted, so the same
// resolution always produces byte-identical output on any machine or operating
// system.
package lockfile

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/Go-Python-Toolchain/gopip/internal/resolve"
)

// formatVersion is the gpt.lock schema version.
const formatVersion = 1

// Lock is the content of a gpt.lock file.
type Lock struct {
	Version  int       `json:"version"`
	Roots    []string  `json:"roots"`
	Packages []Package `json:"packages"`
}

// Package is one locked package with the resolved packages it depends on.
type Package struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// Extras are the optional features of this package the resolution selected,
	// sorted. Absent when none were, so a lock without extras is unchanged.
	Extras       []string `json:"extras,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
}

// Build creates a lock from a resolution.
func Build(sol *resolve.Solution) *Lock {
	lock := &Lock{Version: formatVersion}

	lock.Roots = append(lock.Roots, sol.Roots...)
	sort.Strings(lock.Roots)

	names := make([]string, 0, len(sol.Packages))
	for name := range sol.Packages {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		deps := append([]string(nil), sol.Edges[name]...)
		sort.Strings(deps)
		extras := append([]string(nil), sol.Extras[name]...)
		sort.Strings(extras)
		lock.Packages = append(lock.Packages, Package{
			Name:         name,
			Version:      sol.Packages[name].String(),
			Extras:       extras,
			Dependencies: deps,
		})
	}
	return lock
}

// Marshal renders the lock as pretty-printed JSON with a trailing newline. The
// output is deterministic for a given lock.
func (l *Lock) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(l); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Parse reads a lock from its JSON form.
func Parse(data []byte) (*Lock, error) {
	var l Lock
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, err
	}
	return &l, nil
}
