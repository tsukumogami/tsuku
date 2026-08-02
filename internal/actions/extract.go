package actions

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	lzip "github.com/sorairolake/lzip-go"
	"github.com/ulikunitz/xz"
)

// openDestRoot opens a kernel-enforced directory handle on the extraction
// destination, which is dest resolved relative to workDir.
//
// SECURITY: This handle is the containment boundary. Every write an archive can
// influence goes through it, so the kernel checks each path component against
// the held descriptor and refuses any traversal that leaves the destination.
//
// The destination is resolved *through* a root anchored on workDir rather than
// by joining the two into a string. os.OpenRoot follows symlinks to establish
// its root, so opening a joined path directly would let an archive extracted
// earlier into workDir plant a symlink where a later step's dest points and
// anchor the whole root outside. Resolving dest through workRoot means the
// destination itself is subject to the same enforcement as the entries.
func openDestRoot(workDir, dest string) (*os.Root, error) {
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	workRoot, err := os.OpenRoot(workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open work directory: %w", err)
	}
	// The destination root holds its own descriptor and stays valid after the
	// work-directory handle is closed.
	defer func() { _ = workRoot.Close() }()

	if err := workRoot.MkdirAll(dest, 0755); err != nil {
		return nil, fmt.Errorf("failed to create destination directory: %w", err)
	}

	destRoot, err := workRoot.OpenRoot(dest)
	if err != nil {
		return nil, fmt.Errorf("failed to open destination directory: %w", err)
	}
	return destRoot, nil
}

// splitDest splits an absolute destination path into the anchor directory and
// the name resolved beneath it, so callers holding only a joined path still get
// the final component resolved under enforcement.
func splitDest(destPath string) (workDir, dest string) {
	return filepath.Dir(destPath), filepath.Base(destPath)
}

// isPathWithinDirectory reports whether targetPath is lexically contained within
// basePath.
//
// This is a diagnostic pre-filter, NOT the security boundary. It compares
// cleaned path strings and never consults the filesystem, so it cannot see
// where a path component actually points: an archive that stages a symlink in
// one entry and traverses it in a later one passes this check. Containment is
// enforced by the os.Root handle from openDestRoot; this function runs first
// only because it can name the offending archive entry, which the kernel's
// error cannot.
func isPathWithinDirectory(targetPath, basePath string) bool {
	// Get absolute paths to handle any relative path tricks
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return false
	}

	// Ensure the target starts with the base path
	// Add separator to prevent matching partial directory names (e.g., /tmp/foo matching /tmp/foobar)
	return absTarget == absBase || strings.HasPrefix(absTarget, absBase+string(os.PathSeparator))
}

// validateSymlinkTarget enforces packaging policy on where a symlink may point:
// its target must be relative and must resolve inside the destination.
//
// This is policy, NOT the traversal guard. A symlink pointing outside the tree
// writes nothing by itself, and a symlink pointing *inside* the tree is what
// carries a traversal attack, so this check neither prevents nor detects one --
// the os.Root handle from openDestRoot does that, by refusing to write through
// any symlink that leaves the destination regardless of where it points.
//
// The rule is kept deliberately: relaxing it to accept targets that exit and
// re-enter the destination is a separate change with its own compatibility
// question, and containment no longer depends on the answer.
func validateSymlinkTarget(linkTarget, linkLocation, destPath string) error {
	// If the symlink target is absolute, it could point anywhere - reject it
	if filepath.IsAbs(linkTarget) {
		return fmt.Errorf("absolute symlink targets are not allowed: %s -> %s", linkLocation, linkTarget)
	}

	// Resolve where the symlink would actually point to
	resolvedTarget := filepath.Join(filepath.Dir(linkLocation), linkTarget)

	// Verify the resolved target is within the destination directory
	if !isPathWithinDirectory(resolvedTarget, destPath) {
		return fmt.Errorf("symlink target escapes destination directory: %s -> %s (resolves to %s)",
			linkLocation, linkTarget, resolvedTarget)
	}

	return nil
}

// ExtractAction implements archive extraction
type ExtractAction struct{ BaseAction }

// IsDeterministic returns true because extraction produces identical results.
func (ExtractAction) IsDeterministic() bool { return true }

// Name returns the action name
func (a *ExtractAction) Name() string {
	return "extract"
}

// Preflight validates parameters without side effects.
func (a *ExtractAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}
	if _, ok := GetString(params, "archive"); !ok {
		result.AddError("extract action requires 'archive' parameter")
	}
	return result
}

