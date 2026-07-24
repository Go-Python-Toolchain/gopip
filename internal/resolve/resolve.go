// Package resolve chooses a consistent set of package versions for a set of
// requirements. It is a version-solving resolver in the PubGrub style: it
// reasons over incompatibilities (sets of package terms that cannot all hold),
// propagates their consequences as units, and on a conflict derives a new
// incompatibility and backjumps. Because the universe of versions for a package
// is finite, constraints are represented as finite version sets.
package resolve

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/Go-Python-Toolchain/gopip/internal/pypi"
	"github.com/Go-Python-Toolchain/gopip/internal/requirement"
	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

const (
	rootPkg     = "$root"
	rootVersion = "0"
	// absentVersion is a sentinel in every package's universe standing for "not
	// selected". It keeps the complement of an any-version dependency non-empty,
	// so such a dependency still forces the package into the solution, matching
	// PubGrub's unbounded version space.
	absentVersion = "\x00absent"
	// defaultConcurrency bounds how many index requests are in flight at once.
	// Enough to hide the latency of a wide dependency layer, not so many that a
	// resolve looks like a burst of traffic to the index.
	defaultConcurrency = 16
)

// incompatibility is a set of terms that cannot all be satisfied at once. It has
// at most one term per package.
type incompatibility struct {
	terms  []term
	kind   string // root, dependency, no-versions, unsupported-python, derived
	detail string
}

func (ic *incompatibility) String() string {
	parts := make([]string, len(ic.terms))
	for i, t := range ic.terms {
		parts[i] = t.String()
	}
	return "{" + join(parts, ", ") + "}"
}

// Solution is the result of resolution: the chosen version of each package, the
// canonical names required directly by the project, and the dependency edges
// between resolved packages. Edges and Roots drive the lockfile and the explain
// tree.
type Solution struct {
	Packages map[string]*version.Version
	Roots    []string
	Edges    map[string][]string
}

// ResolutionError reports that no set of versions satisfies the requirements.
type ResolutionError struct {
	Cause *incompatibility
}

func (e *ResolutionError) Error() string {
	if e.Cause != nil && e.Cause.detail != "" {
		return "version resolution failed: " + e.Cause.detail
	}
	return "version resolution failed: the requirements are unsatisfiable"
}

// Resolver holds resolution state for one run.
type Resolver struct {
	source    pypi.Source
	env       requirement.Environment
	pyVersion *version.Version

	universes    map[string]versionSet
	versionsAsc  map[string][]*version.Version
	byPackage    map[string][]*incompatibility
	ps           *partialSolution
	depsAdded    map[string]bool
	displayName  map[string]string
	releaseCache map[string]*pypi.ReleaseInfo
	concurrency  int
}

// Option configures a Resolver.
type Option func(*Resolver)

// WithEnvironment sets the marker environment used to evaluate dependency
// markers. The target Python version should be present for requires-python
// checks.
func WithEnvironment(env requirement.Environment) Option {
	return func(r *Resolver) { r.env = env }
}

// WithPythonVersion sets the target Python version for requires-python filtering.
func WithPythonVersion(v *version.Version) Option {
	return func(r *Resolver) { r.pyVersion = v }
}

// WithConcurrency sets how many index requests may be in flight at once. It
// affects only how quickly a resolution is reached, never what it resolves to.
func WithConcurrency(n int) Option {
	return func(r *Resolver) {
		if n > 0 {
			r.concurrency = n
		}
	}
}

// New creates a resolver over the given source.
func New(source pypi.Source, opts ...Option) *Resolver {
	r := &Resolver{
		source:       source,
		env:          requirement.CurrentEnvironment("3.12"),
		universes:    map[string]versionSet{},
		versionsAsc:  map[string][]*version.Version{},
		byPackage:    map[string][]*incompatibility{},
		ps:           newPartialSolution(),
		depsAdded:    map[string]bool{},
		displayName:  map[string]string{},
		releaseCache: map[string]*pypi.ReleaseInfo{},
		concurrency:  defaultConcurrency,
	}
	for _, o := range opts {
		o(r)
	}
	if r.pyVersion == nil {
		if pv, ok := r.env["python_version"]; ok {
			r.pyVersion, _ = version.Parse(pv)
		}
	}
	return r
}

