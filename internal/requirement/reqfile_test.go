package requirement

import "testing"

func TestParseRequirementsFile(t *testing.T) {
	content := `# a project's dependencies
requests>=2.28    # http client
Django>=3.2,<4.0

flask[async] \
    >=2.0

-r other-requirements.txt
-c constraints.txt
-e ./local-package

--index-url https://example.com/simple
`
	rf, err := ParseRequirementsFile(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(rf.Requirements) != 3 {
		t.Fatalf("expected 3 requirements, got %d: %v", len(rf.Requirements), names(rf))
	}
	if rf.Requirements[0].Name != "requests" {
		t.Errorf("first requirement = %q", rf.Requirements[0].Name)
	}
	if rf.Requirements[2].Name != "flask" || len(rf.Requirements[2].Extras) != 1 {
		t.Errorf("continuation line did not parse: %+v", rf.Requirements[2])
	}
	if len(rf.Includes) != 1 || rf.Includes[0] != "other-requirements.txt" {
		t.Errorf("includes = %v", rf.Includes)
	}
	if len(rf.Constraints) != 1 || rf.Constraints[0] != "constraints.txt" {
		t.Errorf("constraints = %v", rf.Constraints)
	}
	if len(rf.Editables) != 1 || rf.Editables[0] != "./local-package" {
		t.Errorf("editables = %v", rf.Editables)
	}
}

func TestParseRequirementsFileComments(t *testing.T) {
	content := "# full comment\n\n   \nnumpy\n"
	rf, err := ParseRequirementsFile(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(rf.Requirements) != 1 || rf.Requirements[0].Name != "numpy" {
		t.Fatalf("expected only numpy, got %v", names(rf))
	}
}

func names(rf *RequirementsFile) []string {
	var out []string
	for _, r := range rf.Requirements {
		out = append(out, r.Name)
	}
	return out
}