// Execute extracts an archive
//
// Parameters:
//   - archive (required): Archive filename to extract
//   - format (required): Archive format (tar.gz, tar.xz, tar.bz2, zip, auto)
//   - dest (optional): Destination directory (defaults to work_dir)
//   - strip_dirs (optional): Number of leading path components to strip (default: 0)
//   - files (optional): List of specific files to extract (default: all files)
func (a *ExtractAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get archive filename (required)
	archiveName, ok := GetString(params, "archive")
	if !ok {
		return fmt.Errorf("extract action requires 'archive' parameter")
	}

	// Build vars for variable substitution
	vars := GetStandardVars(ctx.Version, ctx.InstallDir, ctx.WorkDir, ctx.LibsDir)

	// Apply OS mapping if present
	if osMapping, ok := GetMapStringString(params, "os_mapping"); ok {
		if mappedOS, ok := osMapping[vars["os"]]; ok {
			vars["os"] = mappedOS
		}
	}

	// Apply arch mapping if present
	if archMapping, ok := GetMapStringString(params, "arch_mapping"); ok {
		if mappedArch, ok := archMapping[vars["arch"]]; ok {
			vars["arch"] = mappedArch
		}
	}

	archiveName = ExpandVars(archiveName, vars)
	archivePath := filepath.Join(ctx.WorkDir, archiveName)

	// Get format (required)
	format, ok := GetString(params, "format")
	if !ok {
		return fmt.Errorf("extract action requires 'format' parameter")
	}

	// Auto-detect format if needed
	if format == "auto" {
		format = a.detectFormat(archiveName)
	}

	// Get destination directory (defaults to work_dir)
	dest, _ := GetString(params, "dest")
	if dest == "" {
		dest = "."
	}
	dest = ExpandVars(dest, vars)
	destPath := filepath.Join(ctx.WorkDir, dest)

	// Get strip_dirs (defaults to 0)
	stripDirs, _ := GetInt(params, "strip_dirs")

	// Get files list (optional)
	files, _ := GetStringSlice(params, "files")

	// Log extraction details
	logger := ctx.Log()
	logger.Debug("extract action starting",
		"archive", archiveName,
		"format", format,
		"destPath", destPath,
		"stripDirs", stripDirs)

	reporter := ctx.GetReporter()
	reporter.Status(fmt.Sprintf("   Extracting: %s", archiveName))

	// Extract based on format
	switch format {
	case "zip":
		return a.extractZipInWorkDir(ctx.WorkDir, dest, archivePath, stripDirs, files)
	default:
		// Anchor on the work directory so the recipe-supplied dest is resolved
		// under the same containment as the archive's own entries.
		if decompress := tarDecompressorFor(format); decompress != nil {
			return a.extractCompressedTar(ctx.WorkDir, dest, archivePath, stripDirs, files, decompress)
		}
		return fmt.Errorf("unsupported archive format: %s", format)
	}
}

// detectFormat auto-detects archive format from filename
func (a *ExtractAction) detectFormat(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".txz"):
		return "tar.xz"
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"), strings.HasSuffix(lower, ".tbz"):
		return "tar.bz2"
	case strings.HasSuffix(lower, ".tar.zst"), strings.HasSuffix(lower, ".tzst"):
		return "tar.zst"
	case strings.HasSuffix(lower, ".tar.lz"), strings.HasSuffix(lower, ".tlz"):
		return "tar.lz"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	default:
		return "unknown"
	}
}

// The tar extract* functions come in pairs: an InWorkDir form that anchors the
// destination root on the work directory (what Execute uses, and what makes the
// destination path itself subject to containment), and a thin destPath form for
// callers that only hold a joined path.

// tarDecompressor wraps a compressed archive reader in its format's decoder.
// The returned close function is nil for decoders that need no cleanup.
type tarDecompressor func(io.Reader) (io.Reader, func(), error)

func gzipDecompressor(r io.Reader) (io.Reader, func(), error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	return gzr, func() { gzr.Close() }, nil
}

func xzDecompressor(r io.Reader) (io.Reader, func(), error) {
	xzr, err := xz.NewReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create xz reader: %w", err)
	}
	return xzr, nil, nil
}

func bzip2Decompressor(r io.Reader) (io.Reader, func(), error) {
	return bzip2.NewReader(r), nil, nil
}

func zstdDecompressor(r io.Reader) (io.Reader, func(), error) {
	zr, err := zstd.NewReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create zstd reader: %w", err)
	}
	return zr, func() { zr.Close() }, nil
}

func lzipDecompressor(r io.Reader) (io.Reader, func(), error) {
	lr, err := lzip.NewReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create lzip reader: %w", err)
	}
	return lr, nil, nil
}

func plainTarDecompressor(r io.Reader) (io.Reader, func(), error) {
	return r, nil, nil
}

func (a *ExtractAction) extractTarGz(archivePath, destPath string, stripDirs int, files []string) error {
	workDir, dest := splitDest(destPath)
	return a.extractTarGzInWorkDir(workDir, dest, archivePath, stripDirs, files)
}

