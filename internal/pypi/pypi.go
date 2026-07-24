// Package pypi fetches package metadata from a Python package index using the
// JSON API. It pools connections, retries transient failures with exponential
// backoff, and can fetch many releases concurrently. A Source interface lets the
// resolver run against the network client or an in-memory source in tests.
package pypi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// DefaultBaseURL is the JSON API root of the public Python Package Index.
const DefaultBaseURL = "https://pypi.org/pypi"

// ErrNotFound is returned when a package or release does not exist.
var ErrNotFound = errors.New("not found")

// ReleaseInfo is the metadata gopip needs about one release of a package.
type ReleaseInfo struct {
	Name           string
	Version        *version.Version
	RequiresPython string
	RequiresDist   []*requirement.Requirement
	Yanked         bool
	// Files are the artifacts published for this release, one per wheel or
	// source distribution. A release usually has several, because a wheel is
	// built per platform, and any of them may be the one that gets installed.
	Files []FileInfo
}

// FileInfo identifies one published artifact of a release by its name and the
// digest the index publishes for it.
type FileInfo struct {
	Filename string
	SHA256   string
}

// Hashes returns the release's artifact digests in pip's hash syntax, sorted and
// without duplicates. Every artifact is listed because which one gets installed
// depends on the machine doing the installing.
func (r *ReleaseInfo) Hashes() []string {
	if len(r.Files) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(r.Files))
	for _, f := range r.Files {
		if f.SHA256 == "" {
			continue
		}
		h := "sha256:" + f.SHA256
		if seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// Source provides package versions and release metadata.
type Source interface {
	// Versions returns all available versions of a package, ascending.
	Versions(ctx context.Context, name string) ([]*version.Version, error)
	// Release returns the metadata for a specific version.
	Release(ctx context.Context, name string, v *version.Version) (*ReleaseInfo, error)
}

// Client fetches metadata from a JSON package index.
type Client struct {
	baseURL    string
	http       *http.Client
	maxRetries int
	minBackoff time.Duration

	// A package document carries the full metadata of its latest release
	// alongside the version list. Keeping it means the resolver, which picks the
	// latest version for most packages, does not have to ask for something the
	// index already sent.
	mu     sync.Mutex
	latest map[string]*ReleaseInfo
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL sets the JSON API root, for a mirror or a private index.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient supplies a custom HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithMaxRetries sets how many times a transient failure is retried.
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// WithMinBackoff sets the initial backoff delay, which doubles per retry.
func WithMinBackoff(d time.Duration) Option {
	return func(c *Client) { c.minBackoff = d }
}

// NewClient creates a client with connection pooling suited to fetching many
// small metadata documents.
func NewClient(opts ...Option) *Client {
	transport := &http.Transport{
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 256,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	c := &Client{
		baseURL:    DefaultBaseURL,
		http:       &http.Client{Timeout: 30 * time.Second, Transport: transport},
		maxRetries: 5,
		minBackoff: 200 * time.Millisecond,
		latest:     map[string]*ReleaseInfo{},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// get fetches a URL, retrying on rate limiting and server errors with
// exponential backoff. A 404 maps to ErrNotFound.
func (c *Client) get(ctx context.Context, u string) ([]byte, error) {
	backoff := c.minBackoff
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "gopip")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusOK:
			if readErr != nil {
				lastErr = readErr
				continue
			}
			return body, nil
		case resp.StatusCode == http.StatusNotFound:
			return nil, ErrNotFound
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("index returned status %d", resp.StatusCode)
			if secs := retryAfter(resp.Header.Get("Retry-After")); secs > 0 {
				backoff = time.Duration(secs) * time.Second
			}
		default:
			return nil, fmt.Errorf("index returned status %d", resp.StatusCode)
		}
	}
	if lastErr == nil {
		lastErr = errors.New("request failed")
	}
	return nil, lastErr
}

func retryAfter(h string) int {
	if h == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// packageDocument is the shape gopip reads out of an index's JSON responses.
// The same info block appears in a package document, where it describes the
// latest release, and in a release document, where it describes that release.
type packageDocument struct {
	Info struct {
		Name           string   `json:"name"`
		Version        string   `json:"version"`
		RequiresPython string   `json:"requires_python"`
		RequiresDist   []string `json:"requires_dist"`
		Yanked         bool     `json:"yanked"`
	} `json:"info"`
	// URLs lists a release document's artifacts. A package document carries the
	// same shape per version under Releases instead.
	URLs     []fileDocument             `json:"urls"`
	Releases map[string]json.RawMessage `json:"releases"`
}

// fileDocument is one published artifact as the index describes it.
type fileDocument struct {
	Filename string `json:"filename"`
	Digests  struct {
		SHA256 string `json:"sha256"`
	} `json:"digests"`
}

func filesFrom(docs []fileDocument) []FileInfo {
	if len(docs) == 0 {
		return nil
	}
	out := make([]FileInfo, 0, len(docs))
	for _, d := range docs {
		out = append(out, FileInfo{Filename: d.Filename, SHA256: d.Digests.SHA256})
	}
	return out
}

// releaseInfo turns an info block into release metadata for a known version.
func (d *packageDocument) releaseInfo(name string, v *version.Version) *ReleaseInfo {
	if d.Info.Name != "" {
		name = d.Info.Name
	}
	info := &ReleaseInfo{
		Name:           name,
		Version:        v,
		RequiresPython: d.Info.RequiresPython,
		Yanked:         d.Info.Yanked,
	}
	for _, rd := range d.Info.RequiresDist {
		req, err := requirement.Parse(rd)
		if err != nil {
			continue // tolerate a malformed dependency rather than failing the release
		}
		info.RequiresDist = append(info.RequiresDist, req)
	}
	info.Files = filesFrom(d.URLs)
	return info
}

// Versions returns the parseable versions of a package, ascending. Unparseable
// version strings in the index are skipped rather than failing the whole call.
//
// The response also describes the package's latest release, which is the
// version the resolver goes on to ask about for most packages, so that metadata
// is kept rather than fetched a second time.
func (c *Client) Versions(ctx context.Context, name string) ([]*version.Version, error) {
	body, err := c.get(ctx, c.baseURL+"/"+url.PathEscape(name)+"/json")
	if err != nil {
		return nil, err
	}
	var doc packageDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decoding metadata for %q: %w", name, err)
	}

	versions := make([]*version.Version, 0, len(doc.Releases))
	for vs := range doc.Releases {
		if v, err := version.Parse(vs); err == nil {
			versions = append(versions, v)
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		return version.Compare(versions[i], versions[j]) < 0
	})

	if latest, err := version.Parse(doc.Info.Version); err == nil {
		info := doc.releaseInfo(name, latest)
		// A package document lists every version's artifacts, so the latest
		// release's files are here too, under the version string the index used.
		if raw, ok := doc.Releases[doc.Info.Version]; ok {
			var files []fileDocument
			if err := json.Unmarshal(raw, &files); err == nil {
				info.Files = filesFrom(files)
			}
		}
		c.mu.Lock()
		c.latest[requirement.CanonicalizeName(name)] = info
		c.mu.Unlock()
	}
	return versions, nil
}

// cachedLatest returns the release metadata already received alongside a version
// list, when it describes the version being asked for.
func (c *Client) cachedLatest(name string, v *version.Version) *ReleaseInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	info, ok := c.latest[requirement.CanonicalizeName(name)]
	if !ok || info.Version.String() != v.String() {
		return nil
	}
	return info
}

// Release returns the metadata for a specific version.
func (c *Client) Release(ctx context.Context, name string, v *version.Version) (*ReleaseInfo, error) {
	if info := c.cachedLatest(name, v); info != nil {
		return info, nil
	}

	body, err := c.get(ctx, c.baseURL+"/"+url.PathEscape(name)+"/"+url.PathEscape(v.String())+"/json")
	if err != nil {
		return nil, err
	}
	var doc packageDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decoding release metadata for %q %s: %w", name, v, err)
	}
	return doc.releaseInfo(name, v), nil
}

// Ref names a specific release to fetch.
type Ref struct {
	Name    string
	Version *version.Version
}

// Result pairs a Ref with its fetched metadata or an error.
type Result struct {
	Ref  Ref
	Info *ReleaseInfo
	Err  error
}

// FetchReleases fetches many releases concurrently, bounded by concurrency. The
// results are returned in the same order as refs.
func (c *Client) FetchReleases(ctx context.Context, refs []Ref, concurrency int) []Result {
	if concurrency < 1 {
		concurrency = 1
	}
	results := make([]Result, len(refs))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, ref := range refs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, ref Ref) {
			defer wg.Done()
			defer func() { <-sem }()
			info, err := c.Release(ctx, ref.Name, ref.Version)
			results[i] = Result{Ref: ref, Info: info, Err: err}
		}(i, ref)
	}
	wg.Wait()
	return results
}

// Ensure Client satisfies Source.
var _ Source = (*Client)(nil)