// Resolve finds versions for the given root requirements.
func (r *Resolver) Resolve(ctx context.Context, roots []*requirement.Requirement) (*Solution, error) {
	r.universes[rootPkg] = newVersionSet(rootVersion)

	// The roots are known up front and none of them depends on another, so their
	// metadata is fetched together rather than one at a time.
	var applicable []*requirement.Requirement
	for _, req := range roots {
		if req.AppliesTo(r.env) {
			applicable = append(applicable, req)
		}
	}
	if err := r.prefetch(ctx, applicable); err != nil {
		return nil, err
	}

	var rootNames []string
	for _, req := range roots {
		if !req.AppliesTo(r.env) {
			continue
		}
		rootNames = append(rootNames, requirement.CanonicalizeName(req.Name))
		ic, err := r.dependencyIncompatibility(ctx, term{pkg: rootPkg, allowed: newVersionSet(rootVersion)}, "the root project", req)
		if err != nil {
			return nil, err
		}
		r.addIncompatibility(ic)
	}

	r.ps.decide(rootPkg, rootVersion, r.universes[rootPkg])

	next := rootPkg
	for {
		if err := r.unitPropagation(ctx, next); err != nil {
			return nil, err
		}
		chosen, err := r.decide(ctx)
		if err != nil {
			return nil, err
		}
		if chosen == "" {
			break
		}
		next = chosen
	}

	sol := &Solution{
		Packages: map[string]*version.Version{},
		Edges:    map[string][]string{},
	}
	for pkg, verStr := range r.ps.decisions {
		if pkg == rootPkg {
			continue
		}
		v, err := version.Parse(verStr)
		if err != nil {
			return nil, err
		}
		sol.Packages[pkg] = v
	}

	// Record the roots that made it into the solution, sorted for stability.
	for _, name := range rootNames {
		if _, ok := sol.Packages[name]; ok {
			sol.Roots = append(sol.Roots, name)
		}
	}
	sol.Roots = sortedUnique(sol.Roots)

	// Build dependency edges between resolved packages so the lockfile and the
	// explain tree can show the graph.
	if err := r.buildEdges(ctx, sol); err != nil {
		return nil, err
	}
	return sol, nil
}

// Verify checks that a solution actually satisfies the requirements: every
// applicable root and every applicable dependency of every resolved package is
// present at a version that meets its constraint. It returns an error describing
// the first violation, or nil when the solution is valid.
func Verify(ctx context.Context, source pypi.Source, env requirement.Environment, roots []*requirement.Requirement, sol *Solution) error {
	for _, req := range roots {
		if !req.AppliesTo(env) {
			continue
		}
		name := requirement.CanonicalizeName(req.Name)
		v, ok := sol.Packages[name]
		if !ok {
			return fmt.Errorf("root %s is missing from the solution", req.Name)
		}
		if req.Specifier != nil && !req.Specifier.Matches(v) {
			return fmt.Errorf("root %s at %s does not satisfy %s", req.Name, v, req.Specifier)
		}
	}

	for name, v := range sol.Packages {
		info, err := source.Release(ctx, name, v)
		if err != nil {
			return fmt.Errorf("verifying %s %s: %w", name, v, err)
		}
		for _, dep := range info.RequiresDist {
			if !dep.AppliesTo(env) {
				continue
			}
			depName := requirement.CanonicalizeName(dep.Name)
			dv, ok := sol.Packages[depName]
			if !ok {
				return fmt.Errorf("%s %s depends on %s, which is missing from the solution", name, v, dep.Name)
			}
			if dep.Specifier != nil && !dep.Specifier.Matches(dv) {
				return fmt.Errorf("%s %s depends on %s%s, but the solution has %s", name, v, dep.Name, dep.Specifier, dv)
			}
		}
	}
	return nil
}

// buildEdges fills sol.Edges with, for each resolved package, the resolved
// packages it depends on under the current environment.
func (r *Resolver) buildEdges(ctx context.Context, sol *Solution) error {
	for pkg, v := range sol.Packages {
		info, err := r.release(ctx, pkg, v)
		if err != nil {
			return err
		}
		var deps []string
		for _, dep := range info.RequiresDist {
			if !dep.AppliesTo(r.env) {
				continue
			}
			depName := requirement.CanonicalizeName(dep.Name)
			if _, ok := sol.Packages[depName]; ok {
				deps = append(deps, depName)
			}
		}
		sol.Edges[pkg] = sortedUnique(deps)
	}
	return nil
}

