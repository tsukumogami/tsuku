package platform

import "path/filepath"

// ValidLibcTypes lists the recognized libc values.
// The libc affects binary compatibility and package availability:
//   - glibc: GNU C Library (most Linux distributions)
//   - musl: musl libc (Alpine Linux, Void Linux musl variant)
var ValidLibcTypes = []string{"glibc", "musl"}

// DetectLibc returns the libc implementation for the current system.
// Returns "musl" if the musl dynamic linker is present, "glibc" otherwise.
//
// Detection checks for /lib/ld-musl-*.so.1 which is the standard location
// for the musl dynamic linker across all architectures (x86_64, aarch64, etc.).
func DetectLibc() string {
	return DetectLibcWithRoot("")
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
