package version

import "testing"

func matches(t *testing.T, spec, ver string) bool {
	t.Helper()
	ss, err := ParseSpecifierSet(spec)
	if err != nil {
		t.Fatalf("ParseSpecifierSet(%q) failed: %v", spec, err)
	}
	return ss.Matches(MustParse(ver))
}

func TestBasicOperators(t *testing.T) {
	type c struct {
		spec, ver string
		want      bool
	}
	cases := []c{
		{">=1.0", "1.0", true},
		{">=1.0", "0.9", false},
		{">1.0", "1.0", false},
		{">1.0", "1.0.1", true},
		{"<=1.0", "1.0", true},
		{"<1.0", "1.0", false},
		{"<1.0", "0.9", true},
		{"==1.2.3", "1.2.3", true},
		{"==1.2.3", "1.2.4", false},
		{"!=1.2.3", "1.2.4", true},
		{"!=1.2.3", "1.2.3", false},
		{"==1.0", "1.0.0", true},
		{"==1.0", "1.0.0+local", true},
		{"==1.0+local", "1.0.0+local", true},
		{"==1.0+local", "1.0.0", false},
	}
	for _, tc := range cases {
		if got := matches(t, tc.spec, tc.ver); got != tc.want {
			t.Errorf("%q matches %q = %v, want %v", tc.spec, tc.ver, got, tc.want)
		}
	}
}

func TestRangeAndSet(t *testing.T) {
	if !matches(t, ">=1.0,<2.0", "1.5") {
		t.Error("1.5 should satisfy >=1.0,<2.0")
	}
	if matches(t, ">=1.0,<2.0", "2.0") {
		t.Error("2.0 should not satisfy >=1.0,<2.0")
	}
	if matches(t, ">=1.0,<2.0", "0.5") {
		t.Error("0.5 should not satisfy >=1.0,<2.0")
	}
}

func TestWildcard(t *testing.T) {
	cases := []struct {
		spec, ver string
		want      bool
	}{
		{"==1.4.*", "1.4", true},
		{"==1.4.*", "1.4.5", true},
		{"==1.4.*", "1.5", false},
		{"==1.4.*", "1.4.0", true},
		{"==2.*", "2.9.9", true},
		{"==2.*", "3.0", false},
		{"!=1.4.*", "1.5", true},
		{"!=1.4.*", "1.4.2", false},
	}
	for _, tc := range cases {
		if got := matches(t, tc.spec, tc.ver); got != tc.want {
			t.Errorf("%q matches %q = %v, want %v", tc.spec, tc.ver, got, tc.want)
		}
	}
}

func TestCompatibleRelease(t *testing.T) {
	cases := []struct {
		spec, ver string
		want      bool
	}{
		{"~=2.2", "2.2", true},
		{"~=2.2", "2.9", true},
		{"~=2.2", "3.0", false},
		{"~=2.2", "2.1", false},
		{"~=1.4.5", "1.4.5", true},
		{"~=1.4.5", "1.4.9", true},
		{"~=1.4.5", "1.5.0", false},
		{"~=1.4.5", "1.4.4", false},
	}
	for _, tc := range cases {
		if got := matches(t, tc.spec, tc.ver); got != tc.want {
			t.Errorf("%q matches %q = %v, want %v", tc.spec, tc.ver, got, tc.want)
		}
	}
}

func TestPrereleaseExclusion(t *testing.T) {
	// A pre-release is excluded by default.
	if matches(t, ">=1.0", "2.0a1") {
		t.Error("pre-release 2.0a1 should be excluded from >=1.0 by default")
	}
	// Unless the version satisfies and pre-releases are allowed.
	ss, _ := ParseSpecifierSet(">=1.0")
	if !ss.Contains(MustParse("2.0a1"), true) {
		t.Error("2.0a1 should satisfy >=1.0 when pre-releases are allowed")
	}
	// A specifier that names a pre-release opts in.
	if !matches(t, ">=1.0a1", "1.0a5") {
		t.Error("naming a pre-release should allow pre-release versions")
	}
}

func TestArbitraryEquality(t *testing.T) {
	if !matches(t, "===1.0+ubuntu1", "1.0+ubuntu1") {
		t.Error("arbitrary equality should match the exact string")
	}
	if matches(t, "===1.0", "1.0.0") {
		t.Error("arbitrary equality should not normalize")
	}
}

func TestEmptySetMatchesAll(t *testing.T) {
	if !matches(t, "", "1.2.3") {
		t.Error("an empty specifier set should match any final release")
	}
}

func TestInvalidSpecifiers(t *testing.T) {
	for _, s := range []string{">=", "1.0", "~=1", "<=1.*", "=>1.0"} {
		if _, err := ParseSpecifierSet(s); err == nil {
			t.Errorf("expected %q to be rejected", s)
		}
	}
}
