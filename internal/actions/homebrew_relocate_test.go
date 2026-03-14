package actions

import (
	"testing"
)

// -- homebrew_relocate.go: Dependencies, extractBottlePrefixes --

func TestHomebrewRelocateAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := HomebrewRelocateAction{}
	deps := action.Dependencies()
	if len(deps.LinuxInstallTime) != 1 || deps.LinuxInstallTime[0] != "patchelf" {
		t.Errorf("Dependencies().LinuxInstallTime = %v, want [patchelf]", deps.LinuxInstallTime)
	}
}

func TestHomebrewRelocateAction_ExtractBottlePrefixes(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}

	content := []byte(`some text /tmp/action-validator-abc12345/.install/libyaml/0.2.5/lib/libyaml.so more text
another line /tmp/action-validator-abc12345/.install/libyaml/0.2.5/include/yaml.h end`)

	prefixMap := make(map[string]string)
	action.extractBottlePrefixes(content, prefixMap)

	if len(prefixMap) != 2 {
		t.Errorf("extractBottlePrefixes() found %d entries, want 2", len(prefixMap))
	}

	expectedPrefix := "/tmp/action-validator-abc12345/.install/libyaml/0.2.5"
	for fullPath, prefix := range prefixMap {
		if prefix != expectedPrefix {
			t.Errorf("prefix for %q = %q, want %q", fullPath, prefix, expectedPrefix)
		}
	}
}

func TestHomebrewRelocateAction_ExtractBottlePrefixes_NoMatch(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}
	prefixMap := make(map[string]string)
	action.extractBottlePrefixes([]byte("no bottle paths here"), prefixMap)
	if len(prefixMap) != 0 {
		t.Errorf("extractBottlePrefixes() found %d entries for no-match content, want 0", len(prefixMap))
	}
}

func TestHomebrewRelocateAction_ExtractBottlePrefixes_NoInstallSegment(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}
	// Has the marker but no /.install/ segment
	content := []byte("/tmp/action-validator-abc12345/other/path")
	prefixMap := make(map[string]string)
	action.extractBottlePrefixes(content, prefixMap)
	if len(prefixMap) != 0 {
		t.Errorf("extractBottlePrefixes() found %d entries for no-install content, want 0", len(prefixMap))
	}
}
