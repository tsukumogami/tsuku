package builders

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// makeWheelZIP creates an in-memory ZIP archive (simulating a .whl file)
// with the given entry_points.txt content inside a dist-info directory.
func makeWheelZIP(distInfoDir, entryPointsContent string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	f, _ := w.Create(distInfoDir + "/entry_points.txt")
	_, _ = f.Write([]byte(entryPointsContent))

	// Add a dummy METADATA file to make it look like a real wheel
	m, _ := w.Create(distInfoDir + "/METADATA")
	_, _ = m.Write([]byte("Metadata-Version: 2.1\nName: test\n"))

	_ = w.Close()
	return buf.Bytes()
}

func sha256hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// newPyPIBuilderWithTLSServer creates a PyPIBuilder pointing at a TLS test
// server. The builder uses the server's TLS client so self-signed certs work.
func newPyPIBuilderWithTLSServer(server *httptest.Server) *PyPIBuilder {
	b := &PyPIBuilder{
		httpClient:  server.Client(),
		pypiBaseURL: server.URL,
		topPyPIURL:  server.URL + "/top-pypi-packages-30-days.min.json",
	}
	return b
}

// TestPyPIBuilder_Build_BlackFromWheel verifies that the black package
// produces "black" and "blackd" executables from wheel entry_points.txt.
func TestPyPIBuilder_Build_BlackFromWheel(t *testing.T) {
	wheelData := makeWheelZIP("black-24.0.0.dist-info", `[console_scripts]
black = black:patched_main
blackd = blackd:patched_main
`)
	wheelHash := sha256hex(wheelData)

	var serverURL string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pypi/black/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"info": {
					"name": "black",
					"summary": "The uncompromising code formatter",
					"home_page": "",
					"project_urls": {"Homepage": "https://github.com/psf/black"}
				},
				"releases": {},
				"urls": [
					{
						"packagetype": "bdist_wheel",
						"url": "%s/wheels/black-24.0.0-py3-none-any.whl",
						"filename": "black-24.0.0-py3-none-any.whl",
						"digests": {"sha256": "%s"}
					}
				]
			}`, serverURL, wheelHash)))
		case "/wheels/black-24.0.0-py3-none-any.whl":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(wheelData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	builder := newPyPIBuilderWithTLSServer(server)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "black"})
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
	if executables[0] != "black" || executables[1] != "blackd" {
		t.Errorf("executables = %v, want [black, blackd]", executables)
	}
}

// TestPyPIBuilder_Build_HttpieFromWheel verifies that httpie produces
// "http" and "https" executables from wheel entry_points.txt.
func TestPyPIBuilder_Build_HttpieFromWheel(t *testing.T) {
	wheelData := makeWheelZIP("httpie-3.2.2.dist-info", `[console_scripts]
http = httpie.__main__:main
https = httpie.__main__:main
`)
	wheelHash := sha256hex(wheelData)

	var serverURL string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pypi/httpie/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"info": {
					"name": "httpie",
					"summary": "HTTPie - a CLI HTTP client",
					"home_page": "",
					"project_urls": {"Repository": "https://github.com/httpie/cli"}
				},
				"releases": {},
				"urls": [
					{
						"packagetype": "bdist_wheel",
						"url": "%s/wheels/httpie-3.2.2-py3-none-any.whl",
						"filename": "httpie-3.2.2-py3-none-any.whl",
						"digests": {"sha256": "%s"}
					}
				]
			}`, serverURL, wheelHash)))
		case "/wheels/httpie-3.2.2-py3-none-any.whl":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(wheelData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	builder := newPyPIBuilderWithTLSServer(server)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "httpie"})
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
	if executables[0] != "http" || executables[1] != "https" {
		t.Errorf("executables = %v, want [http, https]", executables)
	}
}

