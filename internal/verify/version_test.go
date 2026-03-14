package verify

import "testing"

func TestPinnedDltestVersion(t *testing.T) {
	// Default value when not injected via ldflags
	version := PinnedDltestVersion()
	if version != "dev" {
		t.Errorf("expected default version 'dev', got %q", version)
	}
}

func TestPinnedLlmVersion(t *testing.T) {
	version := PinnedLlmVersion()
	if version != "dev" {
		t.Errorf("expected default version 'dev', got %q", version)
	}
}

func TestSetPinnedLlmVersionForTest(t *testing.T) {
	original := PinnedLlmVersion()
	SetPinnedLlmVersionForTest("1.2.3")
	if PinnedLlmVersion() != "1.2.3" {
		t.Errorf("expected '1.2.3', got %q", PinnedLlmVersion())
	}
	SetPinnedLlmVersionForTest(original)
	if PinnedLlmVersion() != original {
		t.Errorf("expected %q after restore, got %q", original, PinnedLlmVersion())
	}
}
