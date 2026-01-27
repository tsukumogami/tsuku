package platform

import (
	"bytes"
	"debug/elf"
	"path/filepath"
	"strings"
)

// ValidLibcTypes lists the recognized libc values.
// The libc affects binary compatibility and package availability:
//   - glibc: GNU C Library (most Linux distributions)
//   - musl: musl libc (Alpine Linux, Void Linux musl variant)
var ValidLibcTypes = []string{"glibc", "musl"}

// LibcForFamily returns the libc implementation used by a Linux family.
// Alpine uses musl, all other families use glibc.
// Returns "glibc" for unknown or empty family values.
func LibcForFamily(family string) string {
	if family == "alpine" {
		return "musl"
	}
	return "glibc"
}

// DetectLibc returns the libc implementation for the current system.
// Returns "musl" if the system uses musl libc, "glibc" otherwise.
//
// Detection examines the ELF interpreter of /bin/sh, which definitively
// identifies the system's libc. Falls back to checking for the musl
// dynamic linker at /lib/ld-musl-*.so.1 if ELF parsing fails.
func DetectLibc() string {
	// Check /bin/sh's interpreter - this definitively answers
	// "what libc do dynamically-linked binaries use on this system?"
	if libc := detectLibcFromBinary("/bin/sh"); libc != "" {
		return libc
	}
	// Fallback for unusual systems where /bin/sh isn't readable
	return DetectLibcWithRoot("")
}

// detectLibcFromBinary reads the ELF interpreter from a binary.
// Returns "musl" if interpreter contains "musl", "glibc" for other
// Linux interpreters, or "" if detection fails.
func detectLibcFromBinary(path string) string {
	f, err := elf.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			data := make([]byte, prog.Filesz)
			if _, err := prog.ReadAt(data, 0); err != nil {
				return ""
			}
			interp := string(bytes.TrimRight(data, "\x00"))
			if strings.Contains(interp, "musl") {
				return "musl"
			}
			return "glibc"
		}
	}
	// No PT_INTERP means static binary - can't determine from this file
	return ""
}

// DetectLibcWithRoot detects libc with a custom root path for testing.
// An empty root uses the real filesystem root.
func DetectLibcWithRoot(root string) string {
	// Check for musl dynamic linker
	// Pattern matches: ld-musl-x86_64.so.1, ld-musl-aarch64.so.1, etc.
	pattern := filepath.Join(root, "lib", "ld-musl-*.so.1")
	matches, _ := filepath.Glob(pattern)
	if len(matches) > 0 {
		return "musl"
	}
	return "glibc"
}