func sortedUnique(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func (r *Resolver) addIncompatibility(ic *incompatibility) {
	seen := map[string]bool{}
	for _, t := range ic.terms {
		if seen[t.pkg] {
			continue
		}
		seen[t.pkg] = true
		r.byPackage[t.pkg] = append(r.byPackage[t.pkg], ic)
	}
}

// unitPropagation derives consequences of incompatibilities that mention the
// changed package, cascading to other packages, until nothing new is derived or
// a conflict is resolved.
func (r *Resolver) unitPropagation(ctx context.Context, changed string) error {
	queue := []string{changed}
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]

		ics := r.byPackage[pkg]
		for i := len(ics) - 1; i >= 0; i-- {
			ic := ics[i]
			rel, unit := r.relate(ic)
			switch rel {
			case relSatisfied:
				root, err := r.conflictResolution(ic)
				if err != nil {
					return err
				}
				_, unit2 := r.relate(root)
				r.ps.derive(r.negate(unit2), root, r.universes[unit2.pkg])
				queue = []string{unit2.pkg}
				ics = r.byPackage[pkg]
				i = len(ics)
			case relAlmostSatisfied:
				r.ps.derive(r.negate(unit), ic, r.universes[unit.pkg])
				queue = append(queue, unit.pkg)
			}
		}
	}
	return nil
}

type icRelation int

const (
	relInconclusive icRelation = iota
	relSatisfied
	relContradicted
	relAlmostSatisfied
)

// relate computes how an incompatibility stands against the partial solution.
func (r *Resolver) relate(ic *incompatibility) (icRelation, term) {
	inconclusive := 0
	var unit term
	for _, t := range ic.terms {
		u := r.universes[t.pkg]
		d := r.ps.derivedFor(t.pkg, u)
		switch {
		case d.subsetOf(t.allowed):
			// this term is satisfied
		case d.disjoint(t.allowed):
			return relContradicted, term{}
		default:
			inconclusive++
			unit = t
		}
	}
	switch inconclusive {
	case 0:
		return relSatisfied, term{}
	case 1:
		return relAlmostSatisfied, unit
	default:
		return relInconclusive, term{}
	}
}

func (r *Resolver) negate(t term) term {
	return term{pkg: t.pkg, allowed: t.allowed.complement(r.universes[t.pkg])}
}

// conflictResolution turns a satisfied incompatibility into a new incompatibility
// that is almost satisfied at an earlier decision level, backjumping the partial
// solution. It returns an error when the conflict traces back to the root, which
// means the requirements are unsatisfiable.
func (r *Resolver) conflictResolution(incompat *incompatibility) (*incompatibility, error) {
	newlyDerived := false
	for {
		if len(incompat.terms) == 0 {
			return nil, &ResolutionError{Cause: incompat}
		}
		if len(incompat.terms) == 1 && incompat.terms[0].pkg == rootPkg {
			return nil, &ResolutionError{Cause: incompat}
		}

		var mostRecentTerm term
		var mostRecentSat assignment
		mostRecentIdx := -2
		previousLevel := 1
		found := false

		for _, t := range incompat.terms {
			sat, idx := r.ps.satisfier(t, r.universes)
			if !found || idx > mostRecentIdx {
				if found {
					previousLevel = max(previousLevel, mostRecentSat.level)
				}
				mostRecentTerm = t
				mostRecentSat = sat
				mostRecentIdx = idx
				found = true
			} else {
				previousLevel = max(previousLevel, sat.level)
			}
		}

		if mostRecentSat.decision || previousLevel < mostRecentSat.level {
			if newlyDerived {
				r.addIncompatibility(incompat)
			}
			r.ps.backtrack(previousLevel, r.universes)
			return incompat, nil
		}

		// Resolve: merge this incompatibility with the cause of the satisfier,
		// dropping the terms about the satisfier's package, and add a difference
		// term when the satisfier only partially covers the term.
		prior := mostRecentSat.cause
		merged := map[string]term{}
		addTerm := func(t term) {
			if existing, ok := merged[t.pkg]; ok {
				merged[t.pkg] = term{pkg: t.pkg, allowed: existing.allowed.intersect(t.allowed)}
			} else {
				merged[t.pkg] = t
			}
		}
		for _, t := range incompat.terms {
			if t.pkg != mostRecentTerm.pkg {
				addTerm(t)
			}
		}
		for _, t := range prior.terms {
			if t.pkg != mostRecentSat.t.pkg {
				addTerm(t)
			}
		}
		if !mostRecentSat.t.allowed.subsetOf(mostRecentTerm.allowed) {
			diff := mostRecentTerm.allowed.complement(mostRecentSat.t.allowed)
			addTerm(term{pkg: mostRecentTerm.pkg, allowed: diff})
		}

		terms := make([]term, 0, len(merged))
		for _, t := range merged {
			terms = append(terms, t)
		}
		incompat = &incompatibility{terms: terms, kind: "derived", detail: prior.detail}
		newlyDerived = true
	}
}

