package pypi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// ErrNotCaptured means a package or version exists in a snapshot's version list
// but its release metadata was not recorded. It is distinct from ErrNotFound so
// a test can tell "the index does not have this" from "the capture is too
// narrow, widen it and record again".
var ErrNotCaptured = fmt.Errorf("not captured in the snapshot")

// Snapshot is a frozen capture of index metadata for a fixed set of packages. It
// exists so resolution can be tested against real, messy package data without a
// network and without the answer changing every time the index publishes a new
// release. A snapshot is a Source, so it drops in wherever the live client goes.
type Snapshot struct {
	// Index records which index the capture came from.
	Index string `json:"index"`
	// Packages is keyed by canonical package name.
	Packages map[string]*SnapshotPackage `json:"packages"`

	// parsed holds the decoded releases, filled on load and by Add.
	parsed map[string]map[string]*ReleaseInfo
}

// SnapshotPackage is one package's captured metadata: its full version list, and
// release details for the subset of versions that were recorded.
type SnapshotPackage struct {
	// Name is the package name as the index spells it.
	Name string `json:"name"`
	// Versions is every version the index listed, ascending. The full list is
	// always captured, because the resolver's search space depends on it.
	Versions []string `json:"versions"`
	// Releases holds recorded release metadata, keyed by normalized version.
	Releases map[string]*SnapshotRelease `json:"releases"`
}

// SnapshotRelease is the captured metadata of a single release.
type SnapshotRelease struct {
	RequiresPython string   `json:"requires_python,omitempty"`
	RequiresDist   []string `json:"requires_dist,omitempty"`
	Yanked         bool     `json:"yanked,omitempty"`
}

// NewSnapshot creates an empty snapshot for an index.
func NewSnapshot(index string) *Snapshot {
	return &Snapshot{
		Index:    index,
		Packages: map[string]*SnapshotPackage{},
		parsed:   map[string]map[string]*ReleaseInfo{},
	}
}

// SetVersions records a package's full version list, stored in version order
// rather than string order so the saved fixture reads the way the index does.
func (s *Snapshot) SetVersions(name string, versions []*version.Version) {
	pkg := s.pkg(name)
	pkg.Name = name

	ordered := append([]*version.Version(nil), versions...)
	sort.Slice(ordered, func(i, j int) bool { return version.Compare(ordered[i], ordered[j]) < 0 })

	pkg.Versions = make([]string, 0, len(ordered))
	for _, v := range ordered {
		pkg.Versions = append(pkg.Versions, v.String())
	}
}

// AddRelease records one release's metadata.
func (s *Snapshot) AddRelease(info *ReleaseInfo) {
	pkg := s.pkg(info.Name)
	if pkg.Name == "" {
		pkg.Name = info.Name
	}
	rel := &SnapshotRelease{RequiresPython: info.RequiresPython, Yanked: info.Yanked}
	for _, d := range info.RequiresDist {
		rel.RequiresDist = append(rel.RequiresDist, d.String())
	}
	key := info.Version.String()
	pkg.Releases[key] = rel

	canon := requirement.CanonicalizeName(info.Name)
	if s.parsed[canon] == nil {
		s.parsed[canon] = map[string]*ReleaseInfo{}
	}
	s.parsed[canon][key] = info
}

func (s *Snapshot) pkg(name string) *SnapshotPackage {
	canon := requirement.CanonicalizeName(name)
	if s.Packages == nil {
		s.Packages = map[string]*SnapshotPackage{}
	}
	p, ok := s.Packages[canon]
	if !ok {
		p = &SnapshotPackage{Name: name, Releases: map[string]*SnapshotRelease{}}
		s.Packages[canon] = p
	}
	if p.Releases == nil {
		p.Releases = map[string]*SnapshotRelease{}
	}
	return p
}

// Versions returns every version the capture recorded for a package, ascending.
func (s *Snapshot) Versions(_ context.Context, name string) ([]*version.Version, error) {
	pkg, ok := s.Packages[requirement.CanonicalizeName(name)]
	if !ok {
		return nil, ErrNotFound
	}
	out := make([]*version.Version, 0, len(pkg.Versions))
	for _, vs := range pkg.Versions {
		v, err := version.Parse(vs)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return version.Compare(out[i], out[j]) < 0 })
	return out, nil
}

// Release returns the captured metadata for a version.
func (s *Snapshot) Release(_ context.Context, name string, v *version.Version) (*ReleaseInfo, error) {
	canon := requirement.CanonicalizeName(name)
	if _, ok := s.Packages[canon]; !ok {
		return nil, ErrNotFound
	}
	info, ok := s.parsed[canon][v.String()]
	if !ok {
		return nil, fmt.Errorf("%s %s: %w", name, v, ErrNotCaptured)
	}
	return info, nil
}

// Save writes the snapshot as indented JSON. Map keys are emitted sorted by the
// encoder and version lists are sorted on record, so the file is stable across
// captures of the same data.
func (s *Snapshot) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// sortVersionStrings puts a stored version list into version order in place, so
// a snapshot behaves the same however its file was written or edited.
func sortVersionStrings(versions []string) error {
	parsed := make(map[string]*version.Version, len(versions))
	for _, vs := range versions {
		v, err := version.Parse(vs)
		if err != nil {
			return fmt.Errorf("version %q: %w", vs, err)
		}
		parsed[vs] = v
	}
	sort.Slice(versions, func(i, j int) bool {
		return version.Compare(parsed[versions[i]], parsed[versions[j]]) < 0
	})
	return nil
}

// LoadSnapshot reads a snapshot and decodes its releases.
func LoadSnapshot(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("reading snapshot %s: %w", path, err)
	}
	s.parsed = map[string]map[string]*ReleaseInfo{}
	for canon, pkg := range s.Packages {
		s.parsed[canon] = map[string]*ReleaseInfo{}
		if err := sortVersionStrings(pkg.Versions); err != nil {
			return nil, fmt.Errorf("snapshot %s: %s: %w", path, canon, err)
		}
		for vs, rel := range pkg.Releases {
			v, err := version.Parse(vs)
			if err != nil {
				return nil, fmt.Errorf("snapshot %s: version %q of %s: %w", path, vs, canon, err)
			}
			info := &ReleaseInfo{
				Name:           pkg.Name,
				Version:        v,
				RequiresPython: rel.RequiresPython,
				Yanked:         rel.Yanked,
			}
			for _, rd := range rel.RequiresDist {
				req, err := requirement.Parse(rd)
				if err != nil {
					continue // tolerate the same malformed metadata the live client tolerates
				}
				info.RequiresDist = append(info.RequiresDist, req)
			}
			s.parsed[canon][vs] = info
		}
	}
	return &s, nil
}

// Ensure Snapshot satisfies Source.
var _ Source = (*Snapshot)(nil)
