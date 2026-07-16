package requirement

import (
	"strings"
)

// RequirementsFile is the parsed content of a pip requirements file.
type RequirementsFile struct {
	Requirements []*Requirement
	Includes     []string // referenced with -r or --requirement
	Constraints  []string // referenced with -c or --constraint
	Editables    []string // referenced with -e or --editable
}

// ParseRequirementsFile parses the text of a pip requirements file. It handles
// comments, blank lines, and backslash line continuations, and recognizes the
// common -r, -c, and -e options. Other global options are ignored.
func ParseRequirementsFile(content string) (*RequirementsFile, error) {
	rf := &RequirementsFile{}
	for _, line := range logicalLines(content) {
		line = stripComment(line)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if arg, ok := optionArg(line, "-r", "--requirement"); ok {
			rf.Includes = append(rf.Includes, arg)
			continue
		}
		if arg, ok := optionArg(line, "-c", "--constraint"); ok {
			rf.Constraints = append(rf.Constraints, arg)
			continue
		}
		if arg, ok := optionArg(line, "-e", "--editable"); ok {
			rf.Editables = append(rf.Editables, arg)
			continue
		}
		if strings.HasPrefix(line, "-") {
			// Other global options such as --index-url are not our concern here.
			continue
		}

		req, err := Parse(line)
		if err != nil {
			return nil, err
		}
		rf.Requirements = append(rf.Requirements, req)
	}
	return rf, nil
}

// logicalLines joins physical lines that end with a backslash.
func logicalLines(content string) []string {
	var lines []string
	var current strings.Builder
	for _, raw := range strings.Split(content, "\n") {
		raw = strings.TrimRight(raw, "\r")
		if strings.HasSuffix(raw, "\\") {
			current.WriteString(raw[:len(raw)-1])
			continue
		}
		current.WriteString(raw)
		lines = append(lines, current.String())
		current.Reset()
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

// stripComment removes a trailing comment. A comment starts at a hash that is at
// the start of the line or preceded by whitespace.
func stripComment(line string) string {
	if strings.HasPrefix(strings.TrimSpace(line), "#") {
		return ""
	}
	for i := 1; i < len(line); i++ {
		if line[i] == '#' && (line[i-1] == ' ' || line[i-1] == '\t') {
			return line[:i]
		}
	}
	return line
}

// optionArg returns the argument of an option line for any of the given flags.
func optionArg(line string, flags ...string) (string, bool) {
	for _, flag := range flags {
		if line == flag {
			return "", true
		}
		if strings.HasPrefix(line, flag+" ") {
			return strings.TrimSpace(line[len(flag):]), true
		}
		if strings.HasPrefix(line, flag+"=") {
			return strings.TrimSpace(line[len(flag)+1:]), true
		}
	}
	return "", false
}
