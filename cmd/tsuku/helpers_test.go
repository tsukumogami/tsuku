package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/registry"
)

func TestPrintWarning_WritesToStderr(t *testing.T) {
	origQuiet := quietFlag
	defer func() { quietFlag = origQuiet }()

	quietFlag = false

	// Capture stderr
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printWarning("test warning message")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if !strings.Contains(buf.String(), "test warning message") {
		t.Errorf("expected stderr to contain 'test warning message', got %q", buf.String())
	}
}

func TestPrintWarning_QuietSuppresses(t *testing.T) {
	origQuiet := quietFlag
	defer func() { quietFlag = origQuiet }()

	quietFlag = true

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printWarning("should not appear")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if buf.Len() > 0 {
		t.Errorf("expected no output in quiet mode, got %q", buf.String())
	}
}

func TestCheckDeprecationWarning_NilManifest(t *testing.T) {
	resetDeprecationWarning()
	defer resetDeprecationWarning()

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	checkDeprecationWarning(nil, "https://example.com")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if buf.Len() > 0 {
		t.Errorf("expected no output for nil manifest, got %q", buf.String())
	}
}

func TestCheckDeprecationWarning_NoDeprecation(t *testing.T) {
	resetDeprecationWarning()
	defer resetDeprecationWarning()

	manifest := &registry.Manifest{
		SchemaVersion: 1,
		Deprecation:   nil,
	}

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	checkDeprecationWarning(manifest, "https://example.com")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if buf.Len() > 0 {
		t.Errorf("expected no output when deprecation is nil, got %q", buf.String())
	}
}

func TestCheckDeprecationWarning_DisplaysWarning(t *testing.T) {
	resetDeprecationWarning()
	defer resetDeprecationWarning()

	origQuiet := quietFlag
	defer func() { quietFlag = origQuiet }()
	quietFlag = false

	manifest := &registry.Manifest{
		SchemaVersion: 1,
		Deprecation: &registry.DeprecationNotice{
			SunsetDate:    "2026-09-01",
			MinCLIVersion: "v99.0.0",
			Message:       "Schema v2 coming soon.",
		},
	}

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	checkDeprecationWarning(manifest, "https://tsuku.dev/recipes.json")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Warning:") || !strings.Contains(output, "Schema v2 coming soon.") {
		t.Errorf("expected warning containing message, got %q", output)
	}
}

func TestCheckDeprecationWarning_FiresOnce(t *testing.T) {
	resetDeprecationWarning()
	defer resetDeprecationWarning()

	origQuiet := quietFlag
	defer func() { quietFlag = origQuiet }()
	quietFlag = false

	manifest := &registry.Manifest{
		SchemaVersion: 1,
		Deprecation: &registry.DeprecationNotice{
			SunsetDate:    "2026-09-01",
			MinCLIVersion: "v99.0.0",
			Message:       "Unique marker for dedup test.",
		},
	}

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Call three times
	checkDeprecationWarning(manifest, "https://example.com")
	checkDeprecationWarning(manifest, "https://example.com")
	checkDeprecationWarning(manifest, "https://example.com")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	count := strings.Count(output, "Unique marker for dedup test.")
	if count != 1 {
		t.Errorf("expected warning to fire exactly once, but found %d occurrences in %q", count, output)
	}
}

func TestCheckDeprecationWarning_QuietSuppresses(t *testing.T) {
	resetDeprecationWarning()
	defer resetDeprecationWarning()

	origQuiet := quietFlag
	defer func() { quietFlag = origQuiet }()
	quietFlag = true

	manifest := &registry.Manifest{
		SchemaVersion: 1,
		Deprecation: &registry.DeprecationNotice{
			SunsetDate:    "2026-09-01",
			MinCLIVersion: "v0.5.0",
			Message:       "Should not appear.",
		},
	}

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	checkDeprecationWarning(manifest, "https://example.com")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if buf.Len() > 0 {
		t.Errorf("expected no output in quiet mode, got %q", buf.String())
	}
}

func TestCheckDeprecationWarning_UpgradeNeeded(t *testing.T) {
	resetDeprecationWarning()
	defer resetDeprecationWarning()

	origQuiet := quietFlag
	defer func() { quietFlag = origQuiet }()
	quietFlag = false

	manifest := &registry.Manifest{
		SchemaVersion: 1,
		Deprecation: &registry.DeprecationNotice{
			SunsetDate:    "2026-09-01",
			MinCLIVersion: "v99.0.0",
			Message:       "Upgrade needed test.",
		},
	}

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	checkDeprecationWarning(manifest, "https://example.com")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Current dev build version will be "dev" or "dev-<hash>" or "unknown",
	// which are all dev builds, so version comparison is skipped.
	// This test validates the basic warning format.
	if !strings.Contains(output, "Warning:") || !strings.Contains(output, "Upgrade needed test.") {
		t.Errorf("expected warning format, got %q", output)
	}
}