// decide chooses the next package to fix to a concrete version. It returns the
// package name, or the empty string when every required package is decided.
func (r *Resolver) decide(ctx context.Context) (string, error) {
	// Candidate packages are those with a narrowed derived set but no decision.
	candidates := make([]string, 0)
	for pkg, allowed := range r.ps.derived {
		if pkg == rootPkg {
			continue
		}
		if _, decided := r.ps.decisions[pkg]; decided {
			continue
		}
		// A package is only decided when it is forced to be present, meaning the
		// absent sentinel has been ruled out of its allowed set.
		if allowed[absentVersion] || allowed.isEmpty() {
			continue
		}
		candidates = append(candidates, pkg)
	}
	if len(candidates) == 0 {
		return "", nil
	}
	sort.Strings(candidates)

	// Prefer the package with the fewest allowed versions, breaking ties by name,
	// which keeps resolution deterministic and tends to fail fast.
	pick := candidates[0]
	pickCount := len(r.ps.derived[pick])
	for _, pkg := range candidates[1:] {
		if n := len(r.ps.derived[pkg]); n < pickCount {
			pick, pickCount = pkg, n
		}
	}

	allowed := r.ps.derived[pick]
	if allowed.isEmpty() {
		// No version works; the incompatibility that emptied it drives the conflict
		// on the next propagation, so just return it to re-run propagation.
		return pick, nil
	}

	chosen := r.bestVersion(pick, allowed)
	if chosen == nil {
		r.addIncompatibility(&incompatibility{
			terms:  []term{{pkg: pick, allowed: allowed.clone()}},
			kind:   "no-versions",
			detail: fmt.Sprintf("no version of %s matches the constraints", r.name(pick)),
		})
		return pick, nil
	}

	if err := r.addDependencyIncompatibilities(ctx, pick, chosen); err != nil {
		return "", err
	}
	r.ps.decide(pick, chosen.String(), r.universes[pick])
	return pick, nil
}

// bestVersion returns the highest allowed version of a package that also
// supports the target Python, or nil if none qualifies.
func (r *Resolver) bestVersion(pkg string, allowed versionSet) *version.Version {
	versions := r.versionsAsc[pkg]
	for i := len(versions) - 1; i >= 0; i-- {
		v := versions[i]
		if !allowed[v.String()] {
			continue
		}
		ok, err := r.supportsPython(context.Background(), pkg, v)
		if err != nil || !ok {
			continue
		}
		return v
	}
	return nil
}

// supportsPython reports whether a release's requires-python admits the target.
func (r *Resolver) supportsPython(ctx context.Context, pkg string, v *version.Version) (bool, error) {
	if r.pyVersion == nil {
		return true, nil
	}
	info, err := r.release(ctx, pkg, v)
	if err != nil {
		return false, err
	}
	if info.RequiresPython == "" {
		return true, nil
	}
	spec, err := version.ParseSpecifierSet(info.RequiresPython)
	if err != nil {
		return true, nil // tolerate a malformed constraint
	}
	return spec.Contains(r.pyVersion, true), nil
}

