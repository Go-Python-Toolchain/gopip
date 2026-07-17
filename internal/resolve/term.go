package resolve

import (
	"sort"
	"strings"
)

// A term constrains one package to a set of allowed versions. Version sets are
// finite because the universe of a package is the set of versions the index
// offers, so the constraint algebra reduces to set operations. A term is stored
// in positive form: allowed is exactly the set of version strings that satisfy
// the term, so a negative constraint is represented by its complement.
type term struct {
	pkg     string
	allowed versionSet
}

// versionSet is a set of normalized version strings.
type versionSet map[string]bool

func newVersionSet(versions ...string) versionSet {
	s := make(versionSet, len(versions))
	for _, v := range versions {
		s[v] = true
	}
	return s
}

func (s versionSet) clone() versionSet {
	out := make(versionSet, len(s))
	for v := range s {
		out[v] = true
	}
	return out
}

func (s versionSet) intersect(other versionSet) versionSet {
	out := versionSet{}
	small, large := s, other
	if len(large) < len(small) {
		small, large = large, small
	}
	for v := range small {
		if large[v] {
			out[v] = true
		}
	}
	return out
}

func (s versionSet) union(other versionSet) versionSet {
	out := s.clone()
	for v := range other {
		out[v] = true
	}
	return out
}

// complement returns the versions of universe not in s.
func (s versionSet) complement(universe versionSet) versionSet {
	out := versionSet{}
	for v := range universe {
		if !s[v] {
			out[v] = true
		}
	}
	return out
}

func (s versionSet) isEmpty() bool { return len(s) == 0 }

// subsetOf reports whether every element of s is in other.
func (s versionSet) subsetOf(other versionSet) bool {
	for v := range s {
		if !other[v] {
			return false
		}
	}
	return true
}

// disjoint reports whether s and other share no elements.
func (s versionSet) disjoint(other versionSet) bool {
	small, large := s, other
	if len(large) < len(small) {
		small, large = large, small
	}
	for v := range small {
		if large[v] {
			return false
		}
	}
	return true
}

func (s versionSet) equal(other versionSet) bool {
	return len(s) == len(other) && s.subsetOf(other)
}

// sorted returns the set's members as a stable, sorted slice for deterministic
// output and readable messages.
func (s versionSet) sorted() []string {
	out := make([]string, 0, len(s))
	for v := range s {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func (t term) String() string {
	return t.pkg + " {" + strings.Join(t.allowed.sorted(), ", ") + "}"
}
