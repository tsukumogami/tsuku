package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxHTTPJSONResponseSize caps the response body to prevent decompression
// bombs. 5 MB is comfortably larger than any of the v1 consumer manifests
// (gcloud's components-2.json is ~700 KB; HashiCorp checkpoint and
// Adoptium responses are well under 100 KB).
const maxHTTPJSONResponseSize = 5 * 1024 * 1024

// HTTPJSONProvider resolves the latest version by fetching a JSON
// document over HTTPS and extracting a single field via a small path
// syntax. See parseVersionPath for the supported path grammar.
type HTTPJSONProvider struct {
	resolver    *Resolver
	url         string
	versionPath string // raw path, kept for SourceDescription
	pathSteps   []pathStep
}

// NewHTTPJSONProvider parses the path expression and returns a provider.
// The URL itself is not validated for protocol here; that is enforced by
// the recipe validator at strict-validate time.
func NewHTTPJSONProvider(resolver *Resolver, url, versionPath string) (*HTTPJSONProvider, error) {
	steps, err := parseVersionPath(versionPath)
	if err != nil {
		return nil, fmt.Errorf("invalid version_path %q: %w", versionPath, err)
	}
	return &HTTPJSONProvider{
		resolver:    resolver,
		url:         url,
		versionPath: versionPath,
		pathSteps:   steps,
	}, nil
}

// ResolveLatest fetches the manifest, parses it, walks the configured
// path, and returns a VersionInfo carrying the leaf as both Version
// and Tag.
func (p *HTTPJSONProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", p.url, err)
	}

	resp, err := p.resolver.httpClient.Do(req)
	if err != nil {
		// Same network-error shaping as ResolveNodeJS / ResolveFossil.
		if strings.Contains(err.Error(), "network is unreachable") ||
			strings.Contains(err.Error(), "no such host") ||
			strings.Contains(err.Error(), "dial tcp") {
			return nil, fmt.Errorf("network unavailable while fetching %s: %w", p.url, err)
		}
		return nil, fmt.Errorf("failed to fetch %s: %w", p.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned status %d", p.url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPJSONResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response from %s: %w", p.url, err)
	}
	if len(body) > maxHTTPJSONResponseSize {
		return nil, fmt.Errorf("response from %s exceeds %d-byte cap", p.url, maxHTTPJSONResponseSize)
	}

	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("failed to parse JSON from %s: %w", p.url, err)
	}

	leaf, err := walkPath(root, p.pathSteps)
	if err != nil {
		return nil, fmt.Errorf("version_path %q failed against %s: %w", p.versionPath, p.url, err)
	}

	version, err := stringifyLeaf(leaf)
	if err != nil {
		return nil, fmt.Errorf("version_path %q at %s: %w", p.versionPath, p.url, err)
	}
	if version == "" {
		return nil, fmt.Errorf("version_path %q at %s resolved to empty string", p.versionPath, p.url)
	}

	return &VersionInfo{
		Version: version,
		Tag:     version,
	}, nil
}

// ResolveVersion accepts the user-provided version string as-is. The
// http_json source has no way to enumerate available versions (the
// manifest only exposes the latest), so any user-specified version is
// passed through to the download URL via {version} substitution. The
// download itself will fail if the version does not exist upstream.
func (p *HTTPJSONProvider) ResolveVersion(_ context.Context, version string) (*VersionInfo, error) {
	if version == "" {
		return nil, fmt.Errorf("http_json: empty version")
	}
	return &VersionInfo{
		Version: version,
		Tag:     version,
	}, nil
}

// SourceDescription returns a human-readable source description. Used
// in error messages and version-resolution logs.
func (p *HTTPJSONProvider) SourceDescription() string {
	return fmt.Sprintf("http_json:%s", p.url)
}
