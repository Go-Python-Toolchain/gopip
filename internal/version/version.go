// Package version implements Python version handling as defined by PEP 440:
// parsing, normalization, ordering, and version specifier sets. The rules here
// match the behavior of the reference packaging library so that gopip resolves
// the same versions pip would.
package version

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// versionPattern is the PEP 440 grammar, case-insensitive and anchored. It is
// the same shape used by the packaging library.
const versionPattern = `^\s*v?` +
	`(?:` +
	`(?:(?P<epoch>[0-9]+)!)?` +
	`(?P<release>[0-9]+(?:\.[0-9]+)*)` +
	`(?P<pre>[-_\.]?(?P<pre_l>alpha|a|beta|b|preview|pre|c|rc)[-_\.]?(?P<pre_n>[0-9]+)?)?` +
	`(?P<post>(?:-(?P<post_n1>[0-9]+))|(?:[-_\.]?(?P<post_l>post|rev|r)[-_\.]?(?P<post_n2>[0-9]+)?))?` +
	`(?P<dev>[-_\.]?(?P<dev_l>dev)[-_\.]?(?P<dev_n>[0-9]+)?)?` +
	`)` +
	`(?:\+(?P<local>[a-z0-9]+(?:[-_\.][a-z0-9]+)*))?` +
	`\s*$`

var versionRe = regexp.MustCompile(`(?i)` + versionPattern)

// preRelease is a pre-release marker such as a1, b2, or rc1.
type preRelease struct {
	label string // normalized to one of a, b, rc
	n     int
}

// localSegment is one dot-separated piece of a local version. It is either
// numeric or a lowercase string.
type localSegment struct {
	isNum bool
	num   int
	str   string
}

// Version is a parsed PEP 440 version.
type Version struct {
	epoch   int
	release []int
	pre     *preRelease
	post    *int
	dev     *int
	local   []localSegment
	orig    string
}

// Parse parses a PEP 440 version string, normalizing as it goes.
func Parse(s string) (*Version, error) {
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return nil, fmt.Errorf("invalid version: %q", s)
	}
	names := versionRe.SubexpNames()
	g := func(name string) string {
		for i, n := range names {
			if n == name {
				return m[i]
			}
		}
		return ""
	}

	v := &Version{orig: strings.TrimSpace(s)}

	if e := g("epoch"); e != "" {
		v.epoch, _ = strconv.Atoi(e)
	}

	for _, part := range strings.Split(g("release"), ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid release segment %q in %q", part, s)
		}
		v.release = append(v.release, n)
	}

	if label := g("pre_l"); label != "" {
		v.pre = &preRelease{label: normalizePreLabel(label), n: atoiOr(g("pre_n"), 0)}
	}

	if n1 := g("post_n1"); n1 != "" {
		n := atoiOr(n1, 0)
		v.post = &n
	} else if g("post_l") != "" {
		n := atoiOr(g("post_n2"), 0)
		v.post = &n
	}

	if g("dev_l") != "" {
		n := atoiOr(g("dev_n"), 0)
		v.dev = &n
	}

	if local := g("local"); local != "" {
		for _, seg := range strings.FieldsFunc(strings.ToLower(local), func(r rune) bool {
			return r == '-' || r == '_' || r == '.'
		}) {
			if n, err := strconv.Atoi(seg); err == nil {
				v.local = append(v.local, localSegment{isNum: true, num: n})
			} else {
				v.local = append(v.local, localSegment{str: seg})
			}
		}
	}

	return v, nil
}

