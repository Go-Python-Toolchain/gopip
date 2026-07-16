package requirement

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

// knownMarkerVars is the set of environment marker variables defined by PEP 508.
var knownMarkerVars = map[string]bool{
	"python_version":                 true,
	"python_full_version":            true,
	"os_name":                        true,
	"sys_platform":                   true,
	"platform_release":               true,
	"platform_system":                true,
	"platform_version":               true,
	"platform_machine":               true,
	"platform_python_implementation": true,
	"implementation_name":            true,
	"implementation_version":         true,
	"extra":                          true,
}

// Environment maps marker variables to values for evaluation.
type Environment map[string]string

// CurrentEnvironment builds an environment from the host, using the given target
// Python version for the python_version and related variables. The Python
// version is a caller input because gopip resolves for a chosen interpreter, not
// for the Go runtime.
func CurrentEnvironment(pythonVersion string) Environment {
	osName := "posix"
	sysPlatform := runtime.GOOS
	platformSystem := "Linux"
	switch runtime.GOOS {
	case "windows":
		osName = "nt"
		sysPlatform = "win32"
		platformSystem = "Windows"
	case "darwin":
		sysPlatform = "darwin"
		platformSystem = "Darwin"
	case "linux":
		sysPlatform = "linux"
		platformSystem = "Linux"
	}

	machine := runtime.GOARCH
	switch runtime.GOARCH {
	case "amd64":
		machine = "x86_64"
	case "arm64":
		machine = "arm64"
	}

	fullVersion := pythonVersion
	if strings.Count(pythonVersion, ".") < 2 {
		fullVersion = pythonVersion + ".0"
	}

	return Environment{
		"python_version":                 pythonVersion,
		"python_full_version":            fullVersion,
		"os_name":                        osName,
		"sys_platform":                   sysPlatform,
		"platform_system":                platformSystem,
		"platform_machine":               machine,
		"platform_release":               "",
		"platform_version":               "",
		"platform_python_implementation": "CPython",
		"implementation_name":            "cpython",
		"implementation_version":         fullVersion,
		"extra":                          "",
	}
}

// Marker is an environment marker expression.
type Marker interface {
	Evaluate(env Environment) bool
	String() string
}

type markerOr struct{ left, right Marker }
type markerAnd struct{ left, right Marker }
type markerComparison struct {
	left  markerOperand
	op    string
	right markerOperand
}

// markerOperand is either a variable reference or a string literal.
type markerOperand struct {
	isVar   bool
	name    string
	literal string
}

func (m markerOr) Evaluate(env Environment) bool {
	return m.left.Evaluate(env) || m.right.Evaluate(env)
}
func (m markerAnd) Evaluate(env Environment) bool {
	return m.left.Evaluate(env) && m.right.Evaluate(env)
}

func (m markerComparison) Evaluate(env Environment) bool {
	l := m.left.resolve(env)
	r := m.right.resolve(env)
	return evalMarkerOp(l, m.op, r)
}

func (o markerOperand) resolve(env Environment) string {
	if o.isVar {
		return env[o.name]
	}
	return o.literal
}

func (o markerOperand) String() string {
	if o.isVar {
		return o.name
	}
	return `"` + o.literal + `"`
}

func (m markerOr) String() string  { return m.left.String() + " or " + m.right.String() }
func (m markerAnd) String() string { return m.left.String() + " and " + m.right.String() }
func (m markerComparison) String() string {
	return m.left.String() + " " + m.op + " " + m.right.String()
}

// evalMarkerOp evaluates one marker comparison. Version-shaped comparisons use
// PEP 440 version ordering; everything else falls back to string operations.
func evalMarkerOp(lhs, op, rhs string) bool {
	switch op {
	case "in":
		return strings.Contains(rhs, lhs)
	case "not in":
		return !strings.Contains(rhs, lhs)
	}

	if ss, err := version.ParseSpecifierSet(op + rhs); err == nil {
		if v, err := version.Parse(lhs); err == nil {
			return ss.Contains(v, true)
		}
	}

	switch op {
	case "==", "===":
		return lhs == rhs
	case "!=":
		return lhs != rhs
	case "<":
		return lhs < rhs
	case "<=":
		return lhs <= rhs
	case ">":
		return lhs > rhs
	case ">=":
		return lhs >= rhs
	}
	return false
}

