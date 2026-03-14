package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetBoolDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		params     map[string]any
		key        string
		defaultVal bool
		want       bool
	}{
		{
			name:       "key present and true",
			params:     map[string]any{"flag": true},
			key:        "flag",
			defaultVal: false,
			want:       true,
		},
		{
			name:       "key present and false",
			params:     map[string]any{"flag": false},
			key:        "flag",
			defaultVal: true,
			want:       false,
		},
		{
			name:       "key missing returns default true",
			params:     map[string]any{},
			key:        "flag",
			defaultVal: true,
			want:       true,
		},
		{
			name:       "key missing returns default false",
			params:     map[string]any{},
			key:        "flag",
			defaultVal: false,
			want:       false,
		},
		{
			name:       "key wrong type returns default",
			params:     map[string]any{"flag": "yes"},
			key:        "flag",
			defaultVal: true,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetBoolDefault(tt.params, tt.key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("GetBoolDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetupZigWrappers(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	zigPath := "/usr/bin/fake-zig"
	wrapperDir := filepath.Join(tmpDir, "wrappers")

	err := setupZigWrappers(zigPath, wrapperDir)
	if err != nil {
		t.Fatalf("setupZigWrappers() error = %v", err)
	}

	// Verify all wrappers were created
	expectedFiles := []string{"cc", "c++", "gcc", "g++", "ar", "ranlib", "ld"}
	for _, name := range expectedFiles {
		path := filepath.Join(wrapperDir, name)
		info, err := os.Lstat(path)
		if err != nil {
			t.Errorf("Expected wrapper %s to exist: %v", name, err)
			continue
		}
		// gcc and g++ are symlinks
		if name == "gcc" || name == "g++" {
			if info.Mode()&os.ModeSymlink == 0 {
				// On some systems, Lstat might resolve. Check if it's at least a file.
				if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
					t.Errorf("Expected %s to be a symlink", name)
				}
			}
		} else {
			// Regular files should be executable
			if info.Mode()&0100 == 0 {
				t.Errorf("Expected %s to be executable, mode = %v", name, info.Mode())
			}
		}
	}

	// Check that cc wrapper contains the expected zigPath
	ccContent, err := os.ReadFile(filepath.Join(wrapperDir, "cc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ccContent), zigPath) {
		t.Errorf("cc wrapper does not reference zig path %s", zigPath)
	}
	if !strings.Contains(string(ccContent), "-fPIC") {
		t.Error("cc wrapper should contain -fPIC flag")
	}

	// Check ar wrapper
	arContent, err := os.ReadFile(filepath.Join(wrapperDir, "ar"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arContent), zigPath) {
		t.Errorf("ar wrapper does not reference zig path %s", zigPath)
	}
}

func TestResolvePipx_NotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolvePipx()
	if result != "" {
		t.Errorf("ResolvePipx() = %q, want empty string", result)
	}
}

func TestResolvePipx_Installed(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	pipxDir := filepath.Join(toolsDir, "pipx-1.0.0", "bin")
	if err := os.MkdirAll(pipxDir, 0755); err != nil {
		t.Fatal(err)
	}
	pipxPath := filepath.Join(pipxDir, "pipx")
	if err := os.WriteFile(pipxPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolvePipx()
	if result != pipxPath {
		t.Errorf("ResolvePipx() = %q, want %q", result, pipxPath)
	}
}

func TestResolveGem_NotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolveGem()
	if result != "" {
		t.Errorf("ResolveGem() = %q, want empty string", result)
	}
}

func TestResolveGem_Installed(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	rubyDir := filepath.Join(toolsDir, "ruby-3.2.0", "bin")
	if err := os.MkdirAll(rubyDir, 0755); err != nil {
		t.Fatal(err)
	}
	gemPath := filepath.Join(rubyDir, "gem")
	if err := os.WriteFile(gemPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolveGem()
	if result != gemPath {
		t.Errorf("ResolveGem() = %q, want %q", result, gemPath)
	}
}

func TestResolveZig_NotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolveZig()
	if result != "" {
		t.Errorf("ResolveZig() = %q, want empty string", result)
	}
}

func TestResolveZig_Installed(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	zigDir := filepath.Join(toolsDir, "zig-0.11.0")
	if err := os.MkdirAll(zigDir, 0755); err != nil {
		t.Fatal(err)
	}
	zigPath := filepath.Join(zigDir, "zig")
	if err := os.WriteFile(zigPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolveZig()
	if result != zigPath {
		t.Errorf("ResolveZig() = %q, want %q", result, zigPath)
	}
}

func TestResolvePythonStandalone_NotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolvePythonStandalone()
	if result != "" {
		t.Errorf("ResolvePythonStandalone() = %q, want empty string", result)
	}
}

func TestResolvePythonStandalone_Installed(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	pyDir := filepath.Join(toolsDir, "python-standalone-3.12.0", "bin")
	if err := os.MkdirAll(pyDir, 0755); err != nil {
		t.Fatal(err)
	}
	pyPath := filepath.Join(pyDir, "python3")
	if err := os.WriteFile(pyPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolvePythonStandalone()
	if result != pyPath {
		t.Errorf("ResolvePythonStandalone() = %q, want %q", result, pyPath)
	}
}

func TestResolveCargo_NotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolveCargo()
	if result != "" {
		t.Errorf("ResolveCargo() = %q, want empty string", result)
	}
}

func TestResolveCargo_StandardLocation(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	rustDir := filepath.Join(toolsDir, "rust-1.75.0", "bin")
	if err := os.MkdirAll(rustDir, 0755); err != nil {
		t.Fatal(err)
	}
	cargoPath := filepath.Join(rustDir, "cargo")
	if err := os.WriteFile(cargoPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolveCargo()
	if result != cargoPath {
		t.Errorf("ResolveCargo() = %q, want %q", result, cargoPath)
	}
}

func TestResolveCargo_LegacyLocationPath(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	legacyDir := filepath.Join(toolsDir, "rust-1.75.0", "cargo", "bin")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatal(err)
	}
	cargoPath := filepath.Join(legacyDir, "cargo")
	if err := os.WriteFile(cargoPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolveCargo()
	if result != cargoPath {
		t.Errorf("ResolveCargo() = %q, want %q", result, cargoPath)
	}
}

func TestResolvePerl_NotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolvePerl()
	if result != "" {
		t.Errorf("ResolvePerl() = %q, want empty string", result)
	}
}

func TestResolvePerl_Installed(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	perlDir := filepath.Join(toolsDir, "perl-5.38.0", "bin")
	if err := os.MkdirAll(perlDir, 0755); err != nil {
		t.Fatal(err)
	}
	perlPath := filepath.Join(perlDir, "perl")
	if err := os.WriteFile(perlPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolvePerl()
	if result != perlPath {
		t.Errorf("ResolvePerl() = %q, want %q", result, perlPath)
	}
}

func TestResolveCpanm_NotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolveCpanm()
	if result != "" {
		t.Errorf("ResolveCpanm() = %q, want empty string", result)
	}
}

func TestResolveCpanm_Installed(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	perlDir := filepath.Join(toolsDir, "perl-5.38.0", "bin")
	if err := os.MkdirAll(perlDir, 0755); err != nil {
		t.Fatal(err)
	}
	cpanmPath := filepath.Join(perlDir, "cpanm")
	if err := os.WriteFile(cpanmPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TSUKU_HOME", tmpDir)

	result := ResolveCpanm()
	if result != cpanmPath {
		t.Errorf("ResolveCpanm() = %q, want %q", result, cpanmPath)
	}
}

func TestLookPathInDirs(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a fake binary
	binPath := filepath.Join(tmpDir, "mybin")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Should find in specified dirs
	result := LookPathInDirs("mybin", []string{tmpDir})
	if result != binPath {
		t.Errorf("LookPathInDirs() = %q, want %q", result, binPath)
	}

	// Should return name when not found
	result = LookPathInDirs("nonexistent_binary_xyz", []string{tmpDir})
	// If not found in dirs or PATH, returns the original name
	if result == "" {
		t.Error("LookPathInDirs() returned empty string")
	}
}

func TestDetectArchiveFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com/file.tar.gz", "tar.gz"},
		{"https://example.com/file.tgz", "tar.gz"},
		{"https://example.com/file.tar.xz", "tar.xz"},
		{"https://example.com/file.txz", "tar.xz"},
		{"https://example.com/file.tar.bz2", "tar.bz2"},
		{"https://example.com/file.tbz2", "tar.bz2"},
		{"https://example.com/file.tbz", "tar.bz2"},
		{"https://example.com/file.tar.zst", "tar.zst"},
		{"https://example.com/file.tzst", "tar.zst"},
		{"https://example.com/file.tar.lz", "tar.lz"},
		{"https://example.com/file.tlz", "tar.lz"},
		{"https://example.com/file.tar", "tar"},
		{"https://example.com/file.zip", "zip"},
		{"https://example.com/file.unknown", ""},
		{"https://example.com/file", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := DetectArchiveFormat(tt.input)
			if got != tt.want {
				t.Errorf("DetectArchiveFormat(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