// MustParse parses a version and panics on failure. It is for tests and
// constants known to be valid.
func MustParse(s string) *Version {
	v, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

func normalizePreLabel(label string) string {
	switch strings.ToLower(label) {
	case "alpha", "a":
		return "a"
	case "beta", "b":
		return "b"
	case "c", "pre", "preview", "rc":
		return "rc"
	}
	return strings.ToLower(label)
}

func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// IsPrerelease reports whether the version is an alpha, beta, rc, or dev release.
func (v *Version) IsPrerelease() bool {
	return v.pre != nil || v.dev != nil
}

// IsPostrelease reports whether the version has a post segment.
func (v *Version) IsPostrelease() bool {
	return v.post != nil
}

// HasLocal reports whether the version carries a local segment.
func (v *Version) HasLocal() bool {
	return len(v.local) > 0
}

// String returns the normalized PEP 440 form of the version.
func (v *Version) String() string {
	var b strings.Builder
	if v.epoch != 0 {
		fmt.Fprintf(&b, "%d!", v.epoch)
	}
	parts := make([]string, len(v.release))
	for i, r := range v.release {
		parts[i] = strconv.Itoa(r)
	}
	b.WriteString(strings.Join(parts, "."))
	if v.pre != nil {
		fmt.Fprintf(&b, "%s%d", v.pre.label, v.pre.n)
	}
	if v.post != nil {
		fmt.Fprintf(&b, ".post%d", *v.post)
	}
	if v.dev != nil {
		fmt.Fprintf(&b, ".dev%d", *v.dev)
	}
	if len(v.local) > 0 {
		b.WriteString("+")
		segs := make([]string, len(v.local))
		for i, s := range v.local {
			if s.isNum {
				segs[i] = strconv.Itoa(s.num)
			} else {
				segs[i] = s.str
			}
		}
		b.WriteString(strings.Join(segs, "."))
	}
	return b.String()
}

// Public returns the version without its local segment.
func (v *Version) Public() *Version {
	clone := *v
	clone.local = nil
	return &clone
}

// BaseVersion returns the epoch and release only, dropping pre, post, dev, and
// local segments.
func (v *Version) BaseVersion() *Version {
	return &Version{epoch: v.epoch, release: append([]int(nil), v.release...)}
}

// Compare returns -1, 0, or 1 as a is less than, equal to, or greater than b,
// following the PEP 440 ordering rules.
func Compare(a, b *Version) int {
	if c := cmpInt(a.epoch, b.epoch); c != 0 {
		return c
	}
	if c := cmpRelease(a.release, b.release); c != 0 {
		return c
	}
	if c := cmpPre(a, b); c != 0 {
		return c
	}
	if c := cmpPost(a.post, b.post); c != 0 {
		return c
	}
	if c := cmpDev(a.dev, b.dev); c != 0 {
		return c
	}
	return cmpLocal(a.local, b.local)
}

// Equal reports whether two versions are equal under PEP 440 ordering.
func Equal(a, b *Version) bool { return Compare(a, b) == 0 }

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// cmpRelease compares release tuples after dropping trailing zeros, so that 1.0
// and 1.0.0 are equal.
func cmpRelease(a, b []int) int {
	a = trimTrailingZeros(a)
	b = trimTrailingZeros(b)
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := cmpInt(a[i], b[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(a), len(b))
}

func trimTrailingZeros(r []int) []int {
	end := len(r)
	for end > 0 && r[end-1] == 0 {
		end--
	}
	return r[:end]
}

// preCategory ranks the pre segment. A dev-only release sorts before a real
// pre-release, and a version with no pre-release sorts after one.
func preCategory(v *Version) (rank int, label string, n int) {
	switch {
	case v.pre == nil && v.post == nil && v.dev != nil:
		return -1, "", 0
	case v.pre == nil:
		return 1, "", 0
	default:
		return 0, v.pre.label, v.pre.n
	}
}

func cmpPre(a, b *Version) int {
	ra, la, na := preCategory(a)
	rb, lb, nb := preCategory(b)
	if c := cmpInt(ra, rb); c != 0 {
		return c
	}
	if ra != 0 {
		return 0
	}
	if la != lb {
		if la < lb {
			return -1
		}
		return 1
	}
	return cmpInt(na, nb)
}

// cmpPost orders versions without a post segment before those with one.
func cmpPost(a, b *int) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return cmpInt(*a, *b)
}

// cmpDev orders versions without a dev segment after those with one.
func cmpDev(a, b *int) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	return cmpInt(*a, *b)
}

// cmpLocal orders versions without a local segment before those with one, and
// compares local segments per PEP 440: string segments sort before numeric
// ones, strings compare lexically, numbers compare numerically, and a shorter
// matching prefix sorts first.
func cmpLocal(a, b []localSegment) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return -1
	case len(b) == 0:
		return 1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := cmpLocalSegment(a[i], b[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(a), len(b))
}

func cmpLocalSegment(a, b localSegment) int {
	// Numeric segments sort after string segments.
	if a.isNum != b.isNum {
		if a.isNum {
			return 1
		}
		return -1
	}
	if a.isNum {
		return cmpInt(a.num, b.num)
	}
	if a.str < b.str {
		return -1
	}
	if a.str > b.str {
		return 1
	}
	return 0
}
