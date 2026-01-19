package verify

import "testing"

func TestPinnedDltestVersion(t *testing.T) {
	// Default value when not injected via ldflags
	version := PinnedDltestVersion()
	if version != "dev" {
		t.Errorf("expected default version 'dev', got %q", version)
	}
}
