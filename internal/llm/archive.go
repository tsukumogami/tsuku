package llm

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
)

// MaxArchiveSize is the maximum size of archive to download (10MB).
const MaxArchiveSize = 10 * 1024 * 1024

// ArchiveFile represents a file found in an archive.
type ArchiveFile struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	Executable bool   `json:"executable"`
}

// InspectArchiveResult is the result of inspecting an archive.
type InspectArchiveResult struct {
	Files []ArchiveFile `json:"files"`
}

// inspectArchive downloads an archive and returns a listing of its contents.
func (c *Client) inspectArchive(ctx context.Context, url string) (string, error) {
	// Download the archive to a temp file
	tmpFile, err := c.downloadArchive(ctx, url)
	if err != nil {
		return "", fmt.Errorf("failed to download archive: %w", err)
	}
	defer os.Remove(tmpFile)

	// Detect format from URL
	format := detectArchiveFormat(url)
	if format == "" {
		return "", fmt.Errorf("unsupported archive format: %s", url)
	}

	// List files in the archive
	files, err := listArchiveContents(tmpFile, format)
	if err != nil {
		return "", fmt.Errorf("failed to list archive contents: %w", err)
	}

	// Return as JSON
	result := InspectArchiveResult{Files: files}
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(jsonBytes), nil
}

// downloadArchive downloads an archive from a URL to a temp file.
func (c *Client) downloadArchive(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Create temp file with appropriate extension
	ext := filepath.Ext(url)
	if strings.HasSuffix(url, ".tar.gz") || strings.HasSuffix(url, ".tgz") {
		ext = ".tar.gz"
	} else if strings.HasSuffix(url, ".tar.xz") || strings.HasSuffix(url, ".txz") {
		ext = ".tar.xz"
	}

	tmpFile, err := os.CreateTemp("", "tsuku-inspect-*"+ext)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Limit download size
	limitedReader := io.LimitReader(resp.Body, MaxArchiveSize+1)

	n, err := io.Copy(tmpFile, limitedReader)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to download archive: %w", err)
	}

	if n > MaxArchiveSize {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("archive too large (max %d bytes)", MaxArchiveSize)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	return tmpPath, nil
}

// detectArchiveFormat detects the archive format from a URL.
func detectArchiveFormat(url string) string {
	lower := strings.ToLower(url)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".txz"):
		return "tar.xz"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	default:
		return ""
	}
}

// listArchiveContents lists the files in an archive.
func listArchiveContents(archivePath, format string) ([]ArchiveFile, error) {
	switch format {
	case "tar.gz":
		return listTarGz(archivePath)
	case "tar.xz":
		return listTarXz(archivePath)
	case "zip":
		return listZip(archivePath)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// listTarGz lists files in a tar.gz archive.
func listTarGz(archivePath string) ([]ArchiveFile, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	return listTarReader(tar.NewReader(gzr))
}

// listTarXz lists files in a tar.xz archive.
func listTarXz(archivePath string) ([]ArchiveFile, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	xzr, err := xz.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create xz reader: %w", err)
	}

	return listTarReader(tar.NewReader(xzr))
}

// listTarReader lists files from a tar.Reader.
func listTarReader(tr *tar.Reader) ([]ArchiveFile, error) {
	var files []ArchiveFile

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		// Skip directories
		if header.Typeflag == tar.TypeDir {
			continue
		}

		// Only include regular files
		if header.Typeflag != tar.TypeReg {
			continue
		}

		// Clean the path
		path := strings.TrimPrefix(header.Name, "./")

		// Check if executable (user execute bit)
		executable := header.Mode&0100 != 0

		files = append(files, ArchiveFile{
			Path:       path,
			Size:       header.Size,
			Executable: executable,
		})
	}

	return files, nil
}

// listZip lists files in a zip archive.
func listZip(archivePath string) ([]ArchiveFile, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	var files []ArchiveFile

	for _, f := range r.File {
		// Skip directories
		if f.FileInfo().IsDir() {
			continue
		}

		// Clean the path
		path := strings.TrimPrefix(f.Name, "./")

		// Check if executable (user execute bit)
		executable := f.Mode()&0100 != 0

		files = append(files, ArchiveFile{
			Path:       path,
			Size:       int64(f.UncompressedSize64),
			Executable: executable,
		})
	}

	return files, nil
}
