package verify

import "strings"

// SystemLibraryRegistry contains patterns for identifying OS-provided libraries.
// System libraries are inherently expected on conforming systems and should not
// be validated during dependency checking (they're part of the OS, not tsuku).
type SystemLibraryRegistry struct {
	// linuxSonamePatterns matches Linux shared library sonames by prefix.
	// These are libraries that are part of the base system on any Linux distro.
	linuxSonamePatterns []string

	// darwinSonamePatterns matches macOS library paths/names by prefix.
	// macOS libraries are typically referenced by full path.
	darwinSonamePatterns []string

	// linuxPathPatterns matches absolute paths to Linux system library directories.
	// Used when a dependency is specified as an absolute path.
	linuxPathPatterns []string

	// darwinPathPatterns matches absolute paths to macOS system library directories.
	darwinPathPatterns []string

	// pathVariablePrefixes matches runtime path variables that indicate
	// the library location will be resolved dynamically.
	pathVariablePrefixes []string
}

// DefaultRegistry is the default system library registry with 47 patterns
// covering Linux and macOS system libraries.
var DefaultRegistry = &SystemLibraryRegistry{
	// Linux soname patterns (18 patterns)
	// These match libraries that are part of the base Linux system.
	linuxSonamePatterns: []string{
		// Virtual dynamic shared objects (kernel-provided, no files on disk)
		"linux-vdso.so", // Virtual DSO for fast syscalls (x86_64)
		"linux-gate.so", // Virtual DSO for fast syscalls (i386)

		// Dynamic linkers/loaders
		"ld-linux", // glibc dynamic linker (ld-linux-x86-64.so.2, ld-linux-aarch64.so.1, etc.)
		"ld-musl",  // musl dynamic linker (ld-musl-x86_64.so.1, etc.)

		// glibc core libraries
		"libc.so",       // C standard library
		"libm.so",       // Math library
		"libdl.so",      // Dynamic linking library (dlopen, dlsym)
		"libpthread.so", // POSIX threads library
		"librt.so",      // POSIX realtime extensions (timers, shared memory)

		// glibc additional libraries
		"libresolv.so", // DNS resolver library
		"libnsl.so",    // Network services library (NIS/YP)
		"libcrypt.so",  // Password hashing library
		"libutil.so",   // Utility functions (login, openpty)
		"libmvec.so",   // Vector math library (SIMD optimizations)

		// GCC runtime libraries
		"libgcc_s.so",  // GCC support library (exception handling, etc.)
		"libstdc++.so", // C++ standard library (GNU implementation)
		"libatomic.so", // Atomic operations library (for architectures without native support)
		"libgomp.so",   // OpenMP runtime library
	},

	// macOS library patterns (10 patterns)
	// macOS libraries are typically referenced by full path in Mach-O binaries.
	darwinSonamePatterns: []string{
		// Core system library (contains libc, libm, libpthread, etc.)
		"/usr/lib/libSystem.B.dylib",

		// C++ runtime
		"/usr/lib/libc++.1.dylib",  // libc++ (LLVM C++ standard library)
		"/usr/lib/libc++abi.dylib", // libc++ ABI library

		// Objective-C runtime
		"/usr/lib/libobjc.A.dylib", // Objective-C runtime library

		// Common system libraries
		"/usr/lib/libresolv.9.dylib",  // DNS resolver
		"/usr/lib/libz.1.dylib",       // zlib (macOS provides a system copy)
		"/usr/lib/libiconv.2.dylib",   // Character encoding conversion
		"/usr/lib/libcharset.1.dylib", // Character set detection

		// System frameworks (all frameworks under these paths are system-provided)
		"/System/Library/Frameworks/",        // Public frameworks (CoreFoundation, Security, etc.)
		"/System/Library/PrivateFrameworks/", // Private frameworks (Apple internal)
	},

	// Linux system library directory patterns (12 patterns)
	// Used for matching absolute paths to system library directories.
	linuxPathPatterns: []string{
		// Debian/Ubuntu multiarch paths (most specific first)
		"/lib/x86_64-linux-gnu/",
		"/lib/aarch64-linux-gnu/",
		"/lib/i386-linux-gnu/",
		"/lib/arm-linux-gnueabihf/",
		"/usr/lib/x86_64-linux-gnu/",
		"/usr/lib/aarch64-linux-gnu/",
		"/usr/lib/i386-linux-gnu/",
		"/usr/lib/arm-linux-gnueabihf/",

		// Traditional FHS paths
		"/lib64/",     // 64-bit libraries on RHEL/Fedora
		"/lib32/",     // 32-bit compatibility libraries
		"/lib/",       // Traditional library path (least specific)
		"/usr/lib64/", // 64-bit libraries (RHEL/Fedora)
	},

	// macOS system library directory patterns (2 patterns)
	darwinPathPatterns: []string{
		"/usr/lib/",        // System libraries
		"/System/Library/", // System frameworks and libraries
	},

	// Path variable prefixes (5 patterns)
	// These indicate runtime-resolved paths that should be handled specially.
	// Returning true for these lets the caller know to expand them first.
	pathVariablePrefixes: []string{
		"$ORIGIN",          // ELF: relative to executable/library location
		"${ORIGIN}",        // ELF: alternate syntax for $ORIGIN
		"@rpath",           // Mach-O: runtime search path
		"@loader_path",     // Mach-O: relative to loading binary
		"@executable_path", // Mach-O: relative to main executable
	},
}

// IsSystemLibrary returns true if the given library name or path represents
// a system-provided library on the specified target OS.
//
// The name can be:
//   - A soname (e.g., "libc.so.6", "libgcc_s.so.1")
//   - An absolute path (e.g., "/usr/lib/libSystem.B.dylib")
//   - A path with runtime variables (e.g., "$ORIGIN/../lib/libfoo.so")
//
// Path variables ($ORIGIN, @rpath, etc.) are recognized to signal the caller
// that path expansion is needed before final validation.
//
// The targetOS should be "linux" or "darwin" (values from runtime.GOOS).
func (r *SystemLibraryRegistry) IsSystemLibrary(name string, targetOS string) bool {
	// Check path variable prefixes first (applies to both OSes)
	// These indicate the path needs expansion before validation.
	for _, prefix := range r.pathVariablePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	switch targetOS {
	case "linux":
		// Check Linux soname patterns
		for _, pattern := range r.linuxSonamePatterns {
			if strings.HasPrefix(name, pattern) {
				return true
			}
		}
		// Check Linux path patterns (for absolute paths)
		for _, pattern := range r.linuxPathPatterns {
			if strings.HasPrefix(name, pattern) {
				return true
			}
		}

	case "darwin":
		// Check Darwin soname/path patterns
		for _, pattern := range r.darwinSonamePatterns {
			if strings.HasPrefix(name, pattern) {
				return true
			}
		}
		// Check Darwin path patterns
		for _, pattern := range r.darwinPathPatterns {
			if strings.HasPrefix(name, pattern) {
				return true
			}
		}
	}

	return false
}

// IsSystemLibrary is a convenience function that uses the DefaultRegistry.
// Returns true if the library is a system-provided library for the target OS.
func IsSystemLibrary(name string, targetOS string) bool {
	return DefaultRegistry.IsSystemLibrary(name, targetOS)
}

// IsPathVariable returns true if the name starts with a runtime path variable
// that needs expansion before validation ($ORIGIN, @rpath, etc.).
func IsPathVariable(name string) bool {
	for _, prefix := range DefaultRegistry.pathVariablePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
