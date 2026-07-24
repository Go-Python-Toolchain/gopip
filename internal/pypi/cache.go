package pypi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// A resolve asks the index the same two questions over and over: which versions
// of this package exist, and what does this exact release require. The answers
// have very different lifetimes. A version list changes whenever anyone
// publishes, so it is worth re-reading often. The metadata of one released
// version is fixed at publication and only ever changes if it is later yanked,
// so re-reading it on every resolve is pure waste. The cache treats them
// accordingly.
const (
	// DefaultVersionTTL bounds how stale a package's version list may be. Short,
	// because a resolve that silently ignores a release published this morning is
	// answering yesterday's question.
	DefaultVersionTTL = 10 * time.Minute
	// DefaultReleaseTTL bounds how stale one release's metadata may be. Long,
	// because the only part of it that can change after publication is the yanked
	// flag.
	DefaultReleaseTTL = 7 * 24 * time.Hour
)

// CacheMode selects how the cache and the index are used together.
type CacheMode int

const (
	// CacheNormal serves entries that are still fresh and fetches the rest.
	CacheNormal CacheMode = iota
	// CacheRefresh ignores what is stored, fetches everything, and rewrites it.
	CacheRefresh
	// CacheOffline serves only what is stored and never reaches the network.
	CacheOffline
)

// ErrOffline reports that a lookup was needed but the cache did not have it and
// fetching was disallowed.
var ErrOffline = errors.New("not in the cache and offline")

// CacheStats counts what a cached source did, so a run can report how much of it
// avoided the network.
type CacheStats struct {
	VersionHits   int64
	VersionMisses int64
	ReleaseHits   int64
	ReleaseMisses int64
}

// Requests returns how many index requests the misses implied.
func (s CacheStats) Requests() int64 { return s.VersionMisses + s.ReleaseMisses }

// Hits returns how many lookups were served from disk.
func (s CacheStats) Hits() int64 { return s.VersionHits + s.ReleaseHits }

// CachedSource wraps a source with an on-disk metadata cache, so a second
// resolve does little or no network work. Entries are written atomically and an
// unreadable entry is treated as a miss, so a cache damaged by a crash or a full
// disk costs time and never correctness.
type CachedSource struct {
	inner Source
	dir   string
	mode  CacheMode

	versionTTL time.Duration
	releaseTTL time.Duration
	now        func() time.Time

	versionHits   int64
	versionMisses int64
	releaseHits   int64
	releaseMisses int64
}

// CacheOption configures a CachedSource.
type CacheOption func(*CachedSource)

// WithCacheMode sets whether the cache is consulted, bypassed, or exclusive.
func WithCacheMode(m CacheMode) CacheOption {
	return func(c *CachedSource) { c.mode = m }
}

// WithVersionTTL overrides how long a cached version list stays fresh.
func WithVersionTTL(d time.Duration) CacheOption {
	return func(c *CachedSource) { c.versionTTL = d }
}

// WithReleaseTTL overrides how long cached release metadata stays fresh.
func WithReleaseTTL(d time.Duration) CacheOption {
	return func(c *CachedSource) { c.releaseTTL = d }
}

