package verify

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LC_ID_DYLIB is the Mach-O load command for the library's install name.
// This constant is not exported by Go's standard library.
const lcIDDylib macho.LoadCmd = 0xd

// ExtractELFSoname extracts the DT_SONAME from an ELF shared library.
// Returns empty string if no soname is set (library uses filename as soname).
func ExtractELFSoname(path string) (soname string, err error) {
	// Panic recovery for robustness against malformed input
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("parser panic: %v", r)
		}
	}()

	f, err := elf.Open(path)
	if err != nil {
		return "", fmt.Errorf("open ELF: %w", err)
	}
	defer func() { _ = f.Close() }()

	// DT_SONAME is in the .dynamic section
	sonames, err := f.DynString(elf.DT_SONAME)
	if err != nil {
		// No dynamic section or error reading - not a failure, just no soname
		return "", nil
	}
	if len(sonames) == 0 {
		return "", nil
	}
	return sonames[0], nil
}

// ExtractMachOInstallName extracts the install name from a Mach-O dynamic library.
// Returns empty string if no LC_ID_DYLIB is present.
func ExtractMachOInstallName(path string) (installName string, err error) {
	// Panic recovery for robustness against malformed input
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("parser panic: %v", r)
		}
	}()

	f, err := macho.Open(path)
	if err != nil {
		// Check if this is a fat binary
		if isFatBinary(path) {
			return extractFatInstallName(path)
		}
		return "", fmt.Errorf("open Mach-O: %w", err)
	}
	defer func() { _ = f.Close() }()

	return extractMachOInstallNameFromFile(f), nil
}

// extractMachOInstallNameFromFile extracts the install name from an open macho.File.
// This is a helper for reuse with fat binary slices.
func extractMachOInstallNameFromFile(f *macho.File) string {
	// Look for LC_ID_DYLIB (0xd) in load commands
	// LC_ID_DYLIB identifies this library's install name
	// The Go standard library doesn't parse LC_ID_DYLIB specifically,
	// so we need to check the raw bytes of each load command.
	for _, load := range f.Loads {
		raw := load.Raw()
		if len(raw) < 8 {
			continue
		}

		// Parse the load command header (cmd uint32, cmdsize uint32)
		cmd := f.ByteOrder.Uint32(raw[0:4])
		if macho.LoadCmd(cmd) == lcIDDylib {
			// This is LC_ID_DYLIB - parse the dylib structure
			// dylib_command: cmd(4) + cmdsize(4) + name_offset(4) + timestamp(4) + current_version(4) + compatibility_version(4)
			if len(raw) < 24 {
				continue
			}
			nameOffset := f.ByteOrder.Uint32(raw[8:12])
			if int(nameOffset) < len(raw) {
				// Name is a null-terminated string starting at nameOffset
				name := raw[nameOffset:]
				if idx := bytes.IndexByte(name, 0); idx >= 0 {
					return string(name[:idx])
				}
				return string(name)
			}
		}
	}
	return ""
}

// isFatBinary checks if a file is a fat/universal binary.
func isFatBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	magic := make([]byte, 4)
	_, err = f.Read(magic)
	if err != nil {
		return false
	}

	return bytes.Equal(magic, fatMagic)
}

// extractFatInstallName extracts the install name from a fat/universal binary.
func extractFatInstallName(path string) (installName string, err error) {
	// Panic recovery for robustness against malformed input
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("parser panic: %v", r)
		}
	}()

	ff, err := macho.OpenFat(path)
	if err != nil {
		return "", fmt.Errorf("open fat binary: %w", err)
	}
	defer func() { _ = ff.Close() }()

	// Install name should be the same across all architectures,
	// so just extract from the first slice
	if len(ff.Arches) > 0 {
		return extractMachOInstallNameFromFile(ff.Arches[0].File), nil
	}
	return "", nil
}

// ExtractSoname detects the binary format and extracts the soname/install name.
// Returns empty string if the library has no explicit soname set.
func ExtractSoname(path string) (string, error) {
	// Read magic bytes for format detection
	magic, err := readMagicForSoname(path)
	if err != nil {
		return "", err
	}

	format := detectFormatForSoname(magic)
	switch format {
	case "elf":
		return ExtractELFSoname(path)
	case "macho":
		return ExtractMachOInstallName(path)
	case "fat":
		return extractFatInstallName(path)
	default:
		return "", fmt.Errorf("not a recognized binary format")
	}
}

// readMagicForSoname reads the first 8 bytes of a file for format detection.
// This is a local copy to avoid coupling with header.go's unexported function.
func readMagicForSoname(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	magic := make([]byte, 8)
	n, err := f.Read(magic)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return magic[:n], nil
}

// detectFormatForSoname determines the binary format from magic bytes.
// This is a local copy to avoid coupling with header.go's unexported function.
func detectFormatForSoname(magic []byte) string {
	if len(magic) < 4 {
		return ""
	}

	switch {
	case bytes.HasPrefix(magic, elfMagic):
		return "elf"
	case bytes.Equal(magic[:4], machO32) || bytes.Equal(magic[:4], machO32Rev) ||
		bytes.Equal(magic[:4], machO64) || bytes.Equal(magic[:4], machO64Rev):
		return "macho"
	case bytes.Equal(magic[:4], fatMagic):
		return "fat"
	default:
		return ""
	}
}

// ExtractSonames recursively scans a directory tree and extracts sonames from all library files.
// Returns a slice of discovered sonames (excluding empty strings).
// Non-library files and extraction errors are silently skipped.
func ExtractSonames(libDir string) ([]string, error) {
	// Check that the root directory exists before walking
	if _, err := os.Stat(libDir); err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var sonames []string

	err := filepath.Walk(libDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip files/directories we can't access (permission errors, etc.)
			return nil
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip symlinks - only process real files
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		soname, extractErr := ExtractSoname(path)
		if extractErr != nil || soname == "" {
			// Skip non-libraries and libraries without sonames
			return nil
		}
		sonames = append(sonames, soname)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	return sonames, nil
}
