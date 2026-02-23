package builders

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// makeGemTar creates an in-memory tar archive (simulating a .gem file)
// containing a metadata.gz entry built from the given YAML content.
// If yamlContent is empty, no metadata.gz is included.
func makeGemTar(yamlContent string) []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	if yamlContent != "" {
		// Compress the YAML into gzip
		var gzBuf bytes.Buffer
		gzw := gzip.NewWriter(&gzBuf)
		_, _ = gzw.Write([]byte(yamlContent))
		_ = gzw.Close()

		gzData := gzBuf.Bytes()

		// Write metadata.gz entry
		_ = tw.WriteHeader(&tar.Header{
			Name: "metadata.gz",
			Size: int64(len(gzData)),
		})
		_, _ = tw.Write(gzData)
	}

	// Write a dummy data.tar.gz entry to make it look like a real .gem
	dummyData := []byte("dummy data")
	_ = tw.WriteHeader(&tar.Header{
		Name: "data.tar.gz",
		Size: int64(len(dummyData)),
	})
	_, _ = tw.Write(dummyData)

	_ = tw.Close()
	return buf.Bytes()
}

// newGemBuilderWithTLSServer creates a GemBuilder pointing at a TLS test
// server. The builder uses the server's TLS client so self-signed certs work.
func newGemBuilderWithTLSServer(server *httptest.Server) *GemBuilder {
	return &GemBuilder{
		httpClient:      server.Client(),
		rubyGemsBaseURL: server.URL,
	}
}

// TestGemBuilder_Build_BundlerFromArtifact verifies that the bundler gem
// produces ["bundle", "bundler"] executables from metadata.gz.
func TestGemBuilder_Build_BundlerFromArtifact(t *testing.T) {
	gemData := makeGemTar(`--- !ruby/object:Gem::Specification
name: bundler
version: !ruby/object:Gem::Version
  version: 2.5.4
executables:
- bundle
- bundler
bindir: exe
`)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/gems/bundler.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "bundler",
				"version": "2.5.4",
				"info": "Bundler manages Ruby application dependencies",
				"homepage_uri": "https://bundler.io",
				"source_code_uri": "https://github.com/rubygems/rubygems",
				"downloads": 900000000
			}`))
		case "/gems/bundler-2.5.4.gem":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(gemData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := newGemBuilderWithTLSServer(server)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "bundler"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}

	sort.Strings(executables)
	if len(executables) != 2 {
		t.Fatalf("expected 2 executables, got %d: %v", len(executables), executables)
	}
	if executables[0] != "bundle" || executables[1] != "bundler" {
		t.Errorf("executables = %v, want [bundle, bundler]", executables)
	}
}

// TestGemBuilder_Build_GemDownloadFails_FallsBackToGemspec verifies that
// when .gem download fails, the builder falls back to gemspec-from-GitHub.
// Since the test server won't serve the gemspec either, it ultimately falls
// back to gem name.
func TestGemBuilder_Build_GemDownloadFails_FallsBackToGemspec(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/gems/fallback-tool.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "fallback-tool",
				"version": "1.0.0",
				"info": "A tool that falls back",
				"homepage_uri": "",
				"source_code_uri": "https://github.com/owner/fallback-tool",
				"downloads": 1000
			}`))
		case "/gems/fallback-tool-1.0.0.gem":
			// Simulate download failure
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGemBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "fallback-tool"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Should fall back to gem name since both artifact and gemspec fail
	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "fallback-tool" {
		t.Errorf("executables = %v, want [fallback-tool]", executables)
	}

	// Should have warning about gem download failure
	hasDownloadWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "Gem download failed") {
			hasDownloadWarning = true
			break
		}
	}
	if !hasDownloadWarning {
		t.Errorf("expected gem download failure warning, got: %v", result.Warnings)
	}
}

// TestGemBuilder_Build_NoMetadataGZ_FallsBack verifies that when the .gem
// tar archive has no metadata.gz entry, discovery falls back gracefully.
func TestGemBuilder_Build_NoMetadataGZ_FallsBack(t *testing.T) {
	// Create a .gem tar with no metadata.gz
	gemData := makeGemTar("")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/gems/no-metadata.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "no-metadata",
				"version": "1.0.0",
				"info": "A gem without metadata.gz",
				"homepage_uri": "",
				"source_code_uri": "",
				"downloads": 500
			}`))
		case "/gems/no-metadata-1.0.0.gem":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(gemData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := newGemBuilderWithTLSServer(server)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "no-metadata"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Should fall back to gem name
	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "no-metadata" {
		t.Errorf("executables = %v, want [no-metadata]", executables)
	}

	// Should have warning about missing metadata.gz
	hasMetadataWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "metadata.gz") {
			hasMetadataWarning = true
			break
		}
	}
	if !hasMetadataWarning {
		t.Errorf("expected metadata.gz warning, got: %v", result.Warnings)
	}
}

// TestGemBuilder_Build_ShellMetacharactersFiltered verifies that executable
// names with shell metacharacters are filtered out by isValidExecutableName().
func TestGemBuilder_Build_ShellMetacharactersFiltered(t *testing.T) {
	gemData := makeGemTar(`--- !ruby/object:Gem::Specification
