package verify

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

// Magic numbers for binary format detection
var (
	elfMagic   = []byte{0x7f, 'E', 'L', 'F'}
	machO32    = []byte{0xfe, 0xed, 0xfa, 0xce}
	machO64    = []byte{0xfe, 0xed, 0xfa, 0xcf}
	machO32Rev = []byte{0xce, 0xfa, 0xed, 0xfe}
	machO64Rev = []byte{0xcf, 0xfa, 0xed, 0xfe}
	fatMagic   = []byte{0xca, 0xfe, 0xba, 0xbe}
	arMagic    = []byte{'!', '<', 'a', 'r', 'c', 'h', '>', '\n'}
)

// ValidateHeader validates that the file at path is a valid shared library
// for the current platform. It returns header information on success or
// a categorized error on failure.
func ValidateHeader(path string) (info *HeaderInfo, err error) {
	// Read magic bytes for format detection
	magic, err := readMagic(path)
	if err != nil {
		return nil, &ValidationError{
			Category: ErrUnreadable,
			Path:     path,
			Message:  fmt.Sprintf("cannot read file: %v", err),
			Err:      err,
		}
	}

	format := detectFormat(magic)
	switch format {
	case "elf":
		return validateELFPath(path)
	case "macho":
		return validateMachOPath(path)
	case "fat":
		return validateFatPath(path)
	case "ar":
		return nil, &ValidationError{
			Category: ErrNotSharedLib,
			Path:     path,
			Message:  "file is a static library (ar archive), not a shared library",
		}
	default:
		return nil, &ValidationError{
			Category: ErrInvalidFormat,
			Path:     path,
			Message:  "file is not a recognized binary format (ELF, Mach-O, or Fat)",
		}
	}
}

// readMagic reads the first 8 bytes of a file for format detection.
func readMagic(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	magic := make([]byte, 8)
	n, err := f.Read(magic)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return magic[:n], nil
}

// detectFormat determines the binary format from magic bytes.
func detectFormat(magic []byte) string {
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
	case len(magic) >= 8 && bytes.Equal(magic[:8], arMagic):
		return "ar"
	default:
		return ""
	}
}

// validateELFPath validates an ELF file as a shared object.
func validateELFPath(path string) (info *HeaderInfo, err error) {
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
		return nil, categorizeELFError(path, err)
	}
	defer func() { _ = f.Close() }()

	// Check file type - must be shared object (ET_DYN)
	if f.Type != elf.ET_DYN {
		return nil, &ValidationError{
			Category: ErrNotSharedLib,
			Path:     path,
			Message:  fmt.Sprintf("file is %s, not shared object", elfTypeName(f.Type)),
		}
	}

	// Check architecture
	currentArch := mapGoArchToELF(runtime.GOARCH)
	if f.Machine != currentArch {
		return nil, &ValidationError{
			Category: ErrWrongArch,
			Path:     path,
			Message: fmt.Sprintf("library is %s, expected %s",
				mapELFMachine(f.Machine), mapELFMachine(currentArch)),
		}
	}

	// Extract dependencies
	deps, err := f.ImportedLibraries()
	if err != nil {
		// No dependencies is valid (leaf library)
		deps = nil
	}

	// Symbol counting is lazy - skip by default for performance
	symbolCount := -1

	return &HeaderInfo{
		Format:       "ELF",
		Type:         "shared object",
		Architecture: mapELFMachine(f.Machine),
		Dependencies: deps,
		SymbolCount:  symbolCount,
	}, nil
}

