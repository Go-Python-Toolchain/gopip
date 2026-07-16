// Package requirement parses Python dependency requirements as defined by
// PEP 508, including names, extras, version specifiers, direct URL references,
// and environment markers. It also reads requirements files in the pip format.
package requirement

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// nameRe matches a PEP 508 project name at the start of a string. It is written
// as a single greedy pattern rather than an alternation so that Go's
// leftmost-first matching does not stop at the first character.
var nameRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?`)

// canonicalRe collapses runs of dot, hyphen, and underscore for normalization.
var canonicalRe = regexp.MustCompile(`[-_.]+`)

// Requirement is a single parsed dependency requirement.
type Requirement struct {
	Name      string
	Extras    []string
	Specifier *version.SpecifierSet
	URL       string
	Marker    Marker
	raw       string
}

// Parse parses a PEP 508 requirement string.
func Parse(s string) (*Requirement, error) {
	orig := strings.TrimSpace(s)
	rest := orig

	var markerStr string
	if i := strings.Index(rest, ";"); i >= 0 {
		markerStr = strings.TrimSpace(rest[i+1:])
		rest = strings.TrimSpace(rest[:i])
	}

	name := nameRe.FindString(rest)
	if name == "" {
		return nil, fmt.Errorf("requirement %q has no valid project name", s)
	}
	r := &Requirement{Name: name, raw: orig}
	rest = strings.TrimSpace(rest[len(name):])

	if strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end < 0 {
			return nil, fmt.Errorf("requirement %q has an unclosed extras list", s)
		}
		for _, e := range strings.Split(rest[1:end], ",") {
			if e = strings.TrimSpace(e); e != "" {
				r.Extras = append(r.Extras, e)
			}
		}
		rest = strings.TrimSpace(rest[end+1:])
	}

	switch {
	case strings.HasPrefix(rest, "@"):
		r.URL = strings.TrimSpace(rest[1:])
		if r.URL == "" {
			return nil, fmt.Errorf("requirement %q has an empty URL", s)
		}
	case rest != "":
		specStr := rest
		if strings.HasPrefix(specStr, "(") && strings.HasSuffix(specStr, ")") {
			specStr = specStr[1 : len(specStr)-1]
		}
		ss, err := version.ParseSpecifierSet(specStr)
		if err != nil {
			return nil, fmt.Errorf("requirement %q has an invalid version specifier: %w", s, err)
		}
		r.Specifier = ss
	}

	if r.Specifier == nil {
		r.Specifier, _ = version.ParseSpecifierSet("")
	}

	if markerStr != "" {
		m, err := ParseMarker(markerStr)
		if err != nil {
			return nil, fmt.Errorf("requirement %q has an invalid marker: %w", s, err)
		}
		r.Marker = m
	}

	return r, nil
}

// CanonicalName returns the normalized project name per PEP 503: lowercase with
// runs of dot, hyphen, and underscore collapsed to a single hyphen.
func (r *Requirement) CanonicalName() string {
	return CanonicalizeName(r.Name)
}

// CanonicalizeName normalizes a project name per PEP 503.
func CanonicalizeName(name string) string {
	return strings.ToLower(canonicalRe.ReplaceAllString(name, "-"))
}

// AppliesTo reports whether this requirement is active in the given environment.
// A requirement with no marker always applies. When the requirement's marker
// mentions an extra, set env["extra"] to the extra being resolved.
func (r *Requirement) AppliesTo(env Environment) bool {
	if r.Marker == nil {
		return true
	}
	return r.Marker.Evaluate(env)
}

// String reconstructs the requirement in a normalized form.
func (r *Requirement) String() string {
	var b strings.Builder
	b.WriteString(r.Name)
	if len(r.Extras) > 0 {
		b.WriteString("[")
		b.WriteString(strings.Join(r.Extras, ","))
		b.WriteString("]")
	}
	if r.URL != "" {
		b.WriteString(" @ ")
		b.WriteString(r.URL)
	} else if r.Specifier != nil {
		if spec := r.Specifier.String(); spec != "" {
			b.WriteString(spec)
		}
	}
	if r.Marker != nil {
		b.WriteString("; ")
		b.WriteString(r.Marker.String())
	}
	return b.String()
}
