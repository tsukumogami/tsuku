package verify

// SetPinnedLlmVersionForTest sets the pinned LLM version for testing.
// This is exported for use by other packages' tests.
func SetPinnedLlmVersionForTest(version string) {
	pinnedLlmVersion = version
}