// validateMachOPath validates a Mach-O file as a dynamic library.
func validateMachOPath(path string) (info *HeaderInfo, err error) {
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

	f, err := macho.Open(path)
	if err != nil {
		return nil, categorizeMachOError(path, err)
	}
	defer func() { _ = f.Close() }()

	// Check file type - must be dylib or bundle
	if f.Type != macho.TypeDylib && f.Type != macho.TypeBundle {
		return nil, &ValidationError{
			Category: ErrNotSharedLib,
			Path:     path,
			Message:  fmt.Sprintf("file is %s, not dynamic library", machoTypeName(f.Type)),
		}
	}

	// Check architecture
	currentCpu := mapGoArchToMachO(runtime.GOARCH)
	if f.Cpu != currentCpu {
		return nil, &ValidationError{
			Category: ErrWrongArch,
			Path:     path,
			Message: fmt.Sprintf("library is %s, expected %s",
				mapMachOCpu(f.Cpu), mapMachOCpu(currentCpu)),
		}
	}

	// Extract dependencies
	deps, err := f.ImportedLibraries()
	if err != nil {
		deps = nil
	}

	// Symbol counting is lazy - skip by default for performance
	symbolCount := -1

	return &HeaderInfo{
		Format:       "Mach-O",
		Type:         machoTypeName(f.Type),
		Architecture: mapMachOCpu(f.Cpu),
		Dependencies: deps,
		SymbolCount:  symbolCount,
	}, nil
}

// validateFatPath validates a fat/universal binary and extracts the appropriate architecture.
func validateFatPath(path string) (info *HeaderInfo, err error) {
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

	ff, err := macho.OpenFat(path)
	if err != nil {
		return nil, categorizeFatError(path, err)
	}
	defer func() { _ = ff.Close() }()

	targetCpu := mapGoArchToMachO(runtime.GOARCH)

	// Find matching architecture
	for _, arch := range ff.Arches {
		if arch.Cpu == targetCpu {
			info, err := validateMachOFile(arch.File)
			if err != nil {
				return nil, err
			}
			info.SourceArch = fmt.Sprintf("fat(%s)", mapMachOCpu(arch.Cpu))
			return info, nil
		}
	}

	// No matching architecture
	available := make([]string, len(ff.Arches))
	for i, arch := range ff.Arches {
		available[i] = mapMachOCpu(arch.Cpu)
	}

	return nil, &ValidationError{
		Category: ErrWrongArch,
		Path:     path,
		Message: fmt.Sprintf("no %s slice in universal binary (has: %s)",
			runtime.GOARCH, strings.Join(available, ", ")),
	}
}

// validateMachOFile validates a Mach-O file struct (used for fat binary slices).
func validateMachOFile(f *macho.File) (*HeaderInfo, error) {
	// Check file type - must be dylib or bundle
	if f.Type != macho.TypeDylib && f.Type != macho.TypeBundle {
		return nil, &ValidationError{
			Category: ErrNotSharedLib,
			Message:  fmt.Sprintf("file is %s, not dynamic library", machoTypeName(f.Type)),
		}
	}

	// Extract dependencies
	deps, err := f.ImportedLibraries()
	if err != nil {
		deps = nil
	}

	// Symbol counting is lazy
	symbolCount := -1

	return &HeaderInfo{
		Format:       "Mach-O",
		Type:         machoTypeName(f.Type),
		Architecture: mapMachOCpu(f.Cpu),
		Dependencies: deps,
		SymbolCount:  symbolCount,
	}, nil
}

// Architecture mapping functions

func mapGoArchToELF(goarch string) elf.Machine {
	switch goarch {
	case "amd64":
		return elf.EM_X86_64
	case "arm64":
		return elf.EM_AARCH64
	case "386":
		return elf.EM_386
	case "arm":
		return elf.EM_ARM
	default:
		return elf.EM_NONE
	}
}

func mapGoArchToMachO(goarch string) macho.Cpu {
	switch goarch {
	case "amd64":
		return macho.CpuAmd64
	case "arm64":
		return macho.CpuArm64
	case "386":
		return macho.Cpu386
	default:
		return 0
	}
}

func mapELFMachine(m elf.Machine) string {
	switch m {
	case elf.EM_X86_64:
		return "x86_64"
	case elf.EM_AARCH64:
		return "arm64"
	case elf.EM_386:
		return "i386"
	case elf.EM_ARM:
		return "arm"
	default:
		return fmt.Sprintf("unknown(%d)", m)
	}
}