// TestPyPIBuilder_Build_WheelNotAvailable_FallsBack verifies that when no
// wheel is in the urls array, discovery falls back to pyproject.toml.
func TestPyPIBuilder_Build_WheelNotAvailable_FallsBack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pypi/no-wheel-pkg/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"info": {
					"name": "no-wheel-pkg",
					"summary": "A package without wheels",
					"home_page": "",
					"project_urls": null
				},
				"releases": {},
				"urls": [
					{
						"packagetype": "sdist",
						"url": "https://example.com/no-wheel-pkg-1.0.tar.gz",
						"filename": "no-wheel-pkg-1.0.tar.gz",
						"digests": {"sha256": "abc123"}
					}
				]
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "no-wheel-pkg"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Should fall back to package name since no wheel and no source URL
	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "no-wheel-pkg" {
		t.Errorf("executables = %v, want [no-wheel-pkg]", executables)
	}

	// Should have warning about no wheel
	hasWheelWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "No wheel artifact") {
			hasWheelWarning = true
			break
		}
	}
	if !hasWheelWarning {
		t.Errorf("expected wheel-unavailable warning, got: %v", result.Warnings)
	}
}

// TestPyPIBuilder_Build_WheelDownloadExceedsSizeLimit_FallsBack verifies
// that exceeding the download size limit triggers a fallback.
func TestPyPIBuilder_Build_WheelDownloadExceedsSizeLimit_FallsBack(t *testing.T) {
	// Create a wheel larger than our limit for the test.
	largePayload := make([]byte, maxWheelDownloadSize+100)

	var serverURL string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pypi/big-wheel/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"info": {
					"name": "big-wheel",
					"summary": "Big package",
					"home_page": "",
					"project_urls": null
				},
				"releases": {},
				"urls": [
					{
						"packagetype": "bdist_wheel",
						"url": "%s/wheels/big-wheel-1.0-py3-none-any.whl",
						"filename": "big-wheel-1.0-py3-none-any.whl",
						"digests": {"sha256": ""}
					}
				]
			}`, serverURL)))
		case "/wheels/big-wheel-1.0-py3-none-any.whl":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(largePayload)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	builder := newPyPIBuilderWithTLSServer(server)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "big-wheel"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Should fall back to package name
	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "big-wheel" {
		t.Errorf("executables = %v, want [big-wheel]", executables)
	}

	// Should have warning about download failure
	hasDownloadWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "Wheel download failed") {
			hasDownloadWarning = true
			break
		}
	}
	if !hasDownloadWarning {
		t.Errorf("expected download failure warning, got: %v", result.Warnings)
	}
}

// TestPyPIBuilder_Build_WheelHashMismatch_FallsBack verifies that
// SHA256 hash mismatch on the wheel triggers a fallback.
func TestPyPIBuilder_Build_WheelHashMismatch_FallsBack(t *testing.T) {
	wheelData := makeWheelZIP("pkg-1.0.dist-info", `[console_scripts]
tool = pkg:main
`)

	var serverURL string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pypi/hash-mismatch/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"info": {
					"name": "hash-mismatch",
					"summary": "Test",
					"home_page": "",
					"project_urls": null
				},
				"releases": {},
				"urls": [
					{
						"packagetype": "bdist_wheel",
						"url": "%s/wheels/pkg-1.0-py3-none-any.whl",
						"filename": "pkg-1.0-py3-none-any.whl",
						"digests": {"sha256": "0000000000000000000000000000000000000000000000000000000000000000"}
					}
				]
			}`, serverURL)))
		case "/wheels/pkg-1.0-py3-none-any.whl":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(wheelData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	builder := newPyPIBuilderWithTLSServer(server)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "hash-mismatch"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Should fall back to package name
	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "hash-mismatch" {
		t.Errorf("executables = %v, want [hash-mismatch]", executables)
	}

	hasHashWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "Wheel download failed") && strings.Contains(w, "SHA256") {
			hasHashWarning = true
			break
		}
	}
	if !hasHashWarning {
		t.Errorf("expected SHA256 mismatch warning, got: %v", result.Warnings)
	}
}

