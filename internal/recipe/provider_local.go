package recipe

// NewLocalProvider creates a RegistryProvider backed by a filesystem store
// rooted at the given directory ($TSUKU_HOME/recipes).
func NewLocalProvider(dir string) *RegistryProvider {
	store := NewFSStore(dir)
	return NewRegistryProvider("local", SourceLocal, Manifest{Layout: "flat"}, store)
}
