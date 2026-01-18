package verify

import "testing"

func TestSystemLibraryRegistry_IsSystemLibrary_LinuxSonames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		soname string
		want   bool
	}{
		// Virtual DSOs
		{"linux-vdso.so.1", "linux-vdso.so.1", true},
		{"linux-gate.so.1", "linux-gate.so.1", true},

		// Dynamic linkers
		{"ld-linux-x86-64.so.2", "ld-linux-x86-64.so.2", true},
		{"ld-linux-aarch64.so.1", "ld-linux-aarch64.so.1", true},
		{"ld-musl-x86_64.so.1", "ld-musl-x86_64.so.1", true},

		// glibc core
		{"libc.so.6", "libc.so.6", true},
		{"libm.so.6", "libm.so.6", true},
		{"libdl.so.2", "libdl.so.2", true},
		{"libpthread.so.0", "libpthread.so.0", true},
		{"librt.so.1", "librt.so.1", true},

		// glibc additional
		{"libresolv.so.2", "libresolv.so.2", true},
		{"libnsl.so.1", "libnsl.so.1", true},
		{"libcrypt.so.1", "libcrypt.so.1", true},
		{"libutil.so.1", "libutil.so.1", true},
		{"libmvec.so.1", "libmvec.so.1", true},

		// GCC runtime
		{"libgcc_s.so.1", "libgcc_s.so.1", true},
		{"libstdc++.so.6", "libstdc++.so.6", true},
		{"libatomic.so.1", "libatomic.so.1", true},
		{"libgomp.so.1", "libgomp.so.1", true},

		// Non-system libraries
		{"libssl.so.3", "libssl.so.3", false},
		{"libcrypto.so.3", "libcrypto.so.3", false},
		{"libyaml.so.0", "libyaml.so.0", false},
		{"libz.so.1", "libz.so.1", false},
		{"libcurl.so.4", "libcurl.so.4", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultRegistry.IsSystemLibrary(tt.soname, "linux")
			if got != tt.want {
				t.Errorf("IsSystemLibrary(%q, linux) = %v, want %v", tt.soname, got, tt.want)
			}
		})
	}
}

func TestSystemLibraryRegistry_IsSystemLibrary_LinuxPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		// Multiarch paths
		{"/lib/x86_64-linux-gnu/libc.so.6", "/lib/x86_64-linux-gnu/libc.so.6", true},
		{"/lib/aarch64-linux-gnu/libm.so.6", "/lib/aarch64-linux-gnu/libm.so.6", true},
		{"/usr/lib/x86_64-linux-gnu/libssl.so", "/usr/lib/x86_64-linux-gnu/libssl.so", true},

		// Traditional paths
		{"/lib64/libc.so.6", "/lib64/libc.so.6", true},
		{"/lib/libc.so.6", "/lib/libc.so.6", true},
		{"/usr/lib64/libstdc++.so.6", "/usr/lib64/libstdc++.so.6", true},

		// Non-system paths
		{"/home/user/.tsuku/lib/libfoo.so", "/home/user/.tsuku/lib/libfoo.so", false},
		{"/opt/myapp/lib/libfoo.so", "/opt/myapp/lib/libfoo.so", false},
		{"/usr/local/lib/libfoo.so", "/usr/local/lib/libfoo.so", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultRegistry.IsSystemLibrary(tt.path, "linux")
			if got != tt.want {
				t.Errorf("IsSystemLibrary(%q, linux) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestSystemLibraryRegistry_IsSystemLibrary_DarwinSonames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		soname string
		want   bool
	}{
		// Core system libraries
		{"/usr/lib/libSystem.B.dylib", "/usr/lib/libSystem.B.dylib", true},
		{"/usr/lib/libc++.1.dylib", "/usr/lib/libc++.1.dylib", true},
		{"/usr/lib/libc++abi.dylib", "/usr/lib/libc++abi.dylib", true},
		{"/usr/lib/libobjc.A.dylib", "/usr/lib/libobjc.A.dylib", true},

		// Common system libraries
		{"/usr/lib/libresolv.9.dylib", "/usr/lib/libresolv.9.dylib", true},
		{"/usr/lib/libz.1.dylib", "/usr/lib/libz.1.dylib", true},
		{"/usr/lib/libiconv.2.dylib", "/usr/lib/libiconv.2.dylib", true},
		{"/usr/lib/libcharset.1.dylib", "/usr/lib/libcharset.1.dylib", true},

		// Frameworks
		{"/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", true},
		{"/System/Library/Frameworks/Security.framework/Security", "/System/Library/Frameworks/Security.framework/Security", true},
		{"/System/Library/PrivateFrameworks/Something.framework/Something", "/System/Library/PrivateFrameworks/Something.framework/Something", true},

		// Non-system libraries (macOS version of common libs that could be user-installed)
		{"/usr/local/lib/libssl.3.dylib", "/usr/local/lib/libssl.3.dylib", false},
		{"/opt/homebrew/lib/libyaml.dylib", "/opt/homebrew/lib/libyaml.dylib", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultRegistry.IsSystemLibrary(tt.soname, "darwin")
			if got != tt.want {
				t.Errorf("IsSystemLibrary(%q, darwin) = %v, want %v", tt.soname, got, tt.want)
			}
		})
	}
}