// TestPyPIBuilder_Build_EntryPointsMissing_FallsBack verifies that when the
// wheel has no entry_points.txt, discovery falls back.
func TestPyPIBuilder_Build_EntryPointsMissing_FallsBack(t *testing.T) {
	// Create a wheel ZIP with no entry_points.txt
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("no_entry_points-1.0.dist-info/METADATA")
	_, _ = f.Write([]byte("Metadata-Version: 2.1\nName: no-entry-points\n"))
	_ = zw.Close()
	wheelData := buf.Bytes()
	wheelHash := sha256hex(wheelData)

	var serverURL string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pypi/no-entry-points/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"info": {
					"name": "no-entry-points",
					"summary": "Test",
					"home_page": "",
					"project_urls": null
				},
				"releases": {},
				"urls": [
					{
						"packagetype": "bdist_wheel",
						"url": "%s/wheels/no_entry_points-1.0-py3-none-any.whl",
						"filename": "no_entry_points-1.0-py3-none-any.whl",
						"digests": {"sha256": "%s"}
					}
				]
			}`, serverURL, wheelHash)))
		case "/wheels/no_entry_points-1.0-py3-none-any.whl":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(wheelData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	builder := newPyPIBuilderWithTLSServer(server)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "no-entry-points"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Should fall back
	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "no-entry-points" {
		t.Errorf("executables = %v, want [no-entry-points]", executables)
	}

	hasWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "entry_points.txt") {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Errorf("expected entry_points.txt warning, got: %v", result.Warnings)
	}
}

// TestPyPIBuilder_Build_NoConsoleScripts_FallsBack verifies that when
// entry_points.txt exists but has no [console_scripts] section, discovery falls back.
func TestPyPIBuilder_Build_NoConsoleScripts_FallsBack(t *testing.T) {
	wheelData := makeWheelZIP("gui_only-1.0.dist-info", `[gui_scripts]
gui_tool = pkg:gui_main
`)
	wheelHash := sha256hex(wheelData)

	var serverURL string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pypi/gui-only/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"info": {
					"name": "gui-only",
					"summary": "Test",
					"home_page": "",
					"project_urls": null
				},
				"releases": {},
				"urls": [
					{
						"packagetype": "bdist_wheel",
						"url": "%s/wheels/gui_only-1.0-py3-none-any.whl",
						"filename": "gui_only-1.0-py3-none-any.whl",
						"digests": {"sha256": "%s"}
					}
				]
			}`, serverURL, wheelHash)))
		case "/wheels/gui_only-1.0-py3-none-any.whl":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(wheelData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	builder := newPyPIBuilderWithTLSServer(server)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "gui-only"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "gui-only" {
		t.Errorf("executables = %v, want [gui-only]", executables)
	}

	hasWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "No console_scripts") {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Errorf("expected no console_scripts warning, got: %v", result.Warnings)
	}
}

// TestPyPIBuilder_Build_InvalidExecutableNamesFiltered verifies that
// invalid executable names from console_scripts are filtered out.
func TestPyPIBuilder_Build_InvalidExecutableNamesFiltered(t *testing.T) {
	wheelData := makeWheelZIP("mixed_names-1.0.dist-info", `[console_scripts]
good-tool = pkg:main
also-good = pkg:other
`)
	wheelHash := sha256hex(wheelData)

	var serverURL string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pypi/mixed-names/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"info": {
					"name": "mixed-names",
					"summary": "Test",
					"home_page": "",
					"project_urls": null
				},
				"releases": {},
				"urls": [
					{
						"packagetype": "bdist_wheel",
						"url": "%s/wheels/mixed_names-1.0-py3-none-any.whl",
						"filename": "mixed_names-1.0-py3-none-any.whl",
						"digests": {"sha256": "%s"}
					}
				]
			}`, serverURL, wheelHash)))
		case "/wheels/mixed_names-1.0-py3-none-any.whl":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(wheelData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	builder := newPyPIBuilderWithTLSServer(server)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "mixed-names"})
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

// TestPyPIBuilder_Build_PrefersPlatformIndependentWheel verifies that when
// multiple wheels are available, the builder prefers py3-none-any.
func TestPyPIBuilder_Build_PrefersPlatformIndependentWheel(t *testing.T) {
	// Platform-specific wheel with different console_scripts
	platformWheelData := makeWheelZIP("multi_wheel-1.0.dist-info", `[console_scripts]
platform-tool = pkg:platform_main
`)

	// Platform-independent wheel with correct console_scripts
	anyWheelData := makeWheelZIP("multi_wheel-1.0.dist-info", `[console_scripts]
