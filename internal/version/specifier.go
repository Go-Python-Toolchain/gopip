package version

import (
	"fmt"
	"strings"
)

// operators are matched longest first so that === is not read as ==.
var operators = []string{"===", "~=", "==", "!=", "<=", ">=", "<", ">"}

// Specifier is a single version constraint such as >=1.2 or ==1.4.*.
type Specifier struct {
	op        string
	raw       string   // the version text as written
	ver       *Version // parsed version, for the comparison operators
	prefix    *Version // release prefix, for wildcard and compatible matches
	wildcard  bool     // true for ==X.* or !=X.*
	arbitrary bool     // true for ===
}

// ParseSpecifier parses one specifier such as ">=1.2.3".
func ParseSpecifier(s string) (*Specifier, error) {
	s = strings.TrimSpace(s)
	for _, op := range operators {
		if !strings.HasPrefix(s, op) {
			continue
		}
		raw := strings.TrimSpace(s[len(op):])
		if raw == "" {
			return nil, fmt.Errorf("specifier %q is missing a version", s)
		}
		spec := &Specifier{op: op, raw: raw}

		if op == "===" {
			spec.arbitrary = true
			return spec, nil
		}

		if strings.HasSuffix(raw, ".*") {
			if op != "==" && op != "!=" {
				return nil, fmt.Errorf("operator %q does not support wildcards: %q", op, s)
			}
			base := strings.TrimSuffix(raw, ".*")
			pv, err := Parse(base)
			if err != nil {
				return nil, fmt.Errorf("invalid wildcard specifier %q: %w", s, err)
			}
			spec.wildcard = true
			spec.prefix = pv
			return spec, nil
		}

		pv, err := Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid specifier %q: %w", s, err)
		}
		spec.ver = pv

		if op == "~=" {
			if len(pv.release) < 2 {
				return nil, fmt.Errorf("compatible specifier %q needs at least two release segments", s)
			}
			spec.prefix = &Version{epoch: pv.epoch, release: pv.release[:len(pv.release)-1]}
		}
		return spec, nil
	}
	return nil, fmt.Errorf("specifier %q has no valid operator", s)
}

// Matches reports whether the version satisfies this single specifier.
func (s *Specifier) Matches(v *Version) bool {
	switch s.op {
	case "===":
		return strings.TrimSpace(v.orig) == s.raw || v.String() == s.raw
	case "==":
		if s.wildcard {
			return matchWildcard(v, s.prefix)
		}
		return matchEqual(v, s.ver)
	case "!=":
		if s.wildcard {
			return !matchWildcard(v, s.prefix)
		}
		return !matchEqual(v, s.ver)
	case "<=":
		return Compare(v.Public(), s.ver) <= 0
	case ">=":
		return Compare(v.Public(), s.ver) >= 0
	case "<":
		return matchLess(v, s.ver)
	case ">":
		return matchGreater(v, s.ver)
	case "~=":
		return Compare(v.Public(), s.ver) >= 0 && matchWildcard(v, s.prefix)
	}
	return false
}

// isPrereleaseSpec reports whether this specifier references a pre-release,
// which signals that the user is opting into pre-releases.
func (s *Specifier) isPrereleaseSpec() bool {
	if s.ver != nil && s.ver.IsPrerelease() {
		return true
	}
	if s.prefix != nil && s.prefix.IsPrerelease() {
		return true
	}
	return false
}

// String renders the specifier.
func (s *Specifier) String() string { return s.op + s.raw }

func matchEqual(v, spec *Version) bool {
	if spec.HasLocal() {
		return Equal(v, spec)
	}
	return Equal(v.Public(), spec)
}

// matchWildcard matches a version against a release prefix, as in ==1.4.*.
func matchWildcard(v, prefix *Version) bool {
	if v.epoch != prefix.epoch {
		return false
	}
	rel := append([]int(nil), v.release...)
	for len(rel) < len(prefix.release) {
		rel = append(rel, 0)
	}
	for i := range prefix.release {
		if rel[i] != prefix.release[i] {
			return false
		}
	}
	return true
}

func matchLess(v, spec *Version) bool {
	vp := v.Public()
	if Compare(vp, spec) >= 0 {
		return false
	}
	// A pre-release of the same version is not below a final spec.
	if !spec.IsPrerelease() && vp.IsPrerelease() {
		if Equal(vp.BaseVersion(), spec.BaseVersion()) {
			return false
		}
	}
	return true
}

func matchGreater(v, spec *Version) bool {
	vp := v.Public()
	if Compare(vp, spec) <= 0 {
		return false
	}
	// A post-release of the same version is not above a non-post spec.
	if !spec.IsPostrelease() && vp.IsPostrelease() {
		if Equal(vp.BaseVersion(), spec.BaseVersion()) {
			return false
		}
	}
	// A local version of the same base is not above the spec.
	if v.HasLocal() && Equal(vp.BaseVersion(), spec.BaseVersion()) {
		return false
	}
	return true
}

// SpecifierSet is a comma-separated list of specifiers, all of which must match.
type SpecifierSet struct {
	specs []*Specifier
}

// ParseSpecifierSet parses a comma-separated set such as ">=1.0,<2.0".
func ParseSpecifierSet(s string) (*SpecifierSet, error) {
	set := &SpecifierSet{}
	s = strings.TrimSpace(s)
	if s == "" {
		return set, nil
	}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		spec, err := ParseSpecifier(part)
		if err != nil {
			return nil, err
		}
		set.specs = append(set.specs, spec)
	}
	return set, nil
}

// prereleasesImplied reports whether any member specifier opts into pre-releases.
func (set *SpecifierSet) prereleasesImplied() bool {
	for _, s := range set.specs {
		if s.isPrereleaseSpec() {
			return true
		}
	}
	return false
}

// Matches reports whether the version satisfies every specifier, excluding
// pre-releases unless the set opts into them.
func (set *SpecifierSet) Matches(v *Version) bool {
	return set.Contains(v, false)
}

// Contains reports whether the version satisfies every specifier. Pre-release
// versions are rejected unless allowPre is true or the set references a
// pre-release.
func (set *SpecifierSet) Contains(v *Version, allowPre bool) bool {
	if v.IsPrerelease() && !allowPre && !set.prereleasesImplied() {
		return false
	}
	for _, s := range set.specs {
		if !s.Matches(v) {
			return false
		}
	}
	return true
}

// String renders the specifier set.
func (set *SpecifierSet) String() string {
	parts := make([]string, len(set.specs))
	for i, s := range set.specs {
		parts[i] = s.String()
	}
	return strings.Join(parts, ",")
}