// NewCachedSource wraps a source with a cache rooted at dir, which should
// already be specific to one index. Use CacheDir to derive it.
func NewCachedSource(inner Source, dir string, opts ...CacheOption) *CachedSource {
	c := &CachedSource{
		inner:      inner,
		dir:        dir,
		versionTTL: DefaultVersionTTL,
		releaseTTL: DefaultReleaseTTL,
		now:        time.Now,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Stats reports what the cache did during this run.
func (c *CachedSource) Stats() CacheStats {
	return CacheStats{
		VersionHits:   atomic.LoadInt64(&c.versionHits),
		VersionMisses: atomic.LoadInt64(&c.versionMisses),
		ReleaseHits:   atomic.LoadInt64(&c.releaseHits),
		ReleaseMisses: atomic.LoadInt64(&c.releaseMisses),
	}
}

// Dir returns the directory this source caches into.
func (c *CachedSource) Dir() string { return c.dir }

// versionEntry is the stored form of a package's version list.
type versionEntry struct {
	Fetched  time.Time `json:"fetched"`
	Name     string    `json:"name"`
	Versions []string  `json:"versions"`
}

// releaseEntry is the stored form of one release's metadata.
type releaseEntry struct {
	Fetched        time.Time  `json:"fetched"`
	Name           string     `json:"name"`
	RequiresPython string     `json:"requires_python,omitempty"`
	RequiresDist   []string   `json:"requires_dist,omitempty"`
	Yanked         bool       `json:"yanked,omitempty"`
	Files          []FileInfo `json:"files,omitempty"`
}

// Versions returns a package's versions, from the cache when fresh.
func (c *CachedSource) Versions(ctx context.Context, name string) ([]*version.Version, error) {
	path := c.versionsPath(name)

	if c.mode != CacheRefresh {
		var entry versionEntry
		if c.read(path, &entry, c.versionTTL) {
			out := make([]*version.Version, 0, len(entry.Versions))
			for _, vs := range entry.Versions {
				if v, err := version.Parse(vs); err == nil {
					out = append(out, v)
				}
			}
			sort.Slice(out, func(i, j int) bool { return version.Compare(out[i], out[j]) < 0 })
			atomic.AddInt64(&c.versionHits, 1)
			return out, nil
		}
	}
	if c.mode == CacheOffline {
		return nil, fmt.Errorf("versions of %s: %w", name, ErrOffline)
	}

	atomic.AddInt64(&c.versionMisses, 1)
	versions, err := c.inner.Versions(ctx, name)
	if err != nil {
		return nil, err
	}

	entry := versionEntry{Fetched: c.now(), Name: name}
	for _, v := range versions {
		entry.Versions = append(entry.Versions, v.String())
	}
	c.write(path, entry)
	return versions, nil
}

// Release returns one release's metadata, from the cache when fresh.
func (c *CachedSource) Release(ctx context.Context, name string, v *version.Version) (*ReleaseInfo, error) {
	path := c.releasePath(name, v)

	if c.mode != CacheRefresh {
		var entry releaseEntry
		if c.read(path, &entry, c.releaseTTL) {
			atomic.AddInt64(&c.releaseHits, 1)
			return entry.info(v), nil
		}
	}
	if c.mode == CacheOffline {
		return nil, fmt.Errorf("%s %s: %w", name, v, ErrOffline)
	}

	atomic.AddInt64(&c.releaseMisses, 1)
	info, err := c.inner.Release(ctx, name, v)
	if err != nil {
		return nil, err
	}
	c.write(path, newReleaseEntry(c.now(), info))
	return info, nil
}

func newReleaseEntry(now time.Time, info *ReleaseInfo) releaseEntry {
	entry := releaseEntry{
		Fetched:        now,
		Name:           info.Name,
		RequiresPython: info.RequiresPython,
		Yanked:         info.Yanked,
		Files:          info.Files,
	}
	for _, d := range info.RequiresDist {
		entry.RequiresDist = append(entry.RequiresDist, d.String())
	}
	return entry
}

func (e releaseEntry) info(v *version.Version) *ReleaseInfo {
	info := &ReleaseInfo{
		Name:           e.Name,
		Version:        v,
		RequiresPython: e.RequiresPython,
		Yanked:         e.Yanked,
		Files:          e.Files,
	}
	for _, rd := range e.RequiresDist {
		req, err := requirement.Parse(rd)
		if err != nil {
			continue // tolerate the same malformed metadata the live client tolerates
		}
		info.RequiresDist = append(info.RequiresDist, req)
	}
	return info
}

// read loads an entry and reports whether it was usable and still fresh. An
// entry that cannot be read or parsed is removed and reported as a miss.
func (c *CachedSource) read(path string, into interface{ fetchedAt() time.Time }, ttl time.Duration) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(data, into); err != nil {
		os.Remove(path)
		return false
	}
	fetched := into.fetchedAt()
	if fetched.IsZero() {
		os.Remove(path)
		return false
	}
	if ttl > 0 && c.now().Sub(fetched) > ttl {
		return false
	}
	return true
}

func (e *versionEntry) fetchedAt() time.Time { return e.Fetched }
func (e *releaseEntry) fetchedAt() time.Time { return e.Fetched }

// write stores an entry atomically: a temporary file in the same directory,
// renamed into place, so a reader never sees a half-written entry. A cache that
// cannot be written is not an error, only a lost optimization.
func (c *CachedSource) write(path string, entry any) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
	}
}

func (c *CachedSource) packageDir(name string) string {
	return filepath.Join(c.dir, requirement.CanonicalizeName(name))
}

func (c *CachedSource) versionsPath(name string) string {
	return filepath.Join(c.packageDir(name), "versions.json")
}

func (c *CachedSource) releasePath(name string, v *version.Version) string {
	// Versions may contain characters that are awkward in a filename, such as the
	// epoch marker or a local-version plus, so the name is escaped.
	return filepath.Join(c.packageDir(name), url.PathEscape(v.String())+".json")
}

// Ensure CachedSource satisfies Source.
var _ Source = (*CachedSource)(nil)

// CacheRoot returns the base directory gopip caches into, honoring the usual
// per-platform cache location.
func CacheRoot() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gpt", "gopip"), nil
}

// CacheUsage describes what is stored under one index's cache directory.
type CacheUsage struct {
	Dir      string
	Packages int
	Entries  int
	Bytes    int64
}

// Usage measures one index's cache directory. A directory that does not exist
// is reported as empty rather than as an error, because an unused cache is a
// normal state.
func Usage(dir string) (CacheUsage, error) {
	usage := CacheUsage{Dir: dir}
	packages, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return usage, nil
	}
	if err != nil {
		return usage, err
	}

	for _, pkg := range packages {
		if !pkg.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(dir, pkg.Name()))
		if err != nil {
			continue
		}
		usage.Packages++
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			usage.Entries++
			usage.Bytes += info.Size()
		}
	}
	return usage, nil
}

// ListCaches measures every index cache under a root, sorted by directory so the
// listing is stable.
func ListCaches(root string) ([]CacheUsage, error) {
	dirs, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var out []CacheUsage
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		usage, err := Usage(filepath.Join(root, d.Name()))
		if err != nil {
			continue
		}
		out = append(out, usage)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out, nil
}

// CacheDir returns the cache directory for one index. Entries are separated by
// index so a private index and the public one never answer for each other, with
// a readable host name for anyone looking through the cache and a digest to keep
// distinct URLs distinct.
func CacheDir(root, indexURL string) string {
	if indexURL == "" {
		indexURL = DefaultBaseURL
	}
	normalized := strings.TrimRight(indexURL, "/")

	label := "index"
	if u, err := url.Parse(normalized); err == nil && u.Host != "" {
		label = strings.ReplaceAll(u.Host, ":", "_")
	}
	sum := sha256.Sum256([]byte(normalized))
	return filepath.Join(root, label+"-"+hex.EncodeToString(sum[:])[:12])
}
