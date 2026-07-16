package version

import "testing"

func TestParseAndNormalize(t *testing.T) {
	cases := map[string]string{
		"1.0":            "1.0",
		"1.2.3":          "1.2.3",
		"v1.2.3":         "1.2.3",
		"1.0a1":          "1.0a1",
		"1.0alpha1":      "1.0a1",
		"1.0.alpha.1":    "1.0a1",
		"1.0beta2":       "1.0b2",
		"1.0c1":          "1.0rc1",
		"1.0preview1":    "1.0rc1",
		"1.0rc1":         "1.0rc1",
		"1.0-1":          "1.0.post1",
		"1.0rev2":        "1.0.post2",
		"1.0.post3":      "1.0.post3",
		"1.0.dev4":       "1.0.dev4",
		"1!2.0":          "1!2.0",
		"1.0+Ubuntu.1":   "1.0+ubuntu.1",
		"1.0A1":          "1.0a1",
		"  1.0  ":        "1.0",
		"1.0.post1.dev2": "1.0.post1.dev2",
	}
	for input, want := range cases {
		v, err := Parse(input)
		if err != nil {
			t.Errorf("Parse(%q) failed: %v", input, err)
			continue
		}
		if got := v.String(); got != want {
			t.Errorf("Parse(%q).String() = %q, want %q", input, got, want)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	for _, s := range []string{"", "abc", "1..2", "1.0.", "not-a-version", "1,0"} {
		if v, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) should have failed, got %v", s, v)
		}
	}
}

func TestEqualityIgnoresTrailingZeros(t *testing.T) {
	pairs := [][2]string{
		{"1.0", "1.0.0"},
		{"1", "1.0.0"},
		{"1.0.0", "1.0"},
		{"2!1.0", "2!1.0.0"},
	}
	for _, p := range pairs {
		if !Equal(MustParse(p[0]), MustParse(p[1])) {
			t.Errorf("expected %q == %q", p[0], p[1])
		}
	}
}

func TestOrderingCanonical(t *testing.T) {
	// This ascending sequence is drawn from the PEP 440 ordering examples.
	ordered := []string{
		"1.0.dev456",
		"1.0a1",
		"1.0a2.dev456",
		"1.0a12.dev456",
		"1.0a12",
		"1.0b1.dev456",
		"1.0b2",
		"1.0b2.post345.dev456",
		"1.0b2.post345",
		"1.0rc1.dev456",
		"1.0rc1",
		"1.0",
		"1.0+abc.5",
		"1.0+abc.7",
		"1.0+5",
		"1.0.post456.dev34",
		"1.0.post456",
		"1.1.dev1",
	}
	for i := 0; i+1 < len(ordered); i++ {
		a := MustParse(ordered[i])
		b := MustParse(ordered[i+1])
		if Compare(a, b) >= 0 {
			t.Errorf("expected %q < %q, got Compare = %d", ordered[i], ordered[i+1], Compare(a, b))
		}
		if Compare(b, a) <= 0 {
			t.Errorf("expected %q > %q in reverse", ordered[i+1], ordered[i])
		}
	}
}

func TestEpochOrdering(t *testing.T) {
	if Compare(MustParse("1!1.0"), MustParse("2.0")) <= 0 {
		t.Error("expected epoch 1 version to be greater than epoch 0 version")
	}
}

func TestPrereleaseFlags(t *testing.T) {
	if !MustParse("1.0a1").IsPrerelease() {
		t.Error("1.0a1 should be a pre-release")
	}
	if !MustParse("1.0.dev1").IsPrerelease() {
		t.Error("1.0.dev1 should be a pre-release")
	}
	if MustParse("1.0").IsPrerelease() {
		t.Error("1.0 should not be a pre-release")
	}
	if !MustParse("1.0.post1").IsPostrelease() {
		t.Error("1.0.post1 should be a post-release")
	}
}

func TestPublicAndBase(t *testing.T) {
	v := MustParse("1.2.3a1.post4.dev5+local.7")
	if v.Public().String() != "1.2.3a1.post4.dev5" {
		t.Errorf("Public = %q", v.Public().String())
	}
	if v.BaseVersion().String() != "1.2.3" {
		t.Errorf("BaseVersion = %q", v.BaseVersion().String())
	}
}