correct-tool = pkg:main
`)
	anyWheelHash := sha256hex(anyWheelData)

	var serverURL string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pypi/multi-wheel/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"info": {
					"name": "multi-wheel",
					"summary": "Test",
					"home_page": "",
					"project_urls": null
				},
				"releases": {},
				"urls": [
					{
						"packagetype": "bdist_wheel",
						"url": "%s/wheels/multi_wheel-1.0-cp39-cp39-manylinux1_x86_64.whl",
						"filename": "multi_wheel-1.0-cp39-cp39-manylinux1_x86_64.whl",
						"digests": {"sha256": "%s"}
					},
					{
						"packagetype": "bdist_wheel",
						"url": "%s/wheels/multi_wheel-1.0-py3-none-any.whl",
						"filename": "multi_wheel-1.0-py3-none-any.whl",
						"digests": {"sha256": "%s"}
					}
				]
			}`, serverURL, sha256hex(platformWheelData), serverURL, anyWheelHash)))
		case "/wheels/multi_wheel-1.0-py3-none-any.whl":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(anyWheelData)
		case "/wheels/multi_wheel-1.0-cp39-cp39-manylinux1_x86_64.whl":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(platformWheelData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	builder := newPyPIBuilderWithTLSServer(server)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "multi-wheel"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}

	// Should use the platform-independent wheel's console_scripts
	if len(executables) != 1 || executables[0] != "correct-tool" {
		t.Errorf("executables = %v, want [correct-tool]", executables)
	}
}