// ParseMarker parses a PEP 508 marker expression.
func ParseMarker(s string) (Marker, error) {
	toks, err := tokenizeMarker(s)
	if err != nil {
		return nil, err
	}
	p := &markerParser{toks: toks}
	m, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.cur().kind != mtEOF {
		return nil, fmt.Errorf("unexpected %q in marker", p.cur().val)
	}
	return m, nil
}

// Marker token kinds.
const (
	mtLParen = "lparen"
	mtRParen = "rparen"
	mtAnd    = "and"
	mtOr     = "or"
	mtOp     = "op"
	mtVar    = "var"
	mtStr    = "str"
	mtEOF    = "eof"
)

type mtoken struct {
	kind string
	val  string
}

var markerOps = []string{"===", "==", "!=", "<=", ">=", "~=", "<", ">"}

func tokenizeMarker(s string) ([]mtoken, error) {
	var toks []mtoken
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case c == '(':
			toks = append(toks, mtoken{mtLParen, "("})
			i++
		case c == ')':
			toks = append(toks, mtoken{mtRParen, ")"})
			i++
		case c == '\'' || c == '"':
			quote := c
			j := i + 1
			for j < len(s) && s[j] != quote {
				j++
			}
			if j >= len(s) {
				return nil, fmt.Errorf("unterminated string in marker")
			}
			toks = append(toks, mtoken{mtStr, s[i+1 : j]})
			i = j + 1
		default:
			if op := matchOp(s[i:]); op != "" {
				toks = append(toks, mtoken{mtOp, op})
				i += len(op)
				continue
			}
			if isIdentStart(c) {
				j := i
				for j < len(s) && isIdentPart(s[j]) {
					j++
				}
				word := s[i:j]
				i = j
				switch word {
				case "and":
					toks = append(toks, mtoken{mtAnd, word})
				case "or":
					toks = append(toks, mtoken{mtOr, word})
				case "in":
					toks = append(toks, mtoken{mtOp, "in"})
				case "not":
					// Must be followed by in.
					k := i
					for k < len(s) && (s[k] == ' ' || s[k] == '\t') {
						k++
					}
					if strings.HasPrefix(s[k:], "in") {
						toks = append(toks, mtoken{mtOp, "not in"})
						i = k + 2
					} else {
						return nil, fmt.Errorf("expected 'in' after 'not' in marker")
					}
				default:
					toks = append(toks, mtoken{mtVar, word})
				}
				continue
			}
			return nil, fmt.Errorf("unexpected character %q in marker", string(c))
		}
	}
	toks = append(toks, mtoken{mtEOF, ""})
	return toks, nil
}

func matchOp(s string) string {
	for _, op := range markerOps {
		if strings.HasPrefix(s, op) {
			return op
		}
	}
	return ""
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9') || c == '.'
}

type markerParser struct {
	toks []mtoken
	pos  int
}

func (p *markerParser) cur() mtoken { return p.toks[p.pos] }
func (p *markerParser) advance() mtoken {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *markerParser) parseOr() (Marker, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == mtOr {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = markerOr{left, right}
	}
	return left, nil
}

func (p *markerParser) parseAnd() (Marker, error) {
	left, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == mtAnd {
		p.advance()
		right, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		left = markerAnd{left, right}
	}
	return left, nil
}

func (p *markerParser) parseExpr() (Marker, error) {
	if p.cur().kind == mtLParen {
		p.advance()
		m, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.cur().kind != mtRParen {
			return nil, fmt.Errorf("expected ) in marker")
		}
		p.advance()
		return m, nil
	}
	return p.parseComparison()
}

func (p *markerParser) parseComparison() (Marker, error) {
	left, err := p.parseOperand()
	if err != nil {
		return nil, err
	}
	if p.cur().kind != mtOp {
		return nil, fmt.Errorf("expected an operator in marker, found %q", p.cur().val)
	}
	op := p.advance().val
	right, err := p.parseOperand()
	if err != nil {
		return nil, err
	}
	return markerComparison{left: left, op: op, right: right}, nil
}

func (p *markerParser) parseOperand() (markerOperand, error) {
	t := p.cur()
	switch t.kind {
	case mtStr:
		p.advance()
		return markerOperand{literal: t.val}, nil
	case mtVar:
		if !knownMarkerVars[t.val] {
			return markerOperand{}, fmt.Errorf("unknown marker variable %q", t.val)
		}
		p.advance()
		return markerOperand{isVar: true, name: t.val}, nil
	}
	return markerOperand{}, fmt.Errorf("expected a marker variable or string, found %q", t.val)
}
