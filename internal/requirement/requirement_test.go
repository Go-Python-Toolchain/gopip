package requirement

import "testing"

func TestParseSimple(t *testing.T) {
	r, err := Parse("requests")
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "requests" || len(r.Extras) != 0 || r.URL != "" || r.Marker != nil {
		t.Fatalf("unexpected requirement: %+v", r)
	}
}

func TestParseWithSpecifier(t *testing.T) {
	r, err := Parse("Django>=3.2,<4.0")
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Django" {
		t.Fatalf("name = %q", r.Name)
	}
	if r.Specifier.String() != ">=3.2,<4.0" {
		t.Fatalf("specifier = %q", r.Specifier.String())
	}
}

func TestParseWithExtras(t *testing.T) {
	r, err := Parse("requests[security,socks]>=2.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Extras) != 2 || r.Extras[0] != "security" || r.Extras[1] != "socks" {
		t.Fatalf("extras = %v", r.Extras)
	}
}

func TestParseParenthesizedSpecifier(t *testing.T) {
	r, err := Parse("requests (>=2.0,<3.0)")
	if err != nil {
		t.Fatal(err)
	}
	if r.Specifier.String() != ">=2.0,<3.0" {
		t.Fatalf("specifier = %q", r.Specifier.String())
	}
}

func TestParseURL(t *testing.T) {
	r, err := Parse("mypkg @ https://example.com/mypkg-1.0.whl")
	if err != nil {
		t.Fatal(err)
	}
	if r.URL != "https://example.com/mypkg-1.0.whl" {
		t.Fatalf("url = %q", r.URL)
	}
}

func TestParseWithMarker(t *testing.T) {
	r, err := Parse(`requests>=2.0; python_version < "3.11"`)
	if err != nil {
		t.Fatal(err)
	}
	if r.Marker == nil {
		t.Fatal("expected a marker")
	}
	if !r.AppliesTo(Environment{"python_version": "3.10"}) {
		t.Error("should apply when python_version is 3.10")
	}
	if r.AppliesTo(Environment{"python_version": "3.11"}) {
		t.Error("should not apply when python_version is 3.11")
	}
}

func TestCanonicalName(t *testing.T) {
	cases := map[string]string{
		"Flask":          "flask",
		"zope.interface": "zope-interface",
		"a_b-c":          "a-b-c",
		"PyYAML":         "pyyaml",
		"Foo...Bar":      "foo-bar",
	}
	for in, want := range cases {
		if got := CanonicalizeName(in); got != want {
			t.Errorf("CanonicalizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	for _, s := range []string{"", ">=1.0", "[extras]", "@"} {
		if r, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) should have failed, got %+v", s, r)
		}
	}
}

func TestRoundTripString(t *testing.T) {
	r, err := Parse(`requests[security]>=2.0; python_version >= "3.8"`)
	if err != nil {
		t.Fatal(err)
	}
	got := r.String()
	want := `requests[security]>=2.0; python_version >= "3.8"`
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