// addDependencyIncompatibilities registers the dependencies of a chosen release.
func (r *Resolver) addDependencyIncompatibilities(ctx context.Context, pkg string, v *version.Version) error {
	key := pkg + "@" + v.String()
	if r.depsAdded[key] {
		return nil
	}
	r.depsAdded[key] = true

	info, err := r.release(ctx, pkg, v)
	if err != nil {
		return err
	}
	// Everything this release depends on is needed regardless of what the
	// resolver decides next, so it is fetched together before the loop below
	// starts asking for it one dependency at a time.
	var deps []*requirement.Requirement
	for _, dep := range info.RequiresDist {
		if dep.AppliesTo(r.envForExtras(nil)) {
			deps = append(deps, dep)
		}
	}
	if err := r.prefetch(ctx, deps); err != nil {
		return err
	}

	self := term{pkg: pkg, allowed: newVersionSet(v.String())}
	for _, dep := range deps {
		ic, err := r.dependencyIncompatibility(ctx, self, r.name(pkg)+" "+v.String(), dep)
		if err != nil {
			return err
		}
		r.addIncompatibility(ic)
	}
	return nil
}

func (r *Resolver) envForExtras(extras []string) requirement.Environment {
	if len(extras) == 0 {
		return r.env
	}
	env := requirement.Environment{}
	for k, v := range r.env {
		env[k] = v
	}
	env["extra"] = extras[0]
	return env
}

// dependencyIncompatibility builds the incompatibility "depender depends on dep",
// fetching the dependency's universe so its allowed set can be computed.
func (r *Resolver) dependencyIncompatibility(ctx context.Context, depender term, dependerName string, dep *requirement.Requirement) (*incompatibility, error) {
	depPkg := requirement.CanonicalizeName(dep.Name)
	if _, err := r.ensureUniverse(ctx, depPkg, dep.Name); err != nil {
		return nil, err
	}
	allowed := r.allowedFor(depPkg, dep)
	detail := fmt.Sprintf("%s depends on %s%s", dependerName, dep.Name, specText(dep))
	if allowed.isEmpty() {
		// No version of the dependency satisfies the constraint, so the depender
		// itself cannot be selected. The incompatibility is just the depender.
		return &incompatibility{
			terms:  []term{depender},
			kind:   "no-versions",
			detail: detail + ", which has no matching version",
		}, nil
	}
	return &incompatibility{
		terms: []term{
			depender,
			{pkg: depPkg, allowed: allowed.complement(r.universes[depPkg])},
		},
		kind:   "dependency",
		detail: detail,
	}, nil
}

func specText(dep *requirement.Requirement) string {
	if dep.Specifier == nil {
		return ""
	}
	if s := dep.Specifier.String(); s != "" {
		return " " + s
	}
	return ""
}

// allowedFor returns the versions of a package that satisfy a requirement.
func (r *Resolver) allowedFor(pkg string, dep *requirement.Requirement) versionSet {
	allowed := versionSet{}
	for _, v := range r.versionsAsc[pkg] {
		if dep.Specifier == nil || dep.Specifier.Matches(v) {
			allowed[v.String()] = true
		}
	}
	return allowed
}

// ensureUniverse fetches and caches a package's available versions.
func (r *Resolver) ensureUniverse(ctx context.Context, pkg, display string) (versionSet, error) {
	if u, ok := r.universes[pkg]; ok {
		return u, nil
	}
	versions, err := r.source.Versions(ctx, display)
	if err != nil {
		if errors.Is(err, pypi.ErrNotFound) {
			r.setMissingUniverse(pkg, display)
			return r.universes[pkg], nil
		}
		return nil, err
	}
	return r.setUniverse(pkg, display, versions), nil
}

// setUniverse records a package's available versions as its search space.
func (r *Resolver) setUniverse(pkg, display string, versions []*version.Version) versionSet {
	r.displayName[pkg] = display
	sort.Slice(versions, func(i, j int) bool { return version.Compare(versions[i], versions[j]) < 0 })

	u := versionSet{absentVersion: true}
	for _, v := range versions {
		u[v.String()] = true
	}
	r.universes[pkg] = u
	r.versionsAsc[pkg] = versions
	return u
}

// setMissingUniverse records a package the index does not have, which leaves it
// with no selectable version at all.
func (r *Resolver) setMissingUniverse(pkg, display string) {
	r.displayName[pkg] = display
	r.universes[pkg] = versionSet{}
	r.versionsAsc[pkg] = nil
}

