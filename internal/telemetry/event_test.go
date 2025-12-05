package telemetry

import (
	"runtime"
	"testing"

	"github.com/tsukumogami/tsuku/internal/buildinfo"
)

func TestNewInstallEvent(t *testing.T) {
	e := NewInstallEvent("nodejs", "@LTS", "22.0.0", false)

	if e.Action != "install" {
		t.Errorf("Action = %q, want %q", e.Action, "install")
	}
	if e.Recipe != "nodejs" {
		t.Errorf("Recipe = %q, want %q", e.Recipe, "nodejs")
	}
	if e.VersionConstraint != "@LTS" {
		t.Errorf("VersionConstraint = %q, want %q", e.VersionConstraint, "@LTS")
	}
	if e.VersionResolved != "22.0.0" {
		t.Errorf("VersionResolved = %q, want %q", e.VersionResolved, "22.0.0")
	}
	if e.IsDependency != false {
		t.Errorf("IsDependency = %v, want %v", e.IsDependency, false)
	}
	if e.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", e.OS, runtime.GOOS)
	}
	if e.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", e.Arch, runtime.GOARCH)
	}
	if e.TsukuVersion != buildinfo.Version() {
		t.Errorf("TsukuVersion = %q, want %q", e.TsukuVersion, buildinfo.Version())
	}
	if e.SchemaVersion != "1" {
		t.Errorf("SchemaVersion = %q, want %q", e.SchemaVersion, "1")
	}
}

func TestNewInstallEvent_Dependency(t *testing.T) {
	e := NewInstallEvent("openssl", "", "3.0.0", true)

	if e.IsDependency != true {
		t.Errorf("IsDependency = %v, want %v", e.IsDependency, true)
	}
	if e.VersionConstraint != "" {
		t.Errorf("VersionConstraint = %q, want empty", e.VersionConstraint)
	}
}

func TestNewUpdateEvent(t *testing.T) {
	e := NewUpdateEvent("nodejs", "20.0.0", "22.0.0")

	if e.Action != "update" {
		t.Errorf("Action = %q, want %q", e.Action, "update")
	}
	if e.Recipe != "nodejs" {
		t.Errorf("Recipe = %q, want %q", e.Recipe, "nodejs")
	}
	if e.VersionPrevious != "20.0.0" {
		t.Errorf("VersionPrevious = %q, want %q", e.VersionPrevious, "20.0.0")
	}
	if e.VersionResolved != "22.0.0" {
		t.Errorf("VersionResolved = %q, want %q", e.VersionResolved, "22.0.0")
	}
	if e.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", e.OS, runtime.GOOS)
	}
}

func TestNewRemoveEvent(t *testing.T) {
	e := NewRemoveEvent("nodejs", "22.0.0")

	if e.Action != "remove" {
		t.Errorf("Action = %q, want %q", e.Action, "remove")
	}
	if e.Recipe != "nodejs" {
		t.Errorf("Recipe = %q, want %q", e.Recipe, "nodejs")
	}
	if e.VersionPrevious != "22.0.0" {
		t.Errorf("VersionPrevious = %q, want %q", e.VersionPrevious, "22.0.0")
	}
	if e.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", e.OS, runtime.GOOS)
	}
}
