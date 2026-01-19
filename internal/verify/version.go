package verify

// pinnedDltestVersion is the expected tsuku-dltest version for this release.
// Injected at build time via ldflags. When "dev", any installed version is accepted.
var pinnedDltestVersion = "dev"

// PinnedDltestVersion returns the expected tsuku-dltest version.
func PinnedDltestVersion() string {
	return pinnedDltestVersion
}
