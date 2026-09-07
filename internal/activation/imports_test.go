package activation

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/version"
)

// TestReachesSharedVersionRoutines demonstrates the property the package split
// exists for: activation can call the pin routines and the version ordering
// directly, with no import cycle. It is asserted by compiling and calling them
// rather than claimed in a comment, because the claim is exactly what was wrong
// before -- internal/install imports internal/shellenv, so activation could not
// reach any of this while it lived there.
func TestReachesSharedVersionRoutines(t *testing.T) {
	if !install.VersionMatchesPin("1.22.5", "1.22") {
		t.Fatal("VersionMatchesPin: unreachable or wrong")
	}
	if install.PinLevelFromRequested("@lts") != install.PinChannel {
		t.Fatal("PinLevelFromRequested: unreachable or wrong")
	}
	if err := install.ValidateRequested("1.22"); err != nil {
		t.Fatalf("ValidateRequested: unreachable: %v", err)
	}
	if got := version.SortVersionsDescending([]string{"9.0.0", "10.0.0"}); got[0] != "10.0.0" {
		t.Fatalf("SortVersionsDescending: unreachable or wrong: %v", got)
	}
}
