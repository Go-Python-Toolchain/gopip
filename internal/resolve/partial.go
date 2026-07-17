package resolve

// assignment is one entry in the partial solution. A decision fixes a package to
// a single version. A derivation narrows a package's allowed versions as a
// consequence of an incompatibility, recorded as cause.
type assignment struct {
	t        term
	level    int
	decision bool
	version  string           // set when decision is true
	cause    *incompatibility // set for derivations, nil for decisions
}

// partialSolution is the ordered list of assignments made so far, plus the
// derived constraint and decision for each package.
type partialSolution struct {
	assignments []assignment
	derived     map[string]versionSet // package -> current allowed set
	decisions   map[string]string     // package -> decided version
	level       int
}

func newPartialSolution() *partialSolution {
	return &partialSolution{
		derived:   map[string]versionSet{},
		decisions: map[string]string{},
		level:     0,
	}
}

// derivedFor returns the current allowed set for a package, defaulting to the
// package's whole universe when it has no assignments yet.
func (ps *partialSolution) derivedFor(pkg string, universe versionSet) versionSet {
	if d, ok := ps.derived[pkg]; ok {
		return d
	}
	return universe
}

// derive records a derivation of term t caused by ic.
func (ps *partialSolution) derive(t term, ic *incompatibility, universe versionSet) {
	cur := ps.derivedFor(t.pkg, universe)
	ps.derived[t.pkg] = cur.intersect(t.allowed)
	ps.assignments = append(ps.assignments, assignment{t: t, level: ps.level, cause: ic})
}

// decide records a decision to use a concrete version, opening a new level.
func (ps *partialSolution) decide(pkg, ver string, universe versionSet) {
	ps.level++
	ps.decisions[pkg] = ver
	t := term{pkg: pkg, allowed: newVersionSet(ver)}
	cur := ps.derivedFor(pkg, universe)
	ps.derived[pkg] = cur.intersect(t.allowed)
	ps.assignments = append(ps.assignments, assignment{t: t, level: ps.level, decision: true, version: ver})
}

// backtrack removes every assignment above the given level and rebuilds the
// derived constraints and decisions from what remains.
func (ps *partialSolution) backtrack(level int, universes map[string]versionSet) {
	kept := make([]assignment, 0, len(ps.assignments))
	for _, a := range ps.assignments {
		if a.level <= level {
			kept = append(kept, a)
		}
	}
	ps.assignments = kept
	ps.level = level
	ps.rebuild(universes)
}

func (ps *partialSolution) rebuild(universes map[string]versionSet) {
	ps.derived = map[string]versionSet{}
	ps.decisions = map[string]string{}
	for _, a := range ps.assignments {
		cur, ok := ps.derived[a.t.pkg]
		if !ok {
			cur = universes[a.t.pkg]
		}
		ps.derived[a.t.pkg] = cur.intersect(a.t.allowed)
		if a.decision {
			ps.decisions[a.t.pkg] = a.version
		}
	}
}

// satisfier returns the earliest assignment (and its index) after which the
// accumulated constraint for t's package is a subset of t.allowed, meaning t is
// satisfied. It assumes t is in fact satisfied by the partial solution.
func (ps *partialSolution) satisfier(t term, universes map[string]versionSet) (assignment, int) {
	acc := universes[t.pkg]
	// A term already covered by the whole universe is satisfied before any
	// assignment. Report it at level 0 so it never counts as the most recent.
	if acc.subsetOf(t.allowed) {
		return assignment{t: t, level: 0}, -1
	}
	for i, a := range ps.assignments {
		if a.t.pkg != t.pkg {
			continue
		}
		acc = acc.intersect(a.t.allowed)
		if acc.subsetOf(t.allowed) {
			return a, i
		}
	}
	last := len(ps.assignments) - 1
	return ps.assignments[last], last
}
