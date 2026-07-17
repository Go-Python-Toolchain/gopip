package pypi

import (
	"context"
	"sort"

	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// MemSource is an in-memory Source. It backs tests and offline resolution from a
// local set of packages, such as a wheelhouse referenced with find-links.
type MemSource struct {
	// releases maps a canonical package name to its releases, keyed by the
	// normalized version string.
	releases map[string]map[string]*ReleaseInfo
}

// NewMemSource creates an empty in-memory source.
func NewMemSource() *MemSource {
	return &MemSource{releases: map[string]map[string]*ReleaseInfo{}}
}

// Add records a release. The package name is canonicalized for lookup.
func (m *MemSource) Add(info *ReleaseInfo) {
	key := requirement.CanonicalizeName(info.Name)
	if m.releases[key] == nil {
		m.releases[key] = map[string]*ReleaseInfo{}
	}
	m.releases[key][info.Version.String()] = info
}

// AddPackage is a convenience for registering a release from strings.
func (m *MemSource) AddPackage(name, ver string, deps ...string) error {
	v, err := version.Parse(ver)
	if err != nil {
		return err
	}
	info := &ReleaseInfo{Name: name, Version: v}
	for _, d := range deps {
		req, err := requirement.Parse(d)
		if err != nil {
			return err
		}
		info.RequiresDist = append(info.RequiresDist, req)
	}
	m.Add(info)
	return nil
}

// Versions returns the versions of a package, ascending.
func (m *MemSource) Versions(_ context.Context, name string) ([]*version.Version, error) {
	rels, ok := m.releases[requirement.CanonicalizeName(name)]
	if !ok {
		return nil, ErrNotFound
	}
	versions := make([]*version.Version, 0, len(rels))
	for _, info := range rels {
		versions = append(versions, info.Version)
	}
	sort.Slice(versions, func(i, j int) bool {
		return version.Compare(versions[i], versions[j]) < 0
	})
	return versions, nil
}

// Release returns the metadata for a specific version.
func (m *MemSource) Release(_ context.Context, name string, v *version.Version) (*ReleaseInfo, error) {
	rels, ok := m.releases[requirement.CanonicalizeName(name)]
	if !ok {
		return nil, ErrNotFound
	}
	info, ok := rels[v.String()]
	if !ok {
		return nil, ErrNotFound
	}
	return info, nil
}

// Ensure MemSource satisfies Source.
var _ Source = (*MemSource)(nil)