func (a *ExtractAction) extractTarGzInWorkDir(workDir, dest, archivePath string, stripDirs int, files []string) error {
	return a.extractCompressedTar(workDir, dest, archivePath, stripDirs, files, gzipDecompressor)
}

func (a *ExtractAction) extractTarXz(archivePath, destPath string, stripDirs int, files []string) error {
	workDir, dest := splitDest(destPath)
	return a.extractCompressedTar(workDir, dest, archivePath, stripDirs, files, xzDecompressor)
}

func (a *ExtractAction) extractTarBz2(archivePath, destPath string, stripDirs int, files []string) error {
	workDir, dest := splitDest(destPath)
	return a.extractCompressedTar(workDir, dest, archivePath, stripDirs, files, bzip2Decompressor)
}

func (a *ExtractAction) extractTarZst(archivePath, destPath string, stripDirs int, files []string) error {
	workDir, dest := splitDest(destPath)
	return a.extractCompressedTar(workDir, dest, archivePath, stripDirs, files, zstdDecompressor)
}

func (a *ExtractAction) extractTarLz(archivePath, destPath string, stripDirs int, files []string) error {
	workDir, dest := splitDest(destPath)
	return a.extractCompressedTar(workDir, dest, archivePath, stripDirs, files, lzipDecompressor)
}

func (a *ExtractAction) extractTar(archivePath, destPath string, stripDirs int, files []string) error {
	workDir, dest := splitDest(destPath)
	return a.extractCompressedTar(workDir, dest, archivePath, stripDirs, files, plainTarDecompressor)
}

// tarDecompressorFor maps an archive format to its decoder, or nil when the
// format is not a tar variant.
func tarDecompressorFor(format string) tarDecompressor {
	switch format {
	case "tar.gz", "tgz":
		return gzipDecompressor
	case "tar.xz", "txz":
		return xzDecompressor
	case "tar.bz2", "tbz2", "tbz":
		return bzip2Decompressor
	case "tar.zst", "tzst":
		return zstdDecompressor
	case "tar.lz", "tlz":
		return lzipDecompressor
	case "tar":
		return plainTarDecompressor
	default:
		return nil
	}
}

// extractCompressedTar opens the archive, wraps it in the format's decompressor,
// opens the contained destination root, and runs the shared tar loop.
func (a *ExtractAction) extractCompressedTar(
	workDir, dest, archivePath string,
	stripDirs int,
	files []string,
	decompress tarDecompressor,
) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	r, closeFn, err := decompress(file)
	if err != nil {
		return err
	}
	if closeFn != nil {
		defer closeFn()
	}

	root, err := openDestRoot(workDir, dest)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	return a.extractTarReader(tar.NewReader(r), root, stripDirs, files)
}

// extractTarReader extracts from a tar.Reader into an already-opened
// destination root. Every filesystem call goes through root, so the kernel
// enforces containment per path component.
func (a *ExtractAction) extractTarReader(tr *tar.Reader, root *os.Root, stripDirs int, files []string) error {
	destPath := root.Name()

	// Build file filter map if files list provided
	fileFilter := make(map[string]bool)
	if len(files) > 0 {
		for _, f := range files {
			fileFilter[f] = true
		}
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Clean the path to remove leading "./"
		cleanPath := strings.TrimPrefix(header.Name, "./")

		// Apply strip_dirs
		parts := strings.Split(cleanPath, "/")
		if len(parts) <= stripDirs {
			continue
		}
		parts = parts[stripDirs:]
		relativePath := filepath.Join(parts...)

		// Apply file filter if provided
		if len(fileFilter) > 0 && !fileFilter[relativePath] {
			continue
		}

		// Stripping every component of a top-level entry leaves an empty path,
		// which the root rejects. It names the destination itself, which already
		// exists. Homebrew bottles and node-style tarballs hit this on every
		// extraction with strip_dirs set.
		if relativePath == "" {
			relativePath = "."
		}

		target := filepath.Join(destPath, relativePath)

		// Lexical pre-filter: catches plain "../" entry names with an error that
		// names the entry. Not the containment guarantee -- see openDestRoot.
		if !isPathWithinDirectory(target, destPath) {
			return fmt.Errorf("archive entry escapes destination directory: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(relativePath, 0755); err != nil {
				return fmt.Errorf("archive entry %q: failed to create directory: %w", header.Name, err)
			}

		case tar.TypeReg:
			if err := root.MkdirAll(filepath.Dir(relativePath), 0755); err != nil {
				return fmt.Errorf("archive entry %q: failed to create parent directory: %w", header.Name, err)
			}

			// Perm() masks off bits the root rejects. Tar stores setuid in the
			// unix layout while os.ModeSetuid is a different bit, so the mode
			// conversion never carried those bits anyway -- this is not a
			// tightening of what lands on disk.
			f, err := root.OpenFile(relativePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode).Perm())
			if err != nil {
				return fmt.Errorf("archive entry %q: failed to create file: %w", header.Name, err)
			}

			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("archive entry %q: failed to write file: %w", header.Name, err)
			}
			f.Close()

		case tar.TypeSymlink:
			// Policy check on the link target; containment is the root's job.
			if err := validateSymlinkTarget(header.Linkname, target, destPath); err != nil {
				return err
			}

			if err := root.MkdirAll(filepath.Dir(relativePath), 0755); err != nil {
				return fmt.Errorf("archive entry %q: failed to create parent directory: %w", header.Name, err)
			}

			if err := atomicSymlinkAt(root, header.Linkname, relativePath); err != nil {
				return fmt.Errorf("archive entry %q: failed to create symlink: %w", header.Name, err)
			}
		}
	}

	return nil
}

