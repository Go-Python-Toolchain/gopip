package pypi

import (
	"context"
	"testing"

	"github.com/Go-Python-Toolchain/gopip/internal/version"
)

func TestMemSource(t *testing.T) {
	m := NewMemSource()
	if err := m.AddPackage("Flask", "2.0.0", "Werkzeug>=2.0", "click>=7.0"); err != nil {
		t.Fatal(err)
	}
	if err := m.AddPackage("flask", "2.1.0"); err != nil {
		t.Fatal(err)
	}

	// Lookup is by canonical name, so Flask and flask are the same package.
	versions, err := m.Versions(context.Background(), "FLASK")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].String() != "2.0.0" || versions[1].String() != "2.1.0" {
		t.Fatalf("unexpected versions: %v", versions)
	}

	info, err := m.Release(context.Background(), "flask", version.MustParse("2.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	if len(info.RequiresDist) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(info.RequiresDist))
	}
}

func TestMemSourceNotFound(t *testing.T) {
	m := NewMemSource()
	if _, err := m.Versions(context.Background(), "absent"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	m.AddPackage("present", "1.0")
	if _, err := m.Release(context.Background(), "present", version.MustParse("9.9")); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing version, got %v", err)
	}
}
