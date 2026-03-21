package recipe

// NewEmbeddedProvider creates a RegistryProvider backed by an in-memory store
// populated from go:embed recipes. Returns nil if the registry is nil.
func NewEmbeddedProvider(er *EmbeddedRegistry) *RegistryProvider {
	if er == nil {
		return nil
	}
	store := NewMemoryStoreFromEmbedded(er)
	return NewRegistryProvider("embedded", SourceEmbedded, Manifest{Layout: "flat"}, store)
}
