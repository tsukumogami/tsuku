package verify

import "github.com/tsukumogami/tsuku/internal/install"

// SonameIndex provides O(1) reverse lookups from soname to recipe.
// Built from state.json at verification time to enable fast dependency classification.
type SonameIndex struct {
	// SonameToRecipe maps soname to recipe name (e.g., "libssl.so.3" -> "openssl")
	SonameToRecipe map[string]string

	// SonameToVersion maps soname to installed version (e.g., "libssl.so.3" -> "3.2.1")
	SonameToVersion map[string]string
}

// NewSonameIndex creates an empty SonameIndex with initialized maps.
func NewSonameIndex() *SonameIndex {
	return &SonameIndex{
		SonameToRecipe:  make(map[string]string),
		SonameToVersion: make(map[string]string),
	}
}

// BuildSonameIndex creates a reverse index from all installed libraries in state.
// The index maps sonames to their providing recipe and version, enabling O(1)
// lookup during dependency classification.
//
// If multiple libraries provide the same soname (collision), the last one
// encountered wins. This is a documented edge case that shouldn't occur in
// properly-managed installations.
func BuildSonameIndex(state *install.State) *SonameIndex {
	index := NewSonameIndex()

	if state == nil || state.Libs == nil {
		return index
	}

	// Iterate state.Libs to populate index
	// state.Libs is map[libName]map[version]LibraryVersionState
	for libName, versions := range state.Libs {
		for version, versionState := range versions {
			for _, soname := range versionState.Sonames {
				index.SonameToRecipe[soname] = libName
				index.SonameToVersion[soname] = version
			}
		}
	}

	return index
}

// Lookup returns the recipe name and version for a given soname.
// Returns empty strings and false if the soname is not in the index.
func (idx *SonameIndex) Lookup(soname string) (recipe, version string, found bool) {
	recipe, found = idx.SonameToRecipe[soname]
	if !found {
		return "", "", false
	}
	version = idx.SonameToVersion[soname]
	return recipe, version, true
}

// Contains returns true if the soname is in the index.
func (idx *SonameIndex) Contains(soname string) bool {
	_, found := idx.SonameToRecipe[soname]
	return found
}

// Size returns the number of sonames in the index.
func (idx *SonameIndex) Size() int {
	return len(idx.SonameToRecipe)
}
