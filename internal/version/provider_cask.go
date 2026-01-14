package version

import (
	"context"
	"fmt"
)

// CaskProvider resolves versions from Homebrew Cask.
// This is a walking skeleton implementation with hardcoded metadata for iTerm2.
// Issue #863 will replace this with real Homebrew Cask API integration.
type CaskProvider struct {
	resolver *Resolver
	cask     string
}

// NewCaskProvider creates a provider for Homebrew Casks
func NewCaskProvider(resolver *Resolver, cask string) *CaskProvider {
	return &CaskProvider{
		resolver: resolver,
		cask:     cask,
	}
}

// hardcodedCaskMetadata contains stub metadata for the walking skeleton.
// This will be replaced with real API calls in issue #863.
var hardcodedCaskMetadata = map[string]struct {
	Version  string
	URL      string
	Checksum string
}{
	"iterm2": {
		Version:  "3.5.20241222",
		URL:      "https://iterm2.com/downloads/stable/iTerm2-3_5_20241222.zip",
		Checksum: "sha256:2accd46a4e1ea26e9ab05d5b92c22e225e6dfcc4eae9d4d93c4c2918d4b73b81",
	},
}

// ResolveLatest returns the latest stable version from Homebrew Cask.
// The returned VersionInfo includes Metadata with "url" and "checksum" fields.
func (p *CaskProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	meta, ok := hardcodedCaskMetadata[p.cask]
	if !ok {
		return nil, fmt.Errorf("cask %s not found (stub implementation only supports: iterm2)", p.cask)
	}

	return &VersionInfo{
		Tag:     meta.Version,
		Version: meta.Version,
		Metadata: map[string]string{
			"url":      meta.URL,
			"checksum": meta.Checksum,
		},
	}, nil
}

// ResolveVersion resolves a specific version for Homebrew Casks.
// For the walking skeleton, this only supports the hardcoded version.
func (p *CaskProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	meta, ok := hardcodedCaskMetadata[p.cask]
	if !ok {
		return nil, fmt.Errorf("cask %s not found (stub implementation only supports: iterm2)", p.cask)
	}

	// For stub, only support exact match on the hardcoded version
	if version != meta.Version {
		return nil, fmt.Errorf("version %s not found for cask %s (stub only supports %s)", version, p.cask, meta.Version)
	}

	return &VersionInfo{
		Tag:     meta.Version,
		Version: meta.Version,
		Metadata: map[string]string{
			"url":      meta.URL,
			"checksum": meta.Checksum,
		},
	}, nil
}

// SourceDescription returns a human-readable source description
func (p *CaskProvider) SourceDescription() string {
	return fmt.Sprintf("Cask:%s", p.cask)
}
