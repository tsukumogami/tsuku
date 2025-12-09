package llm

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ulikunitz/xz"
)

// createTestTarGz creates a tar.gz archive in memory with the given files.
func createTestTarGz(t *testing.T, files map[string]testFile) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	for name, f := range files {
		header := &tar.Header{
			Name: name,
			Mode: f.mode,
			Size: int64(len(f.content)),
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("failed to write tar header: %v", err)
		}
		if _, err := tw.Write([]byte(f.content)); err != nil {
			t.Fatalf("failed to write tar content: %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	return buf.Bytes()
}

// createTestTarXz creates a tar.xz archive in memory with the given files.
func createTestTarXz(t *testing.T, files map[string]testFile) []byte {
	t.Helper()

	var buf bytes.Buffer
	xzw, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatalf("failed to create xz writer: %v", err)
	}
	tw := tar.NewWriter(xzw)

	for name, f := range files {
		header := &tar.Header{
			Name: name,
			Mode: f.mode,
			Size: int64(len(f.content)),
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("failed to write tar header: %v", err)
		}
		if _, err := tw.Write([]byte(f.content)); err != nil {
			t.Fatalf("failed to write tar content: %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := xzw.Close(); err != nil {
		t.Fatalf("failed to close xz writer: %v", err)
	}

	return buf.Bytes()
}

// createTestZip creates a zip archive in memory with the given files.
func createTestZip(t *testing.T, files map[string]testFile) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for name, f := range files {
		header := &zip.FileHeader{
			Name:   name,
			Method: zip.Deflate,
		}
		header.SetMode(os.FileMode(f.mode))

		w, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatalf("failed to create zip entry: %v", err)
		}
		if _, err := w.Write([]byte(f.content)); err != nil {
			t.Fatalf("failed to write zip content: %v", err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}

	return buf.Bytes()
}

type testFile struct {
	content string
	mode    int64
}

func TestInspectArchive_TarGz(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	// Create test archive
	files := map[string]testFile{
		"age/age":        {content: "binary content", mode: 0755},
		"age/age-keygen": {content: "binary content 2", mode: 0755},
		"age/LICENSE":    {content: "MIT License", mode: 0644},
	}
	archiveData := createTestTarGz(t, files)

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archiveData)
	}))
	defer server.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	result, err := client.inspectArchive(context.Background(), server.URL+"/age-v1.2.1.tar.gz")
	if err != nil {
		t.Fatalf("inspectArchive error: %v", err)
	}

	// Parse result
	var inspectResult InspectArchiveResult
	if err := json.Unmarshal([]byte(result), &inspectResult); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if len(inspectResult.Files) != 3 {
		t.Errorf("expected 3 files, got %d", len(inspectResult.Files))
	}

	// Check executable detection
	executableCount := 0
	for _, f := range inspectResult.Files {
		if f.Executable {
			executableCount++
		}
	}
	if executableCount != 2 {
		t.Errorf("expected 2 executables, got %d", executableCount)
	}
}

func TestInspectArchive_TarXz(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	// Create test archive
	files := map[string]testFile{
		"tool/tool": {content: "binary", mode: 0755},
	}
	archiveData := createTestTarXz(t, files)

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-xz")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archiveData)
	}))
	defer server.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	result, err := client.inspectArchive(context.Background(), server.URL+"/tool.tar.xz")
	if err != nil {
		t.Fatalf("inspectArchive error: %v", err)
	}

	var inspectResult InspectArchiveResult
	if err := json.Unmarshal([]byte(result), &inspectResult); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if len(inspectResult.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(inspectResult.Files))
	}

	if !inspectResult.Files[0].Executable {
		t.Error("expected file to be executable")
	}
}

func TestInspectArchive_Zip(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	// Create test archive
	files := map[string]testFile{
		"gh.exe":  {content: "windows binary", mode: 0755},
		"LICENSE": {content: "MIT", mode: 0644},
	}
	archiveData := createTestZip(t, files)

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archiveData)
	}))
	defer server.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	result, err := client.inspectArchive(context.Background(), server.URL+"/gh.zip")
	if err != nil {
		t.Fatalf("inspectArchive error: %v", err)
	}

	var inspectResult InspectArchiveResult
	if err := json.Unmarshal([]byte(result), &inspectResult); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if len(inspectResult.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(inspectResult.Files))
	}
}

func TestInspectArchive_HTTPError(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	_, err = client.inspectArchive(context.Background(), server.URL+"/notfound.tar.gz")
	if err == nil {
		t.Error("expected error for HTTP 404")
	}
}

func TestInspectArchive_UnsupportedFormat(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	_, err = client.inspectArchive(context.Background(), "https://example.com/file.rar")
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}

func TestDetectArchiveFormat(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://example.com/file.tar.gz", "tar.gz"},
		{"https://example.com/file.tgz", "tar.gz"},
		{"https://example.com/file.tar.xz", "tar.xz"},
		{"https://example.com/file.txz", "tar.xz"},
		{"https://example.com/file.zip", "zip"},
		{"https://example.com/file.ZIP", "zip"},
		{"https://example.com/file.rar", ""},
		{"https://example.com/file", ""},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			result := detectArchiveFormat(tc.url)
			if result != tc.expected {
				t.Errorf("detectArchiveFormat(%q) = %q, want %q", tc.url, result, tc.expected)
			}
		})
	}
}

func TestInspectArchive_TooLarge(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	// Create a server that returns more than MaxArchiveSize bytes
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		// Write more than 10MB
		data := make([]byte, MaxArchiveSize+1024)
		_, _ = w.Write(data)
	}))
	defer server.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	_, err = client.inspectArchive(context.Background(), server.URL+"/large.tar.gz")
	if err == nil {
		t.Error("expected error for archive too large")
	}
}
