package version

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// FossilTimelineProvider resolves versions from Fossil SCM repository timelines.
// It parses the HTML timeline page to extract release tags.
//
// Example timeline URL: https://sqlite.org/src/timeline?t=release&n=all&y=ci
// Tag format: version-3.46.0, core-9-0-0 (with version_separator)
type FossilTimelineProvider struct {
	resolver         *Resolver
	repo             string // Full URL to Fossil repo (e.g., "https://sqlite.org/src")
	projectName      string // Project name for tarball (e.g., "sqlite")
	tagPrefix        string // Prefix before version in tags (default: "version-")
	versionSeparator string // Separator in version numbers (default: ".")
	timelineTag      string // Tag filter for timeline URL (default: "release")
}

// NewFossilTimelineProvider creates a provider for Fossil-hosted projects.
func NewFossilTimelineProvider(resolver *Resolver, repo, projectName string) *FossilTimelineProvider {
	return &FossilTimelineProvider{
		resolver:         resolver,
		repo:             repo,
		projectName:      projectName,
		tagPrefix:        "version-",
		versionSeparator: ".",
		timelineTag:      "release",
	}
}

// NewFossilTimelineProviderWithOptions creates a provider with custom tag format options.
func NewFossilTimelineProviderWithOptions(resolver *Resolver, repo, projectName, tagPrefix, versionSeparator, timelineTag string) *FossilTimelineProvider {
	p := NewFossilTimelineProvider(resolver, repo, projectName)
	if tagPrefix != "" {
		p.tagPrefix = tagPrefix
	}
	if versionSeparator != "" {
		p.versionSeparator = versionSeparator
	}
	if timelineTag != "" {
		p.timelineTag = timelineTag
	}
	return p
}

// maxFossilResponseSize limits the response size to prevent decompression bombs.
const maxFossilResponseSize = 10 * 1024 * 1024 // 10MB

// ListVersions returns all available versions from the Fossil timeline (newest first).
func (p *FossilTimelineProvider) ListVersions(ctx context.Context) ([]string, error) {
	timelineURL := p.buildTimelineURL()

	body, err := p.fetchTimeline(ctx, timelineURL)
	if err != nil {
		return nil, err
	}

	tags := p.parseTimelineTags(body)
	if len(tags) == 0 {
		return nil, fmt.Errorf("no version tags found in Fossil timeline at %s", timelineURL)
	}

	// Convert tags to versions
	var versions []string
	for _, tag := range tags {
		version := p.tagToVersion(tag)
		if version != "" {
			versions = append(versions, version)
		}
	}

	return versions, nil
}

// fetchTimeline fetches the Fossil timeline HTML.
func (p *FossilTimelineProvider) fetchTimeline(ctx context.Context, timelineURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", timelineURL, nil)
	if err != nil {
		return "", &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "fossil",
			Message: "failed to create request",
			Err:     err,
		}
	}

	resp, err := p.resolver.httpClient.Do(req)
	if err != nil {
		return "", WrapNetworkError(err, "fossil", "failed to fetch timeline")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "fossil",
			Message: fmt.Sprintf("Fossil repository not found at %s", p.repo),
		}
	}

	if resp.StatusCode != 200 {
		return "", &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "fossil",
			Message: fmt.Sprintf("unexpected status code: %d", resp.StatusCode),
		}
	}

	// Read body with size limit
	limitedReader := io.LimitReader(resp.Body, maxFossilResponseSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "fossil",
			Message: "failed to read response body",
			Err:     err,
		}
	}

	return string(body), nil
}

// ResolveLatest returns the latest stable version from the Fossil timeline.
func (p *FossilTimelineProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	versions, err := p.ListVersions(ctx)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions found in Fossil timeline")
	}

	// Find the first stable version
	for _, v := range versions {
		if isStableVersion(v) {
			return &VersionInfo{
				Version: v,
				Tag:     p.versionToTag(v),
			}, nil
		}
	}

	// Fallback to first version if no stable version found
	return &VersionInfo{
		Version: versions[0],
		Tag:     p.versionToTag(versions[0]),
	}, nil
}