func TestFormatDeprecationWarning_CLIBelowMinVersion(t *testing.T) {
	dep := &registry.DeprecationNotice{
		SunsetDate:    "2026-09-01",
		MinCLIVersion: "v0.5.0",
		Message:       "Registry format changing.",
	}

	msg := formatDeprecationWarning(dep, "https://tsuku.dev/recipes.json", "v0.3.0")

	if !strings.Contains(msg, "Warning: tsuku v0.5.0 or later is required") {
		t.Errorf("expected version requirement in warning, got %q", msg)
	}
	if !strings.Contains(msg, "after 2026-09-01") {
		t.Errorf("expected sunset date, got %q", msg)
	}
	if !strings.Contains(msg, "Upgrade: curl -fsSL https://get.tsuku.dev/now | bash") {
		t.Errorf("expected install command, got %q", msg)
	}
}

func TestFormatDeprecationWarning_CLIMeetsMinVersion(t *testing.T) {
	dep := &registry.DeprecationNotice{
		SunsetDate:    "2026-09-01",
		MinCLIVersion: "v0.5.0",
		Message:       "Registry format changing.",
	}

	msg := formatDeprecationWarning(dep, "https://tsuku.dev/recipes.json", "v0.5.0")

	if !strings.Contains(msg, "already compatible") {
		t.Errorf("expected 'already compatible' message, got %q", msg)
	}
	if !strings.Contains(msg, "tsuku update-registry") {
		t.Errorf("expected update-registry suggestion, got %q", msg)
	}
}

func TestFormatDeprecationWarning_CLIAboveMinVersion(t *testing.T) {
	dep := &registry.DeprecationNotice{
		SunsetDate:    "2026-09-01",
		MinCLIVersion: "v0.5.0",
		Message:       "Registry format changing.",
	}

	msg := formatDeprecationWarning(dep, "https://example.com", "v1.0.0")

	if !strings.Contains(msg, "already compatible") {
		t.Errorf("expected 'already compatible' message, got %q", msg)
	}
	// Should NOT suggest upgrading
	if strings.Contains(msg, "is required") {
		t.Errorf("should not suggest upgrading when CLI version is above min, got %q", msg)
	}
}

func TestFormatDeprecationWarning_DevBuildSkipsComparison(t *testing.T) {
	dep := &registry.DeprecationNotice{
		SunsetDate:    "2026-09-01",
		MinCLIVersion: "v99.0.0",
		Message:       "Dev build test.",
	}

	for _, devVer := range []string{"dev", "dev-abc123", "dev-abc123-dirty", "unknown"} {
		t.Run(devVer, func(t *testing.T) {
			msg := formatDeprecationWarning(dep, "https://example.com", devVer)

			if strings.Contains(msg, "is required") {
				t.Errorf("dev build should not suggest upgrading, got %q", msg)
			}
			if strings.Contains(msg, "already compatible") {
				t.Errorf("dev build should not show compatibility status, got %q", msg)
			}
			// Should still show the warning with the message
			if !strings.Contains(msg, "Warning: Dev build test.") {
				t.Errorf("expected warning with message for dev builds, got %q", msg)
			}
		})
	}
}

func TestFormatDeprecationWarning_NeverSuggestsDowngrade(t *testing.T) {
	// When CLI version is v1.0.0 and min is v0.5.0, should say "already compatible"
	// not "v0.5.0 is required" (which would imply a downgrade)
	dep := &registry.DeprecationNotice{
		SunsetDate:    "2026-09-01",
		MinCLIVersion: "v0.5.0",
		Message:       "Downgrade prevention test.",
	}

	msg := formatDeprecationWarning(dep, "https://example.com", "v1.0.0")

	if strings.Contains(msg, "v0.5.0 or later is required") {
		t.Errorf("CLI should never suggest downgrading, got %q", msg)
	}
}

func TestIsDevBuild(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"dev", true},
		{"dev-abc123", true},
		{"dev-abc123-dirty", true},
		{"unknown", true},
		{"v0.1.0", false},
		{"v1.0.0-rc.1", false},
		{"1.0.0", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("version=%q", tt.version), func(t *testing.T) {
			got := isDevBuild(tt.version)
			if got != tt.want {
				t.Errorf("isDevBuild(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}
