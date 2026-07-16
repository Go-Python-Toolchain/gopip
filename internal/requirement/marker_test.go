package requirement

import "testing"

func evalMarker(t *testing.T, expr string, env Environment) bool {
	t.Helper()
	m, err := ParseMarker(expr)
	if err != nil {
		t.Fatalf("ParseMarker(%q) failed: %v", expr, err)
	}
	return m.Evaluate(env)
}

func TestMarkerVersionComparison(t *testing.T) {
	if !evalMarker(t, `python_version >= "3.8"`, Environment{"python_version": "3.10"}) {
		t.Error("3.10 >= 3.8 should be true")
	}
	if evalMarker(t, `python_version >= "3.8"`, Environment{"python_version": "3.7"}) {
		t.Error("3.7 >= 3.8 should be false")
	}
	if !evalMarker(t, `python_version < "3.11"`, Environment{"python_version": "3.9"}) {
		t.Error("3.9 < 3.11 should be true")
	}
}

func TestMarkerStringComparison(t *testing.T) {
	if !evalMarker(t, `os_name == "posix"`, Environment{"os_name": "posix"}) {
		t.Error("posix == posix should be true")
	}
	if evalMarker(t, `sys_platform == "linux"`, Environment{"sys_platform": "darwin"}) {
		t.Error("darwin == linux should be false")
	}
	if !evalMarker(t, `sys_platform != "win32"`, Environment{"sys_platform": "linux"}) {
		t.Error("linux != win32 should be true")
	}
}

func TestMarkerInOperator(t *testing.T) {
	if !evalMarker(t, `"64" in platform_machine`, Environment{"platform_machine": "x86_64"}) {
		t.Error(`"64" in "x86_64" should be true`)
	}
	if !evalMarker(t, `"arm" not in platform_machine`, Environment{"platform_machine": "x86_64"}) {
		t.Error(`"arm" not in "x86_64" should be true`)
	}
}

func TestMarkerAndOr(t *testing.T) {
	env := Environment{"python_version": "3.10", "sys_platform": "linux"}
	if !evalMarker(t, `python_version >= "3.8" and sys_platform == "linux"`, env) {
		t.Error("both conditions true should be true")
	}
	if evalMarker(t, `python_version >= "3.8" and sys_platform == "darwin"`, env) {
		t.Error("one false with and should be false")
	}
	if !evalMarker(t, `python_version < "3.0" or sys_platform == "linux"`, env) {
		t.Error("one true with or should be true")
	}
}

func TestMarkerParentheses(t *testing.T) {
	env := Environment{"python_version": "3.10", "os_name": "nt"}
	if !evalMarker(t, `(python_version >= "3.8" or os_name == "posix") and python_version < "4.0"`, env) {
		t.Error("grouped expression should evaluate true")
	}
}

func TestMarkerExtra(t *testing.T) {
	if !evalMarker(t, `extra == "test"`, Environment{"extra": "test"}) {
		t.Error("extra == test should be true when extra is test")
	}
	if evalMarker(t, `extra == "test"`, Environment{"extra": "docs"}) {
		t.Error("extra == test should be false when extra is docs")
	}
}

func TestMarkerInvalid(t *testing.T) {
	for _, s := range []string{
		`python_version >=`,
		`unknown_var == "x"`,
		`python_version "3.8"`,
		`(python_version >= "3.8"`,
		`python_version >= "3.8" and`,
	} {
		if _, err := ParseMarker(s); err == nil {
			t.Errorf("ParseMarker(%q) should have failed", s)
		}
	}
}

func TestCurrentEnvironmentPopulated(t *testing.T) {
	env := CurrentEnvironment("3.12")
	if env["python_version"] != "3.12" {
		t.Errorf("python_version = %q", env["python_version"])
	}
	if env["python_full_version"] != "3.12.0" {
		t.Errorf("python_full_version = %q", env["python_full_version"])
	}
	for _, key := range []string{"os_name", "sys_platform", "platform_system", "platform_machine"} {
		if env[key] == "" {
			t.Errorf("expected %s to be set", key)
		}
	}
}