// ResolveVersion resolves a specific version constraint.
// Handles fuzzy matching (e.g., "3.46" -> "3.46.0").
func (p *FossilTimelineProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	versions, err := p.ListVersions(ctx)
	if err != nil {
		return nil, err
	}

	// Try exact match first
	for _, v := range versions {
		if v == version {
			return &VersionInfo{
				Version: v,
				Tag:     p.versionToTag(v),
			}, nil
		}
	}

	// Try prefix match (fuzzy)
	for _, v := range versions {
		if strings.HasPrefix(v, version) {
			return &VersionInfo{
				Version: v,
				Tag:     p.versionToTag(v),
			}, nil
		}
	}

	return nil, fmt.Errorf("version %q not found in Fossil timeline at %s", version, p.repo)
}

// SourceDescription returns a human-readable source description.
func (p *FossilTimelineProvider) SourceDescription() string {
	return fmt.Sprintf("Fossil:%s", p.repo)
}

// TarballURL returns the download URL for a specific version.
func (p *FossilTimelineProvider) TarballURL(version string) string {
	tag := p.versionToTag(version)
	return fmt.Sprintf("%s/tarball/%s/%s.tar.gz", p.repo, tag, p.projectName)
}

// buildTimelineURL constructs the timeline URL for fetching releases.
func (p *FossilTimelineProvider) buildTimelineURL() string {
	return fmt.Sprintf("%s/timeline?t=%s&n=all&y=ci", p.repo, p.timelineTag)
}

// parseTimelineTags extracts version tags from Fossil timeline HTML.
// Tags appear in format: "tags: release, branch-3.51, version-3.51.1"
func (p *FossilTimelineProvider) parseTimelineTags(html string) []string {
	// Build regex for the specific tag prefix
	// Escape the prefix for regex safety
	escapedPrefix := regexp.QuoteMeta(p.tagPrefix)

	// Version pattern depends on separator
	var versionPattern string
	if p.versionSeparator == "-" {
		versionPattern = `[\d]+(?:-[\d]+)*` // e.g., 9-0-0
	} else {
		versionPattern = `[\d]+(?:\.[\d]+)*` // e.g., 3.46.0
	}

	pattern := fmt.Sprintf(`(?:^|,\s*)(%s%s)`, escapedPrefix, versionPattern)
	re := regexp.MustCompile(pattern)

	var tags []string
	seen := make(map[string]bool)

	// Find all matches
	matches := re.FindAllStringSubmatch(html, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			tag := match[1]
			if !seen[tag] {
				tags = append(tags, tag)
				seen[tag] = true
			}
		}
	}

	return tags
}

// tagToVersion converts a Fossil tag to a version string.
// Example: "version-3.46.0" -> "3.46.0"
// Example: "core-9-0-0" with versionSeparator="-" -> "9.0.0"
func (p *FossilTimelineProvider) tagToVersion(tag string) string {
	if !strings.HasPrefix(tag, p.tagPrefix) {
		return ""
	}

	version := strings.TrimPrefix(tag, p.tagPrefix)

	// If version separator is not ".", convert back to dots
	if p.versionSeparator != "." && p.versionSeparator != "" {
		version = strings.ReplaceAll(version, p.versionSeparator, ".")
	}

	return version
}

// versionToTag converts a version string to a Fossil tag.
// Example: "3.46.0" -> "version-3.46.0"
// Example: "9.0.0" with versionSeparator="-" -> "core-9-0-0"
func (p *FossilTimelineProvider) versionToTag(version string) string {
	// If version separator is not ".", convert dots to separator
	tagVersion := version
	if p.versionSeparator != "." && p.versionSeparator != "" {
		tagVersion = strings.ReplaceAll(version, ".", p.versionSeparator)
	}

	return p.tagPrefix + tagVersion
}