func mapMachOCpu(c macho.Cpu) string {
	switch c {
	case macho.CpuAmd64:
		return "x86_64"
	case macho.CpuArm64:
		return "arm64"
	case macho.Cpu386:
		return "i386"
	default:
		return fmt.Sprintf("unknown(%d)", c)
	}
}

// ELF type name mapping
func elfTypeName(t elf.Type) string {
	switch t {
	case elf.ET_NONE:
		return "unknown"
	case elf.ET_REL:
		return "relocatable object"
	case elf.ET_EXEC:
		return "executable"
	case elf.ET_DYN:
		return "shared object"
	case elf.ET_CORE:
		return "core dump"
	default:
		return fmt.Sprintf("unknown type (%d)", t)
	}
}

// Mach-O type name mapping
func machoTypeName(t macho.Type) string {
	switch t {
	case macho.TypeObj:
		return "object file"
	case macho.TypeExec:
		return "executable"
	case macho.TypeDylib:
		return "dynamic library"
	case macho.TypeBundle:
		return "bundle"
	default:
		return fmt.Sprintf("unknown type (%d)", t)
	}
}

// Error categorization functions

func categorizeELFError(path string, err error) *ValidationError {
	errStr := err.Error()

	switch {
	case strings.Contains(errStr, "bad magic"):
		return &ValidationError{Category: ErrInvalidFormat, Path: path, Err: err,
			Message: "invalid ELF magic number"}

	case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
		return &ValidationError{Category: ErrTruncated, Path: path, Err: err,
			Message: "file is truncated"}

	case errors.Is(err, os.ErrNotExist):
		return &ValidationError{Category: ErrUnreadable, Path: path, Err: err,
			Message: "file not found"}

	case errors.Is(err, os.ErrPermission):
		return &ValidationError{Category: ErrUnreadable, Path: path, Err: err,
			Message: "permission denied"}

	default:
		// Check for FormatError by string matching (type is unexported)
		if strings.Contains(errStr, "offset") || strings.Contains(errStr, "section") {
			return &ValidationError{Category: ErrCorrupted, Path: path, Err: err,
				Message: fmt.Sprintf("invalid ELF structure: %v", err)}
		}
		return &ValidationError{Category: ErrUnreadable, Path: path, Err: err,
			Message: fmt.Sprintf("cannot read ELF file: %v", err)}
	}
}

func categorizeMachOError(path string, err error) *ValidationError {
	errStr := err.Error()

	switch {
	case strings.Contains(errStr, "invalid magic"):
		return &ValidationError{Category: ErrInvalidFormat, Path: path, Err: err,
			Message: "invalid Mach-O magic number"}

	case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
		return &ValidationError{Category: ErrTruncated, Path: path, Err: err,
			Message: "file is truncated"}

	case errors.Is(err, os.ErrNotExist):
		return &ValidationError{Category: ErrUnreadable, Path: path, Err: err,
			Message: "file not found"}

	case errors.Is(err, os.ErrPermission):
		return &ValidationError{Category: ErrUnreadable, Path: path, Err: err,
			Message: "permission denied"}

	default:
		if strings.Contains(errStr, "command") || strings.Contains(errStr, "load") {
			return &ValidationError{Category: ErrCorrupted, Path: path, Err: err,
				Message: fmt.Sprintf("invalid Mach-O structure: %v", err)}
		}
		return &ValidationError{Category: ErrUnreadable, Path: path, Err: err,
			Message: fmt.Sprintf("cannot read Mach-O file: %v", err)}
	}
}

func categorizeFatError(path string, err error) *ValidationError {
	if errors.Is(err, macho.ErrNotFat) {
		return &ValidationError{Category: ErrInvalidFormat, Path: path, Err: err,
			Message: "file is not a universal binary"}
	}
	return categorizeMachOError(path, err)
}