name: bad-names
version: !ruby/object:Gem::Version
  version: 1.0.0
executables:
- good-tool
- $(evil)
- bad;name
- also-good
- name with spaces
`)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/gems/bad-names.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "bad-names",
				"version": "1.0.0",
				"info": "Test",
				"homepage_uri": "",
				"source_code_uri": "",
				"downloads": 100
			}`))
		case "/gems/bad-names-1.0.0.gem":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(gemData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := newGemBuilderWithTLSServer(server)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "bad-names"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}

	// Only valid names should pass through
	if len(executables) != 2 {
		t.Fatalf("expected 2 valid executables, got %d: %v", len(executables), executables)
	}
	if executables[0] != "good-tool" || executables[1] != "also-good" {
		t.Errorf("executables = %v, want [good-tool, also-good]", executables)
	}
}

// TestGemBuilder_AuthoritativeBinaryNames_AfterBuild verifies that
// AuthoritativeBinaryNames returns artifact-discovered executables.
func TestGemBuilder_AuthoritativeBinaryNames_AfterBuild(t *testing.T) {
	gemData := makeGemTar(`--- !ruby/object:Gem::Specification
name: bundler
version: !ruby/object:Gem::Version
  version: 2.5.4
executables:
- bundle
- bundler
`)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/gems/bundler.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "bundler",
				"version": "2.5.4",
				"info": "Bundler",
				"homepage_uri": "",
				"source_code_uri": "",
				"downloads": 1000
			}`))
		case "/gems/bundler-2.5.4.gem":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(gemData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := newGemBuilderWithTLSServer(server)

	// Before Build, should return nil
	if names := builder.AuthoritativeBinaryNames(); names != nil {
		t.Errorf("before Build(), AuthoritativeBinaryNames() = %v, want nil", names)
	}

	_, err := builder.Build(context.Background(), BuildRequest{Package: "bundler"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	names := builder.AuthoritativeBinaryNames()
	if len(names) != 2 {
		t.Fatalf("AuthoritativeBinaryNames() returned %d names, want 2: %v", len(names), names)
	}
	sort.Strings(names)
	if names[0] != "bundle" || names[1] != "bundler" {
		t.Errorf("AuthoritativeBinaryNames() = %v, want [bundle, bundler]", names)
	}
}

// TestGemBuilder_AuthoritativeBinaryNames_FallbackReturnsNil verifies that
// when artifact discovery fails and gemspec fallback is used,
// AuthoritativeBinaryNames returns nil.
func TestGemBuilder_AuthoritativeBinaryNames_FallbackReturnsNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/gems/no-artifact.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "no-artifact",
				"version": "1.0.0",
				"info": "Test",
				"homepage_uri": "",
				"source_code_uri": "",
				"downloads": 100
			}`))
		case "/gems/no-artifact-1.0.0.gem":
			// Simulate download failure
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGemBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Build(context.Background(), BuildRequest{Package: "no-artifact"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	names := builder.AuthoritativeBinaryNames()
	if names != nil {
		t.Errorf("AuthoritativeBinaryNames() = %v, want nil (fallback was used)", names)
	}
}

// TestGemBuilder_ImplementsBinaryNameProvider verifies interface compliance.
func TestGemBuilder_ImplementsBinaryNameProvider(t *testing.T) {
	var _ BinaryNameProvider = (*GemBuilder)(nil)
}

// TestParseGemMetadataExecutables tests the YAML metadata parser directly.
func TestParseGemMetadataExecutables(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want []string
	}{
		{
			name: "standard executables list",
			yaml: `--- !ruby/object:Gem::Specification
name: bundler
executables:
- bundle
- bundler
bindir: exe
`,
			want: []string{"bundle", "bundler"},
		},
		{
			name: "single executable",
			yaml: `--- !ruby/object:Gem::Specification
name: jekyll
executables:
- jekyll
`,
			want: []string{"jekyll"},
		},
		{
			name: "no executables section",
			yaml: `--- !ruby/object:Gem::Specification
name: some-lib
version: !ruby/object:Gem::Version
  version: 1.0.0
`,
			want: nil,
		},
		{
			name: "empty executables list",
			yaml: `--- !ruby/object:Gem::Specification
name: empty
executables: []
`,
			want: nil,
		},
		{
			name: "inline single value",
			yaml: `--- !ruby/object:Gem::Specification
name: inline
executables: my-tool
`,
			want: []string{"my-tool"},
		},
		{
			name: "filters invalid names",
			yaml: `--- !ruby/object:Gem::Specification
name: mixed
executables:
- good-tool
- $(evil)
- valid_name
`,
			want: []string{"good-tool", "valid_name"},
		},
		{
			name: "executables between other fields",
			yaml: `--- !ruby/object:Gem::Specification
name: between
version: 1.0.0
executables:
- my-bin
bindir: exe
homepage: https://example.com
`,
			want: []string{"my-bin"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseGemMetadataExecutables([]byte(tc.yaml))
			if len(got) != len(tc.want) {
				t.Fatalf("parseGemMetadataExecutables() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("parseGemMetadataExecutables()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestExtractTarEntry tests the tar entry extraction helper.
func TestExtractTarEntry(t *testing.T) {
	t.Run("extracts existing entry", func(t *testing.T) {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)

		content := []byte("hello world")
		_ = tw.WriteHeader(&tar.Header{
			Name: "test.txt",
			Size: int64(len(content)),
		})
		_, _ = tw.Write(content)
		_ = tw.Close()

		got, err := extractTarEntry(buf.Bytes(), "test.txt")
		if err != nil {
			t.Fatalf("extractTarEntry() error = %v", err)
		}
		if string(got) != "hello world" {
			t.Errorf("extractTarEntry() = %q, want %q", string(got), "hello world")
		}
	})

	t.Run("returns error for missing entry", func(t *testing.T) {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		content := []byte("data")
		_ = tw.WriteHeader(&tar.Header{
			Name: "other.txt",
			Size: int64(len(content)),
		})
		_, _ = tw.Write(content)
		_ = tw.Close()

		_, err := extractTarEntry(buf.Bytes(), "missing.txt")
		if err == nil {
			t.Fatal("expected error for missing entry")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("error = %q, want 'not found' message", err.Error())
		}
	})
}

// TestDecompressGzip tests the gzip decompression helper.
func TestDecompressGzip(t *testing.T) {
	t.Run("decompresses valid data", func(t *testing.T) {
		var buf bytes.Buffer
		gzw := gzip.NewWriter(&buf)
		_, _ = gzw.Write([]byte("test content"))
		_ = gzw.Close()

		got, err := decompressGzip(buf.Bytes(), 1024)
		if err != nil {
			t.Fatalf("decompressGzip() error = %v", err)
		}
		if string(got) != "test content" {
			t.Errorf("decompressGzip() = %q, want %q", string(got), "test content")
		}
	})

	t.Run("rejects oversized data", func(t *testing.T) {
		var buf bytes.Buffer
		gzw := gzip.NewWriter(&buf)
		_, _ = gzw.Write(make([]byte, 1024))
		_ = gzw.Close()

		_, err := decompressGzip(buf.Bytes(), 100)
		if err == nil {
			t.Fatal("expected error for oversized data")
		}
		if !strings.Contains(err.Error(), "exceeds maximum size") {
			t.Errorf("error = %q, want 'exceeds maximum size' message", err.Error())
		}
	})

	t.Run("rejects invalid gzip", func(t *testing.T) {
		_, err := decompressGzip([]byte("not gzip data"), 1024)
		if err == nil {
			t.Fatal("expected error for invalid gzip")
		}
	})
}

// TestGemBuilder_Build_NoVersion_FallsBack verifies that when the gem info
// has no version field, artifact discovery is skipped and falls back.
func TestGemBuilder_Build_NoVersion_FallsBack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/gems/no-version.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "no-version",
				"info": "Test",
				"homepage_uri": "",
				"source_code_uri": "",
				"downloads": 100
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGemBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "no-version"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Should fall back to gem name
	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "no-version" {
		t.Errorf("executables = %v, want [no-version]", executables)
	}

	// Should have warning about no version
	hasVersionWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "No version") {
			hasVersionWarning = true
			break
		}
	}
	if !hasVersionWarning {
		t.Errorf("expected version warning, got: %v", result.Warnings)
	}
}

// TestGemBuilder_Build_SingleExecutableGem verifies discovery for a gem with
// a single executable that differs from the gem name.
func TestGemBuilder_Build_SingleExecutableGem(t *testing.T) {
	gemData := makeGemTar(`--- !ruby/object:Gem::Specification
name: fpm
version: !ruby/object:Gem::Version
  version: 1.15.1
executables:
- fpm
`)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/gems/fpm.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "fpm",
				"version": "1.15.1",
				"info": "FPM - package builder",
				"homepage_uri": "",
				"source_code_uri": "https://github.com/jordansissel/fpm",
				"downloads": 8000000
			}`))
		case "/gems/fpm-1.15.1.gem":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(gemData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := newGemBuilderWithTLSServer(server)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "fpm"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "fpm" {
		t.Errorf("executables = %v, want [fpm]", executables)
	}

	// AuthoritativeBinaryNames should also return from artifact
	names := builder.AuthoritativeBinaryNames()
	if len(names) != 1 || names[0] != "fpm" {
		t.Errorf("AuthoritativeBinaryNames() = %v, want [fpm]", names)
	}
}
