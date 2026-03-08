package addon

import "github.com/tsukumogami/tsuku/internal/verify"

// setPinnedLlmVersionForTest sets the pinned LLM version for testing purposes.
// Returns the previous value so it can be restored with defer.
func setPinnedLlmVersionForTest(version string) string {
	old := verify.PinnedLlmVersion()
	verify.SetPinnedLlmVersionForTest(version)
	return old
}