func TestSystemLibraryRegistry_IsSystemLibrary_PathVariables(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		targetOS string
		want     bool
	}{
		// ELF path variables (work on linux)
		{"$ORIGIN/../lib/libfoo.so", "$ORIGIN/../lib/libfoo.so", "linux", true},
		{"${ORIGIN}/../lib/libfoo.so", "${ORIGIN}/../lib/libfoo.so", "linux", true},

		// Mach-O path variables (work on darwin)
		{"@rpath/libfoo.dylib", "@rpath/libfoo.dylib", "darwin", true},
		{"@loader_path/../lib/libfoo.dylib", "@loader_path/../lib/libfoo.dylib", "darwin", true},
		{"@executable_path/../Frameworks/Foo.framework/Foo", "@executable_path/../Frameworks/Foo.framework/Foo", "darwin", true},

		// Path variables should be recognized on either OS (they're platform-agnostic in detection)
		{"@rpath/libfoo.so", "@rpath/libfoo.so", "linux", true},
		{"$ORIGIN/libfoo.dylib", "$ORIGIN/libfoo.dylib", "darwin", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultRegistry.IsSystemLibrary(tt.path, tt.targetOS)
			if got != tt.want {
				t.Errorf("IsSystemLibrary(%q, %s) = %v, want %v", tt.path, tt.targetOS, got, tt.want)
			}
		})
	}
}

func TestSystemLibraryRegistry_IsSystemLibrary_CrossPlatform(t *testing.T) {
	t.Parallel()

	// Test that Linux-specific patterns don't match on Darwin and vice versa
	tests := []struct {
		name     string
		lib      string
		targetOS string
		want     bool
	}{
		// Linux sonames should NOT match on Darwin
		{"libc.so.6 on darwin", "libc.so.6", "darwin", false},
		{"libm.so.6 on darwin", "libm.so.6", "darwin", false},
		{"libgcc_s.so.1 on darwin", "libgcc_s.so.1", "darwin", false},

		// Darwin paths should NOT match on Linux
		{"/usr/lib/libSystem.B.dylib on linux", "/usr/lib/libSystem.B.dylib", "linux", false},
		{"/System/Library/Frameworks/CoreFoundation on linux", "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", "linux", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultRegistry.IsSystemLibrary(tt.lib, tt.targetOS)
			if got != tt.want {
				t.Errorf("IsSystemLibrary(%q, %s) = %v, want %v", tt.lib, tt.targetOS, got, tt.want)
			}
		})
	}
}

