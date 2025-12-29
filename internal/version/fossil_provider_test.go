package version

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Sample Fossil timeline HTML response for testing
const sampleFossilTimelineHTML = `
<!DOCTYPE html>
<html>
<head><title>SQLite: Timeline</title></head>
<body>
<table id="timelineTable0" class="timelineTable">
<tr class="timelineDateRow"><td>
  <div class="divider timelineDate">2025-11-28</div>
</td><td></td><td></td></tr>
<tr>
<td class="timelineTime">17:28</a></td>
<td class="timelineGraph"><div id="m1" class="tl-nodemark"></div></td>
<td class="timelineSimpleCell" id='mc1'>
<span class='timelineSimpleComment'>Version 3.51.1</span>
<span class='timelineSimpleDetail'>(check-in: 281fc0e9af
<span class='clutter' id='detail-126668'>
user: drh tags: release, branch-3.51, version-3.51.1</span>)</span>
</td></tr>
<tr>
<td class="timelineTime">19:38</a></td>
<td class="timelineGraph"><div id="m2" class="tl-nodemark"></div></td>
<td class="timelineSimpleCell" id='mc2'>
<span class='timelineSimpleComment'>Version 3.51.0</span>
<span class='timelineSimpleDetail'>(check-in: fb2c931ae5
<span class='clutter' id='detail-125759'>
user: drh tags: trunk, release, major-release, version-3.51.0</span>)</span>
</td></tr>
<tr>
<td class="timelineTime">18:50</a></td>
<td class="timelineGraph"><div id="m3" class="tl-nodemark"></div></td>
<td class="timelineSimpleCell" id='mc3'>
<span class='timelineSimpleComment'>Version 3.50.4</span>
<span class='timelineSimpleDetail'>(check-in: 4d8adfb30e
<span class='clutter' id='detail-124014'>
user: drh tags: release, branch-3.50, version-3.50.4</span>)</span>
</td></tr>
</table>
</body>
</html>
`

// Sample Fossil timeline HTML for Tcl (uses core- prefix with dash separators)
const sampleTclTimelineHTML = `
<!DOCTYPE html>
<html>
<body>
<table id="timelineTable0" class="timelineTable">
<tr>
<td class="timelineSimpleCell">
<span class='clutter'>tags: release, core-9-0-1</span>
</td></tr>
<tr>
<td class="timelineSimpleCell">
<span class='clutter'>tags: release, core-9-0-0</span>
</td></tr>
<tr>
<td class="timelineSimpleCell">
<span class='clutter'>tags: release, core-8-6-15</span>
</td></tr>
</table>
</body>
</html>
`

func TestFossilTimelineProvider_ParseTimelineTags(t *testing.T) {
	tests := []struct {
		name             string
		tagPrefix        string
		versionSeparator string
		html             string
		expectedTags     []string
	}{
		{
			name:             "SQLite version tags",
			tagPrefix:        "version-",
			versionSeparator: ".",
			html:             sampleFossilTimelineHTML,
			expectedTags:     []string{"version-3.51.1", "version-3.51.0", "version-3.50.4"},
		},
		{
			name:             "Tcl core tags with dash separator",
			tagPrefix:        "core-",
			versionSeparator: "-",
			html:             sampleTclTimelineHTML,
			expectedTags:     []string{"core-9-0-1", "core-9-0-0", "core-8-6-15"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewFossilTimelineProviderWithOptions(nil, "https://example.com", "test", tt.tagPrefix, tt.versionSeparator, "release")
			tags := p.parseTimelineTags(tt.html)

			if len(tags) != len(tt.expectedTags) {
				t.Errorf("got %d tags, want %d", len(tags), len(tt.expectedTags))
				t.Errorf("got: %v", tags)
				return
			}

			for i, expected := range tt.expectedTags {
				if tags[i] != expected {
					t.Errorf("tag[%d] = %q, want %q", i, tags[i], expected)
				}
			}
		})
	}
}

func TestFossilTimelineProvider_TagVersionConversion(t *testing.T) {
	tests := []struct {
		name             string
		tagPrefix        string
		versionSeparator string
		tag              string
		expectedVersion  string
	}{
		{
			name:             "SQLite tag to version",
			tagPrefix:        "version-",
			versionSeparator: ".",
			tag:              "version-3.51.1",
			expectedVersion:  "3.51.1",
		},
		{
			name:             "Tcl tag to version (dash to dot conversion)",
			tagPrefix:        "core-",
			versionSeparator: "-",
			tag:              "core-9-0-0",
			expectedVersion:  "9.0.0",
		},
		{
			name:             "Invalid prefix",
			tagPrefix:        "version-",
			versionSeparator: ".",
			tag:              "core-9-0-0",
			expectedVersion:  "", // No match
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewFossilTimelineProviderWithOptions(nil, "https://example.com", "test", tt.tagPrefix, tt.versionSeparator, "release")
			version := p.tagToVersion(tt.tag)

			if version != tt.expectedVersion {
				t.Errorf("tagToVersion(%q) = %q, want %q", tt.tag, version, tt.expectedVersion)
			}
		})
	}
}

func TestFossilTimelineProvider_VersionToTag(t *testing.T) {
	tests := []struct {
		name             string
		tagPrefix        string
		versionSeparator string
		version          string
		expectedTag      string
	}{
		{
			name:             "SQLite version to tag",
			tagPrefix:        "version-",
			versionSeparator: ".",
			version:          "3.51.1",
			expectedTag:      "version-3.51.1",
		},
		{
			name:             "Tcl version to tag (dot to dash conversion)",
			tagPrefix:        "core-",
			versionSeparator: "-",
			version:          "9.0.0",
			expectedTag:      "core-9-0-0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewFossilTimelineProviderWithOptions(nil, "https://example.com", "test", tt.tagPrefix, tt.versionSeparator, "release")
			tag := p.versionToTag(tt.version)

			if tag != tt.expectedTag {
				t.Errorf("versionToTag(%q) = %q, want %q", tt.version, tag, tt.expectedTag)
			}
		})
	}
}

