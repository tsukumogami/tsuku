package verify

import (
	"bytes"
	"debug/elf"
	"fmt"
	"io"
	"os"
	"runtime"
)

// ValidateABI checks that the binary's ABI is compatible with the current system.
// For Linux ELF binaries, this verifies the PT_INTERP interpreter exists.
// Returns nil for static binaries (no PT_INTERP) or non-Linux systems.
func ValidateABI(path string) (err error) {
	// Only check PT_INTERP on Linux
	if runtime.GOOS != "linux" {
		return nil
	}

	// Panic recovery for robustness against malicious input
	defer func() {
		if r := recover(); r != nil {
			err = &ValidationError{
				Category: ErrCorrupted,
				Path:     path,
				Message:  fmt.Sprintf("parser panic: %v", r),
			}
		}
	}()

	f, err := elf.Open(path)
	if err != nil {
		// Non-ELF files are handled gracefully (e.g., scripts, Mach-O on Linux)
		return nil
	}
	defer func() { _ = f.Close() }()

	// Find PT_INTERP segment
	var interpPath string
	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			// Read interpreter path from segment
			data := make([]byte, prog.Filesz)
			_, readErr := prog.ReadAt(data, 0)
			if readErr != nil && readErr != io.EOF {
				// Can't read segment - treat as corrupted
				return &ValidationError{
					Category: ErrCorrupted,
					Path:     path,
					Message:  fmt.Sprintf("cannot read PT_INTERP segment: %v", readErr),
				}
			}
			// Strip null terminator and any trailing padding
			interpPath = string(bytes.TrimRight(data, "\x00"))
			break // Use first PT_INTERP found (standard behavior)
		}
	}

	// No PT_INTERP means static binary - valid
	if interpPath == "" {
		return nil
	}

	// Check if interpreter exists on filesystem
	if _, err := os.Stat(interpPath); err != nil {
		if os.IsNotExist(err) {
			return &ValidationError{
				Category: ErrABIMismatch,
				Path:     path,
				Message:  fmt.Sprintf("interpreter %q not found (binary may be built for different libc, e.g., glibc vs musl)", interpPath),
			}
		}
		// Other stat errors (permission, etc.) - report as ABI issue
		return &ValidationError{
			Category: ErrABIMismatch,
			Path:     path,
			Message:  fmt.Sprintf("cannot access interpreter %q: %v", interpPath, err),
		}
	}

	return nil
}