func TestSystemLibraryRegistry_IsSystemLibrary_UnknownOS(t *testing.T) {
	t.Parallel()

	// Unknown OS should return false for OS-specific patterns
	got := DefaultRegistry.IsSystemLibrary("libc.so.6", "windows")
	if got {
		t.Error("IsSystemLibrary(libc.so.6, windows) = true, want false")
	}

	got = DefaultRegistry.IsSystemLibrary("/usr/lib/libSystem.B.dylib", "freebsd")
	if got {
		t.Error("IsSystemLibrary(/usr/lib/libSystem.B.dylib, freebsd) = true, want false")
	}

	// Path variables should still be recognized on unknown OS
	got = DefaultRegistry.IsSystemLibrary("$ORIGIN/libfoo.so", "windows")
	if !got {
		t.Error("IsSystemLibrary($ORIGIN/libfoo.so, windows) = false, want true")
	}
}

func TestIsSystemLibrary_ConvenienceFunction(t *testing.T) {
	t.Parallel()

	// Test the package-level convenience function
	if !IsSystemLibrary("libc.so.6", "linux") {
		t.Error("IsSystemLibrary(libc.so.6, linux) = false, want true")
	}

	if !IsSystemLibrary("/usr/lib/libSystem.B.dylib", "darwin") {
		t.Error("IsSystemLibrary(/usr/lib/libSystem.B.dylib, darwin) = false, want true")
	}

	if IsSystemLibrary("libssl.so.3", "linux") {
		t.Error("IsSystemLibrary(libssl.so.3, linux) = true, want false")
	}
}

func TestIsPathVariable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"$ORIGIN", "$ORIGIN/../lib/libfoo.so", true},
		{"${ORIGIN}", "${ORIGIN}/libfoo.so", true},
		{"@rpath", "@rpath/libfoo.dylib", true},
		{"@loader_path", "@loader_path/../lib/libfoo.dylib", true},
		{"@executable_path", "@executable_path/libfoo.dylib", true},
		{"regular path", "/usr/lib/libfoo.so", false},
		{"soname", "libfoo.so.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPathVariable(tt.path)
			if got != tt.want {
				t.Errorf("IsPathVariable(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestDefaultRegistry_PatternCount(t *testing.T) {
	// Verify the pattern counts match what the design specifies
	t.Run("linux soname patterns", func(t *testing.T) {
		got := len(DefaultRegistry.linuxSonamePatterns)
		want := 25
		if got != want {
			t.Errorf("len(linuxSonamePatterns) = %d, want %d", got, want)
		}
	})

	t.Run("darwin soname patterns", func(t *testing.T) {
		got := len(DefaultRegistry.darwinSonamePatterns)
		want := 10
		if got != want {
			t.Errorf("len(darwinSonamePatterns) = %d, want %d", got, want)
		}
	})

	t.Run("linux path patterns", func(t *testing.T) {
		got := len(DefaultRegistry.linuxPathPatterns)
		want := 12
		if got != want {
			t.Errorf("len(linuxPathPatterns) = %d, want %d", got, want)
		}
	})

	t.Run("darwin path patterns", func(t *testing.T) {
		got := len(DefaultRegistry.darwinPathPatterns)
		want := 2
		if got != want {
			t.Errorf("len(darwinPathPatterns) = %d, want %d", got, want)
		}
	})

	t.Run("path variable prefixes", func(t *testing.T) {
		got := len(DefaultRegistry.pathVariablePrefixes)
		want := 5
		if got != want {
			t.Errorf("len(pathVariablePrefixes) = %d, want %d", got, want)
		}
	})

	t.Run("total patterns", func(t *testing.T) {
		total := len(DefaultRegistry.linuxSonamePatterns) +
			len(DefaultRegistry.darwinSonamePatterns) +
			len(DefaultRegistry.linuxPathPatterns) +
			len(DefaultRegistry.darwinPathPatterns) +
			len(DefaultRegistry.pathVariablePrefixes)
		want := 54
		if total != want {
			t.Errorf("total patterns = %d, want %d", total, want)
		}
	})
}