// prefetch fetches the metadata a set of requirements is about to need, in
// parallel. A resolve's wall-clock cost is almost entirely round-trips to the
// index, and the requests for a release's dependencies do not depend on each
// other, so making them one at a time turns a graph's width into latency.
//
// Fetching concurrently must not make resolution depend on which response
// arrives first. Nothing here decides anything: results are collected and then
// applied in a fixed order, so the resolver sees exactly the state it would have
// built fetching one at a time.
func (r *Resolver) prefetch(ctx context.Context, deps []*requirement.Requirement) error {
	type target struct{ pkg, display string }

	var targets []target
	seen := map[string]bool{}
	for _, dep := range deps {
		pkg := requirement.CanonicalizeName(dep.Name)
		if _, known := r.universes[pkg]; known || seen[pkg] {
			continue
		}
		seen[pkg] = true
		targets = append(targets, target{pkg: pkg, display: dep.Name})
	}
	if len(targets) < 2 {
		return nil // nothing to overlap
	}

	type fetched struct {
		versions []*version.Version
		err      error
	}
	results := make([]fetched, len(targets))
	r.inParallel(len(targets), func(i int) {
		results[i].versions, results[i].err = r.source.Versions(ctx, targets[i].display)
	})

	for i, t := range targets {
		switch {
		case results[i].err == nil:
			r.setUniverse(t.pkg, t.display, results[i].versions)
		case errors.Is(results[i].err, pypi.ErrNotFound):
			r.setMissingUniverse(t.pkg, t.display)
		default:
			return results[i].err
		}
	}

	r.prefetchReleases(ctx, deps)
	return nil
}

// prefetchReleases fetches, in parallel, the release metadata the resolver is
// most likely to ask for next: the highest version of each dependency that its
// constraint allows, which is the version the resolver tries first.
//
// A failure here is deliberately ignored. This is an optimization, and the real
// lookup will make the same request and report the failure properly, so a
// transient error while guessing ahead must never become a resolution error.
func (r *Resolver) prefetchReleases(ctx context.Context, deps []*requirement.Requirement) {
	type target struct {
		pkg string
		v   *version.Version
	}

	var targets []target
	seen := map[string]bool{}
	for _, dep := range deps {
		pkg := requirement.CanonicalizeName(dep.Name)
		allowed := r.allowedFor(pkg, dep)
		versions := r.versionsAsc[pkg]
		for i := len(versions) - 1; i >= 0; i-- {
			v := versions[i]
			if !allowed[v.String()] {
				continue
			}
			key := pkg + "@" + v.String()
			if _, cached := r.releaseCache[key]; !cached && !seen[key] {
				seen[key] = true
				targets = append(targets, target{pkg: pkg, v: v})
			}
			break
		}
	}
	if len(targets) < 2 {
		return
	}

	infos := make([]*pypi.ReleaseInfo, len(targets))
	r.inParallel(len(targets), func(i int) {
		info, err := r.source.Release(ctx, r.name(targets[i].pkg), targets[i].v)
		if err == nil {
			infos[i] = info
		}
	})

	for i, t := range targets {
		if infos[i] != nil {
			r.releaseCache[t.pkg+"@"+t.v.String()] = infos[i]
		}
	}
}

// inParallel runs work for each index below n, with at most concurrency of them
// in flight. The work function must not touch resolver state; callers collect
// into a preallocated slice and apply the results afterwards.
func (r *Resolver) inParallel(n int, work func(i int)) {
	sem := make(chan struct{}, r.concurrency)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			work(i)
		}(i)
	}
	wg.Wait()
}

func (r *Resolver) release(ctx context.Context, pkg string, v *version.Version) (*pypi.ReleaseInfo, error) {
	key := pkg + "@" + v.String()
	if info, ok := r.releaseCache[key]; ok {
		return info, nil
	}
	info, err := r.source.Release(ctx, r.name(pkg), v)
	if err != nil {
		return nil, err
	}
	r.releaseCache[key] = info
	return info, nil
}

func (r *Resolver) name(pkg string) string {
	if d, ok := r.displayName[pkg]; ok {
		return d
	}
	return pkg
}

func join(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}