// atomicSymlinkAt creates a symlink at a root-relative path, replacing whatever
// is there via rename. Both operations go through the root, so neither the
// temporary nor the final path can resolve outside it.
func atomicSymlinkAt(root *os.Root, target, linkPath string) error {
	tmpLink := linkPath + ".tmp"

	// Ignore the error: a stale temp link is removed here, and its absence is
	// the normal case.
	_ = root.Remove(tmpLink)

	if err := root.Symlink(target, tmpLink); err != nil {
		return err
	}

	if err := root.Rename(tmpLink, linkPath); err != nil {
		_ = root.Remove(tmpLink)
		return err
	}

	return nil
}

// atomicSymlink creates a symlink atomically using rename.
//
// Used for links created at absolute paths outside any extraction root (the
// app-bundle install and ~/Applications links), where the destination is chosen
// by tsuku rather than by an archive. Archive-controlled links go through
// atomicSymlinkAt instead.
func atomicSymlink(target, linkPath string) error {
	// Create temporary symlink with unique name
	tmpLink := linkPath + ".tmp"

	// Remove any existing temp symlink
	os.Remove(tmpLink)

	// Create the symlink at temporary location
	if err := os.Symlink(target, tmpLink); err != nil {
		return err
	}

	// Atomically rename to final location (POSIX guarantees atomic rename)
	if err := os.Rename(tmpLink, linkPath); err != nil {
		os.Remove(tmpLink) // Clean up temp file on failure
		return err
	}

	return nil
}

func (a *ExtractAction) extractZip(archivePath, destPath string, stripDirs int, files []string) error {
	workDir, dest := splitDest(destPath)
	return a.extractZipInWorkDir(workDir, dest, archivePath, stripDirs, files)
}

// extractZipInWorkDir extracts a zip archive into dest resolved beneath workDir.
//
// Zip entries never create symlinks here, but they can still be written
// *through* one an earlier archive left in the destination, so the same root
// enforcement applies.
func (a *ExtractAction) extractZipInWorkDir(workDir, dest, archivePath string, stripDirs int, files []string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	root, err := openDestRoot(workDir, dest)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	destPath := root.Name()

	// Build file filter map if files list provided
	fileFilter := make(map[string]bool)
	if len(files) > 0 {
		for _, f := range files {
			fileFilter[f] = true
		}
	}

	for _, f := range r.File {
		// Clean the path
		cleanPath := strings.TrimPrefix(f.Name, "./")

		// Apply strip_dirs
		parts := strings.Split(cleanPath, "/")
		if len(parts) <= stripDirs {
			continue
		}
		parts = parts[stripDirs:]
		relativePath := filepath.Join(parts...)

		// Apply file filter if provided
		if len(fileFilter) > 0 && !fileFilter[relativePath] {
			continue
		}

		if relativePath == "" {
			relativePath = "."
		}

		target := filepath.Join(destPath, relativePath)

		// Lexical pre-filter; the root below is the containment guarantee.
		if !isPathWithinDirectory(target, destPath) {
			return fmt.Errorf("zip entry escapes destination directory: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := root.MkdirAll(relativePath, 0755); err != nil {
				return fmt.Errorf("zip entry %q: failed to create directory: %w", f.Name, err)
			}
			continue
		}

		if err := root.MkdirAll(filepath.Dir(relativePath), 0755); err != nil {
			return fmt.Errorf("zip entry %q: failed to create parent directory: %w", f.Name, err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in zip: %w", err)
		}

		// Perm() strips the type bits a zip mode can carry; the root rejects a
		// mode with anything outside the permission bits set.
		outFile, err := root.OpenFile(relativePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, f.Mode().Perm())
		if err != nil {
			rc.Close()
			return fmt.Errorf("zip entry %q: failed to create file: %w", f.Name, err)
		}

		if _, err := io.Copy(outFile, rc); err != nil {
			outFile.Close()
			rc.Close()
			return fmt.Errorf("failed to write file: %w", err)
		}

		outFile.Close()
		rc.Close()
	}

	return nil
}