func TestFossilTimelineProvider_TarballURL(t *testing.T) {
	tests := []struct {
		name             string
		repo             string
		projectName      string
		tagPrefix        string
		versionSeparator string
		version          string
		expectedURL      string
	}{
		{
			name:             "SQLite tarball URL",
			repo:             "https://sqlite.org/src",
			projectName:      "sqlite",
			tagPrefix:        "version-",
			versionSeparator: ".",
			version:          "3.51.1",
			expectedURL:      "https://sqlite.org/src/tarball/version-3.51.1/sqlite.tar.gz",
		},
		{
			name:             "Tcl tarball URL",
			repo:             "https://core.tcl-lang.org/tcl",
			projectName:      "tcl",
			tagPrefix:        "core-",
			versionSeparator: "-",
			version:          "9.0.0",
			expectedURL:      "https://core.tcl-lang.org/tcl/tarball/core-9-0-0/tcl.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewFossilTimelineProviderWithOptions(nil, tt.repo, tt.projectName, tt.tagPrefix, tt.versionSeparator, "release")
			url := p.TarballURL(tt.version)

			if url != tt.expectedURL {
				t.Errorf("TarballURL(%q) = %q, want %q", tt.version, url, tt.expectedURL)
			}
		})
	}
}

func TestFossilTimelineProvider_BuildTimelineURL(t *testing.T) {
	tests := []struct {
		name        string
		repo        string
		timelineTag string
		expectedURL string
	}{
		{
			name:        "Default release tag",
			repo:        "https://sqlite.org/src",
			timelineTag: "release",
			expectedURL: "https://sqlite.org/src/timeline?t=release&n=all&y=ci",
		},
		{
			name:        "Custom timeline tag",
			repo:        "https://www.gaia-gis.it/fossil/libspatialite",
			timelineTag: "version",
			expectedURL: "https://www.gaia-gis.it/fossil/libspatialite/timeline?t=version&n=all&y=ci",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewFossilTimelineProviderWithOptions(nil, tt.repo, "test", "", "", tt.timelineTag)
			url := p.buildTimelineURL()

			if url != tt.expectedURL {
				t.Errorf("buildTimelineURL() = %q, want %q", url, tt.expectedURL)
			}
		})
	}
}

func TestFossilTimelineProvider_ListVersions(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/timeline" {
			_, _ = w.Write([]byte(sampleFossilTimelineHTML))
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	resolver := New()
	resolver.httpClient = server.Client()
	p := NewFossilTimelineProvider(resolver, server.URL, "sqlite")

	versions, err := p.ListVersions(context.Background())
	if err != nil {
		t.Fatalf("ListVersions failed: %v", err)
	}

	expectedVersions := []string{"3.51.1", "3.51.0", "3.50.4"}
	if len(versions) != len(expectedVersions) {
		t.Errorf("got %d versions, want %d", len(versions), len(expectedVersions))
		t.Errorf("got: %v", versions)
		return
	}

	for i, expected := range expectedVersions {
		if versions[i] != expected {
			t.Errorf("versions[%d] = %q, want %q", i, versions[i], expected)
		}
	}
}

func TestFossilTimelineProvider_ResolveLatest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/timeline" {
			_, _ = w.Write([]byte(sampleFossilTimelineHTML))
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	resolver := New()
	resolver.httpClient = server.Client()
	p := NewFossilTimelineProvider(resolver, server.URL, "sqlite")

	info, err := p.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest failed: %v", err)
	}

	if info.Version != "3.51.1" {
		t.Errorf("Version = %q, want %q", info.Version, "3.51.1")
	}

	if info.Tag != "version-3.51.1" {
		t.Errorf("Tag = %q, want %q", info.Tag, "version-3.51.1")
	}
}

func TestFossilTimelineProvider_ResolveVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/timeline" {
			_, _ = w.Write([]byte(sampleFossilTimelineHTML))
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	resolver := New()
	resolver.httpClient = server.Client()
	p := NewFossilTimelineProvider(resolver, server.URL, "sqlite")

	tests := []struct {
		name          string
		version       string
		expectVersion string
		expectTag     string
		expectError   bool
	}{
		{
			name:          "Exact match",
			version:       "3.51.0",
			expectVersion: "3.51.0",
			expectTag:     "version-3.51.0",
		},
		{
			name:          "Prefix match",
			version:       "3.51",
			expectVersion: "3.51.1",
			expectTag:     "version-3.51.1",
		},
		{
			name:        "Not found",
			version:     "99.0.0",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := p.ResolveVersion(context.Background(), tt.version)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if info.Version != tt.expectVersion {
				t.Errorf("Version = %q, want %q", info.Version, tt.expectVersion)
			}

			if info.Tag != tt.expectTag {
				t.Errorf("Tag = %q, want %q", info.Tag, tt.expectTag)
			}
		})
	}
}

func TestFossilTimelineProvider_SourceDescription(t *testing.T) {
	p := NewFossilTimelineProvider(nil, "https://sqlite.org/src", "sqlite")
	desc := p.SourceDescription()

	expected := "Fossil:https://sqlite.org/src"
	if desc != expected {
		t.Errorf("SourceDescription() = %q, want %q", desc, expected)
	}
}