// TestParseConsoleScripts tests the console_scripts parser directly.
func TestParseConsoleScripts(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name: "standard console_scripts",
			input: `[console_scripts]
black = black:patched_main
blackd = blackd:patched_main
`,
			want: []string{"black", "blackd"},
		},
		{
			name: "with extras in entry point",
			input: `[console_scripts]
tool = package.module:main [extra1]
`,
			want: []string{"tool"},
		},
		{
			name: "empty console_scripts section",
			input: `[console_scripts]
`,
			want: nil,
		},
		{
			name: "no console_scripts section",
			input: `[gui_scripts]
gui = pkg:gui_main
`,
			want: nil,
		},
		{
			name: "mixed sections",
			input: `[console_scripts]
cli = pkg:cli_main

[gui_scripts]
gui = pkg:gui_main
`,
			want: []string{"cli"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name: "skips comments",
			input: `[console_scripts]
# this is a comment
tool = pkg:main
`,
			want: []string{"tool"},
		},
		{
			name: "malformed line without equals",
			input: `[console_scripts]
not-a-valid-entry
tool = pkg:main
`,
			want: []string{"tool"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseConsoleScripts(strings.NewReader(tc.input))
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseConsoleScripts() error = %v, wantErr %v", err, tc.wantErr)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("parseConsoleScripts() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("parseConsoleScripts()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestNormalizePyPIName tests the PEP 503 package name normalization.
func TestNormalizePyPIName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"black", "black"},
		{"Flask", "flask"},
		{"my-package", "my_package"},
		{"my_package", "my_package"},
		{"my.package", "my_package"},
		{"My--Package", "my_package"},
		{"My._.-Package", "my_package"},
		{"HTTPie", "httpie"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizePyPIName(tc.input)
			if got != tc.want {
				t.Errorf("normalizePyPIName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestFindBestWheel tests the wheel selection logic.
func TestFindBestWheel(t *testing.T) {
	builder := NewPyPIBuilder(nil)

	t.Run("prefers py3-none-any", func(t *testing.T) {
		urls := []pypiURLEntry{
			{PackageType: "sdist", Filename: "pkg-1.0.tar.gz"},
			{PackageType: "bdist_wheel", Filename: "pkg-1.0-cp39-cp39-linux.whl", URL: "https://a"},
			{PackageType: "bdist_wheel", Filename: "pkg-1.0-py3-none-any.whl", URL: "https://b"},
		}
		got := builder.findBestWheel(urls)
		if got == nil || got.URL != "https://b" {
			t.Errorf("findBestWheel() picked %v, want py3-none-any", got)
		}
	})

	t.Run("prefers py2.py3-none-any", func(t *testing.T) {
		urls := []pypiURLEntry{
			{PackageType: "bdist_wheel", Filename: "pkg-1.0-cp39-cp39-linux.whl", URL: "https://a"},
			{PackageType: "bdist_wheel", Filename: "pkg-1.0-py2.py3-none-any.whl", URL: "https://b"},
		}
		got := builder.findBestWheel(urls)
		if got == nil || got.URL != "https://b" {
			t.Errorf("findBestWheel() picked %v, want py2.py3-none-any", got)
		}
	})

	t.Run("falls back to platform-specific wheel", func(t *testing.T) {
		urls := []pypiURLEntry{
			{PackageType: "sdist", Filename: "pkg-1.0.tar.gz"},
			{PackageType: "bdist_wheel", Filename: "pkg-1.0-cp39-cp39-linux.whl", URL: "https://a"},
		}
		got := builder.findBestWheel(urls)
		if got == nil || got.URL != "https://a" {
			t.Errorf("findBestWheel() picked %v, want platform-specific wheel", got)
		}
	})

	t.Run("no wheels returns nil", func(t *testing.T) {
		urls := []pypiURLEntry{
			{PackageType: "sdist", Filename: "pkg-1.0.tar.gz"},
		}
		got := builder.findBestWheel(urls)
		if got != nil {
			t.Errorf("findBestWheel() = %v, want nil", got)
		}
	})

	t.Run("empty urls returns nil", func(t *testing.T) {
		got := builder.findBestWheel(nil)
		if got != nil {
			t.Errorf("findBestWheel(nil) = %v, want nil", got)
		}
	})
}

// TestPyPIBuilder_AuthoritativeBinaryNames_AfterWheelBuild verifies that
// AuthoritativeBinaryNames returns wheel-discovered executables.
func TestPyPIBuilder_AuthoritativeBinaryNames_AfterWheelBuild(t *testing.T) {
	wheelData := makeWheelZIP("black-24.0.0.dist-info", `[console_scripts]
black = black:patched_main
blackd = blackd:patched_main
`)
	wheelHash := sha256hex(wheelData)

	var serverURL string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pypi/black/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"info": {"name": "black", "summary": "Formatter", "home_page": "", "project_urls": null},
				"releases": {},
				"urls": [{
					"packagetype": "bdist_wheel",
					"url": "%s/wheels/black-24.0.0-py3-none-any.whl",
					"filename": "black-24.0.0-py3-none-any.whl",
					"digests": {"sha256": "%s"}
				}]
			}`, serverURL, wheelHash)))
		case "/wheels/black-24.0.0-py3-none-any.whl":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(wheelData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	builder := newPyPIBuilderWithTLSServer(server)

	// Before Build, should return nil
	if names := builder.AuthoritativeBinaryNames(); names != nil {
		t.Errorf("before Build(), AuthoritativeBinaryNames() = %v, want nil", names)
	}

	_, err := builder.Build(context.Background(), BuildRequest{Package: "black"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	names := builder.AuthoritativeBinaryNames()
	if len(names) != 2 {
		t.Fatalf("AuthoritativeBinaryNames() returned %d names, want 2: %v", len(names), names)
	}
	sort.Strings(names)
	if names[0] != "black" || names[1] != "blackd" {
		t.Errorf("AuthoritativeBinaryNames() = %v, want [black, blackd]", names)
	}
}

// TestPyPIBuilder_AuthoritativeBinaryNames_FallbackReturnsNil verifies that
// when wheel discovery fails and pyproject.toml fallback is used,
// AuthoritativeBinaryNames returns nil.
func TestPyPIBuilder_AuthoritativeBinaryNames_FallbackReturnsNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pypi/no-wheel/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"info": {"name": "no-wheel", "summary": "Test", "home_page": "", "project_urls": null},
				"releases": {},
				"urls": []
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Build(context.Background(), BuildRequest{Package: "no-wheel"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	names := builder.AuthoritativeBinaryNames()
	if names != nil {
		t.Errorf("AuthoritativeBinaryNames() = %v, want nil (fallback was used)", names)
	}
}

// TestPyPIBuilder_ImplementsBinaryNameProvider verifies interface compliance.
func TestPyPIBuilder_ImplementsBinaryNameProvider(t *testing.T) {
	var _ BinaryNameProvider = (*PyPIBuilder)(nil)
}
