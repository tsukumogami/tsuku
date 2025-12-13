package actions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsValidDistribution(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid distribution names
		{"simple name", "App", true},
		{"with hyphen", "App-Ack", true},
		{"with underscore", "App_Test", true},
		{"mixed case", "MyDistribution", true},
		{"with numbers", "App5", true},
		{"complex", "File-Slurp-Tiny", true},
		{"with underscore and hyphen", "My_App-Test", true},

		// Invalid distribution names
		{"empty", "", false},
		{"starts with number", "1App", false},
		{"starts with hyphen", "-App", false},
		{"starts with underscore", "_App", false},
		{"contains dot", "App.Ack", false},
		{"contains space", "App Ack", false},
		{"contains slash", "App/Ack", false},
		{"contains at", "@scope/pkg", false},
		{"module name with colons", "App::Ack", false},
		{"too long", strings.Repeat("a", 129), false},

		// Security test cases
		{"injection semicolon", "App;echo", false},
		{"injection backtick", "App`pwd`", false},
		{"injection dollar", "App$()", false},
		{"path traversal", "../../etc/passwd", false},
		{"command substitution", "$(whoami)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidDistribution(tt.input)
			if result != tt.expected {
				t.Errorf("isValidDistribution(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidCpanVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid versions
		{"empty (latest)", "", true},
		{"simple", "1.0.0", true},
		{"two parts", "1.2", true},
		{"four parts", "1.2.3.4", true},
		{"with underscore", "1.2.3_01", true},
		{"with TRIAL", "1.2.3-TRIAL", true},
		{"v prefix", "v1.2.3", true},
		{"v with multiple parts", "v1.2.3.4", true},
		{"single digit", "5", true},

		// Invalid versions
		{"starts with letter (not v)", "a1.0.0", false},
		{"contains space", "1.0 .0", false},
		{"too long", strings.Repeat("1", 51), false},

		// Security test cases
		{"injection semicolon", "1.0.0;echo", false},
		{"injection backtick", "1.0.0`pwd`", false},
		{"injection dollar", "1.0.0$()", false},
		{"injection pipe", "1.0.0|cat", false},
		{"injection ampersand", "1.0.0&rm", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidCpanVersion(tt.input)
			if result != tt.expected {
				t.Errorf("isValidCpanVersion(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDistributionToModule(t *testing.T) {
	tests := []struct {
		distribution string
		expected     string
	}{
		{"App-Ack", "App::Ack"},
		{"Perl-Critic", "Perl::Critic"},
		{"File-Slurp-Tiny", "File::Slurp::Tiny"},
		{"App-cpanminus", "App::cpanminus"},
		{"Simple", "Simple"}, // no hyphens
		{"A-B-C-D", "A::B::C::D"},
	}

	for _, tt := range tests {
		t.Run(tt.distribution, func(t *testing.T) {
			result := distributionToModule(tt.distribution)
			if result != tt.expected {
				t.Errorf("distributionToModule(%q) = %q, want %q", tt.distribution, result, tt.expected)
			}
		})
	}
}

func TestCpanInstallAction_Execute_Validation(t *testing.T) {
	action := &CpanInstallAction{}

	tests := []struct {
		name        string
		params      map[string]interface{}
		version     string
		expectError string
	}{
		{
			name:        "missing distribution parameter",
			params:      map[string]interface{}{},
			version:     "1.0.0",
			expectError: "requires 'distribution' parameter",
		},
		{
			name: "invalid distribution name",
			params: map[string]interface{}{
				"distribution": "invalid;dist",
				"executables":  []interface{}{"exe"},
			},
			version:     "1.0.0",
			expectError: "invalid distribution name",
		},
		{
			name: "module name instead of distribution",
			params: map[string]interface{}{
				"distribution": "App::Ack",
				"executables":  []interface{}{"ack"},
			},
			version:     "1.0.0",
			expectError: "invalid distribution name",
		},
		{
			name: "missing executables",
			params: map[string]interface{}{
				"distribution": "App-Ack",
			},
			version:     "1.0.0",
			expectError: "requires 'executables' parameter",
		},
		{
			name: "empty executables",
			params: map[string]interface{}{
				"distribution": "App-Ack",
				"executables":  []interface{}{},
			},
			version:     "1.0.0",
			expectError: "requires 'executables' parameter",
		},
		{
			name: "invalid executable with path",
			params: map[string]interface{}{
				"distribution": "App-Ack",
				"executables":  []interface{}{"../bin/exe"},
			},
			version:     "1.0.0",
			expectError: "must not contain path separators",
		},
		{
			name: "invalid executable with shell metacharacter",
			params: map[string]interface{}{
				"distribution": "App-Ack",
				"executables":  []interface{}{"exe;rm"},
			},
			version:     "1.0.0",
			expectError: "contains shell metacharacters",
		},
		{
			name: "invalid version",
			params: map[string]interface{}{
				"distribution": "App-Ack",
				"executables":  []interface{}{"ack"},
			},
			version:     ";echo hack",
			expectError: "invalid version format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &ExecutionContext{
				Version:    tt.version,
				InstallDir: "/tmp/test",
			}

			err := action.Execute(ctx, tt.params)
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.expectError)
				return
			}

			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestCpanInstallAction_Name(t *testing.T) {
	action := &CpanInstallAction{}
	if action.Name() != "cpan_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "cpan_install")
	}
}

func TestCpanInstallAction_Registration(t *testing.T) {
	// Verify action is registered in the global registry
	action := Get("cpan_install")
	if action == nil {
		t.Error("cpan_install action not found in registry")
		return
	}
	if action.Name() != "cpan_install" {
		t.Errorf("registered action name = %q, want %q", action.Name(), "cpan_install")
	}
}

func TestCpanInstallAction_Execute_ExecutableValidation(t *testing.T) {
	action := &CpanInstallAction{}

	tests := []struct {
		name        string
		executables []interface{}
		expectError string
	}{
		{
			name:        "empty executable name",
			executables: []interface{}{""},
			expectError: "invalid executable name length",
		},
		{
			name:        "executable with backslash",
			executables: []interface{}{"exe\\name"},
			expectError: "must not contain path separators",
		},
		{
			name:        "executable with dot-dot",
			executables: []interface{}{".."},
			expectError: "must not contain path separators",
		},
		{
			name:        "executable is single dot",
			executables: []interface{}{"."},
			expectError: "must not contain path separators",
		},
		{
			name:        "executable with control char",
			executables: []interface{}{"exe\x00name"},
			expectError: "contains control characters",
		},
		{
			name:        "executable with tab",
			executables: []interface{}{"exe\tname"},
			expectError: "contains control characters",
		},
		{
			name:        "executable with dollar sign",
			executables: []interface{}{"$PATH"},
			expectError: "contains shell metacharacters",
		},
		{
			name:        "executable with backtick",
			executables: []interface{}{"`cmd`"},
			expectError: "contains shell metacharacters",
		},
		{
			name:        "executable with pipe",
			executables: []interface{}{"cmd|cat"},
			expectError: "contains shell metacharacters",
		},
		{
			name:        "executable with ampersand",
			executables: []interface{}{"cmd&"},
			expectError: "contains shell metacharacters",
		},
		{
			name:        "executable with angle brackets",
			executables: []interface{}{"cmd>file"},
			expectError: "contains shell metacharacters",
		},
		{
			name:        "executable with parentheses",
			executables: []interface{}{"cmd()"},
			expectError: "contains shell metacharacters",
		},
		{
			name:        "executable with brackets",
			executables: []interface{}{"cmd[0]"},
			expectError: "contains shell metacharacters",
		},
		{
			name:        "executable with braces",
			executables: []interface{}{"cmd{}"},
			expectError: "contains shell metacharacters",
		},
		{
			name:        "executable too long",
			executables: []interface{}{strings.Repeat("a", 257)},
			expectError: "invalid executable name length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &ExecutionContext{
				Version:    "1.0.0",
				InstallDir: "/tmp/test",
			}

			params := map[string]interface{}{
				"distribution": "App-Ack",
				"executables":  tt.executables,
			}

			err := action.Execute(ctx, params)
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.expectError)
				return
			}

			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestCpanInstallAction_Execute_PerlNotFound(t *testing.T) {
	// Test the case where perl is not installed in tsuku's tools directory
	// We use a temporary HOME to ensure ResolvePerl() returns empty
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	// Set HOME to temp directory (no .tsuku/tools/perl-*)
	os.Setenv("HOME", tmpDir)

	action := &CpanInstallAction{}

	ctx := &ExecutionContext{
		Version:    "1.0.0",
		InstallDir: "/tmp/test",
	}

	params := map[string]interface{}{
		"distribution": "App-Ack",
		"executables":  []interface{}{"ack"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error about perl not found, got nil")
		return
	}

	// Should fail because perl is not installed
	if !strings.Contains(err.Error(), "perl not found") && !strings.Contains(err.Error(), "/bin/bash not found") {
		t.Errorf("expected error about perl or bash not found, got %q", err.Error())
	}
}

func TestResolvePerl(t *testing.T) {
	// Test with non-existent home directory
	// ResolvePerl should return empty string when perl is not installed
	result := ResolvePerl()
	// In test environment, perl is likely not installed via tsuku
	// so we just verify it doesn't panic and returns a string
	if result != "" {
		// If perl is found, verify the path looks valid
		if !strings.Contains(result, "perl") {
			t.Errorf("ResolvePerl() returned path not containing 'perl': %s", result)
		}
	}
}

func TestResolveCpanm(t *testing.T) {
	// Test with non-existent home directory
	// ResolveCpanm should return empty string when perl is not installed
	result := ResolveCpanm()
	// In test environment, perl is likely not installed via tsuku
	// so we just verify it doesn't panic and returns a string
	if result != "" {
		// If cpanm is found, verify the path looks valid
		if !strings.Contains(result, "cpanm") {
			t.Errorf("ResolveCpanm() returned path not containing 'cpanm': %s", result)
		}
	}
}

func TestResolvePerl_WithMockDirectory(t *testing.T) {
	// Create a temporary directory structure mimicking tsuku's tools
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	// Set HOME to temp directory
	os.Setenv("HOME", tmpDir)

	// Test 1: No .tsuku directory
	result := ResolvePerl()
	if result != "" {
		t.Errorf("expected empty string when .tsuku doesn't exist, got %q", result)
	}

	// Test 2: .tsuku/tools exists but no perl directories
	toolsDir := filepath.Join(tmpDir, ".tsuku", "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("failed to create tools dir: %v", err)
	}

	result = ResolvePerl()
	if result != "" {
		t.Errorf("expected empty string when no perl dirs exist, got %q", result)
	}

	// Test 3: perl directory exists but no bin/perl
	perlDir := filepath.Join(toolsDir, "perl-5.38.0")
	if err := os.MkdirAll(perlDir, 0755); err != nil {
		t.Fatalf("failed to create perl dir: %v", err)
	}

	result = ResolvePerl()
	if result != "" {
		t.Errorf("expected empty string when bin/perl doesn't exist, got %q", result)
	}

	// Test 4: bin directory exists but perl is not executable
	binDir := filepath.Join(perlDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	perlPath := filepath.Join(binDir, "perl")
	if err := os.WriteFile(perlPath, []byte("#!/bin/sh\necho perl"), 0644); err != nil {
		t.Fatalf("failed to create perl file: %v", err)
	}

	result = ResolvePerl()
	if result != "" {
		t.Errorf("expected empty string when perl is not executable, got %q", result)
	}

	// Test 5: perl is executable
	if err := os.Chmod(perlPath, 0755); err != nil {
		t.Fatalf("failed to chmod perl: %v", err)
	}

	result = ResolvePerl()
	if result != perlPath {
		t.Errorf("expected %q, got %q", perlPath, result)
	}

	// Test 6: Multiple perl versions - should return latest
	perl2Dir := filepath.Join(toolsDir, "perl-5.40.0")
	bin2Dir := filepath.Join(perl2Dir, "bin")
	if err := os.MkdirAll(bin2Dir, 0755); err != nil {
		t.Fatalf("failed to create perl2 dir: %v", err)
	}

	perl2Path := filepath.Join(bin2Dir, "perl")
	if err := os.WriteFile(perl2Path, []byte("#!/bin/sh\necho perl"), 0755); err != nil {
		t.Fatalf("failed to create perl2 file: %v", err)
	}

	result = ResolvePerl()
	if result != perl2Path {
		t.Errorf("expected latest version %q, got %q", perl2Path, result)
	}
}

func TestResolveCpanm_WithMockDirectory(t *testing.T) {
	// Create a temporary directory structure mimicking tsuku's tools
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	// Set HOME to temp directory
	os.Setenv("HOME", tmpDir)

	// Test 1: No .tsuku directory
	result := ResolveCpanm()
	if result != "" {
		t.Errorf("expected empty string when .tsuku doesn't exist, got %q", result)
	}

	// Test 2: .tsuku/tools exists but no perl directories
	toolsDir := filepath.Join(tmpDir, ".tsuku", "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("failed to create tools dir: %v", err)
	}

	result = ResolveCpanm()
	if result != "" {
		t.Errorf("expected empty string when no perl dirs exist, got %q", result)
	}

	// Test 3: perl directory exists but no bin/cpanm
	perlDir := filepath.Join(toolsDir, "perl-5.38.0")
	binDir := filepath.Join(perlDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	result = ResolveCpanm()
	if result != "" {
		t.Errorf("expected empty string when bin/cpanm doesn't exist, got %q", result)
	}

	// Test 4: cpanm exists but is not executable
	cpanmPath := filepath.Join(binDir, "cpanm")
	if err := os.WriteFile(cpanmPath, []byte("#!/bin/sh\necho cpanm"), 0644); err != nil {
		t.Fatalf("failed to create cpanm file: %v", err)
	}

	result = ResolveCpanm()
	if result != "" {
		t.Errorf("expected empty string when cpanm is not executable, got %q", result)
	}

	// Test 5: cpanm is executable
	if err := os.Chmod(cpanmPath, 0755); err != nil {
		t.Fatalf("failed to chmod cpanm: %v", err)
	}

	result = ResolveCpanm()
	if result != cpanmPath {
		t.Errorf("expected %q, got %q", cpanmPath, result)
	}

	// Test 6: Multiple perl versions - should return cpanm from latest
	perl2Dir := filepath.Join(toolsDir, "perl-5.40.0")
	bin2Dir := filepath.Join(perl2Dir, "bin")
	if err := os.MkdirAll(bin2Dir, 0755); err != nil {
		t.Fatalf("failed to create perl2 dir: %v", err)
	}

	cpanm2Path := filepath.Join(bin2Dir, "cpanm")
	if err := os.WriteFile(cpanm2Path, []byte("#!/bin/sh\necho cpanm"), 0755); err != nil {
		t.Fatalf("failed to create cpanm2 file: %v", err)
	}

	result = ResolveCpanm()
	if result != cpanm2Path {
		t.Errorf("expected latest version %q, got %q", cpanm2Path, result)
	}
}

func TestCpanInstallAction_Execute_CpanmNotFound(t *testing.T) {
	// Test the case where perl is found but cpanm is missing
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	// Set HOME to temp directory
	os.Setenv("HOME", tmpDir)

	// Create a mock perl installation WITHOUT cpanm
	toolsDir := filepath.Join(tmpDir, ".tsuku", "tools")
	perlDir := filepath.Join(toolsDir, "perl-5.38.0")
	binDir := filepath.Join(perlDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create executable perl but no cpanm
	perlPath := filepath.Join(binDir, "perl")
	if err := os.WriteFile(perlPath, []byte("#!/bin/sh\necho perl"), 0755); err != nil {
		t.Fatalf("failed to create perl file: %v", err)
	}

	action := &CpanInstallAction{}
	ctx := &ExecutionContext{
		Version:    "1.0.0",
		InstallDir: "/tmp/test",
	}

	params := map[string]interface{}{
		"distribution": "App-Ack",
		"executables":  []interface{}{"ack"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error about cpanm not found, got nil")
		return
	}

	if !strings.Contains(err.Error(), "cpanm not found") {
		t.Errorf("expected error about cpanm not found, got %q", err.Error())
	}
}

func TestCpanInstallAction_Execute_CpanmFails(t *testing.T) {
	// Test the case where both perl and cpanm exist but cpanm fails
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	// Set HOME to temp directory
	os.Setenv("HOME", tmpDir)

	// Create a mock perl installation with a fake cpanm that fails
	toolsDir := filepath.Join(tmpDir, ".tsuku", "tools")
	perlDir := filepath.Join(toolsDir, "perl-5.38.0")
	binDir := filepath.Join(perlDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create executable perl
	perlPath := filepath.Join(binDir, "perl")
	if err := os.WriteFile(perlPath, []byte("#!/bin/sh\necho perl"), 0755); err != nil {
		t.Fatalf("failed to create perl file: %v", err)
	}

	// Create cpanm that always fails
	cpanmPath := filepath.Join(binDir, "cpanm")
	cpanmScript := `#!/bin/sh
echo "cpanm: mock failure" >&2
exit 1
`
	if err := os.WriteFile(cpanmPath, []byte(cpanmScript), 0755); err != nil {
		t.Fatalf("failed to create cpanm file: %v", err)
	}

	// Create install directory
	installDir := filepath.Join(tmpDir, "install")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		t.Fatalf("failed to create install dir: %v", err)
	}

	action := &CpanInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		InstallDir: installDir,
	}

	params := map[string]interface{}{
		"distribution": "App-Ack",
		"executables":  []interface{}{"ack"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error about cpanm install failure, got nil")
		return
	}

	if !strings.Contains(err.Error(), "cpanm install failed") {
		t.Errorf("expected error about cpanm install failed, got %q", err.Error())
	}
}

func TestCpanInstallAction_Execute_MissingExecutable(t *testing.T) {
	// Test the case where cpanm succeeds but the expected executable is not created
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	// Set HOME to temp directory
	os.Setenv("HOME", tmpDir)

	// Create a mock perl installation
	toolsDir := filepath.Join(tmpDir, ".tsuku", "tools")
	perlDir := filepath.Join(toolsDir, "perl-5.38.0")
	binDir := filepath.Join(perlDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create executable perl
	perlPath := filepath.Join(binDir, "perl")
	if err := os.WriteFile(perlPath, []byte("#!/bin/sh\necho perl"), 0755); err != nil {
		t.Fatalf("failed to create perl file: %v", err)
	}

	// Create cpanm that "succeeds" but doesn't create the executable
	cpanmPath := filepath.Join(binDir, "cpanm")
	cpanmScript := `#!/bin/sh
# Mock cpanm that succeeds but creates nothing
exit 0
`
	if err := os.WriteFile(cpanmPath, []byte(cpanmScript), 0755); err != nil {
		t.Fatalf("failed to create cpanm file: %v", err)
	}

	// Create install directory
	installDir := filepath.Join(tmpDir, "install")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		t.Fatalf("failed to create install dir: %v", err)
	}

	action := &CpanInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		InstallDir: installDir,
	}

	params := map[string]interface{}{
		"distribution": "App-Ack",
		"executables":  []interface{}{"ack"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error about missing executable, got nil")
		return
	}

	if !strings.Contains(err.Error(), "expected executable") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error about missing executable, got %q", err.Error())
	}
}

func TestCpanInstallAction_Execute_SuccessfulInstall(t *testing.T) {
	// Test successful installation with wrapper script creation
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	// Set HOME to temp directory
	os.Setenv("HOME", tmpDir)

	// Create a mock perl installation
	toolsDir := filepath.Join(tmpDir, ".tsuku", "tools")
	perlDir := filepath.Join(toolsDir, "perl-5.38.0")
	perlBinDir := filepath.Join(perlDir, "bin")
	if err := os.MkdirAll(perlBinDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create executable perl
	perlPath := filepath.Join(perlBinDir, "perl")
	if err := os.WriteFile(perlPath, []byte("#!/bin/sh\necho perl"), 0755); err != nil {
		t.Fatalf("failed to create perl file: %v", err)
	}

	// Create install directory
	installDir := filepath.Join(tmpDir, "install")
	installBinDir := filepath.Join(installDir, "bin")
	if err := os.MkdirAll(installBinDir, 0755); err != nil {
		t.Fatalf("failed to create install bin dir: %v", err)
	}

	// Create cpanm that creates the expected executable
	cpanmPath := filepath.Join(perlBinDir, "cpanm")
	cpanmScript := fmt.Sprintf(`#!/bin/sh
# Mock cpanm that creates the executable
mkdir -p "%s/bin"
cat > "%s/bin/ack" << 'SCRIPT'
#!/bin/sh
echo "ack - mock script"
SCRIPT
chmod +x "%s/bin/ack"
exit 0
`, installDir, installDir, installDir)
	if err := os.WriteFile(cpanmPath, []byte(cpanmScript), 0755); err != nil {
		t.Fatalf("failed to create cpanm file: %v", err)
	}

	action := &CpanInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		Version:    "3.7.0",
		InstallDir: installDir,
	}

	params := map[string]interface{}{
		"distribution": "App-Ack",
		"executables":  []interface{}{"ack"},
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("expected successful installation, got error: %v", err)
		return
	}

	// Verify wrapper was created
	wrapperPath := filepath.Join(installBinDir, "ack")
	if _, err := os.Stat(wrapperPath); os.IsNotExist(err) {
		t.Errorf("expected wrapper at %s to exist", wrapperPath)
	}

	// Verify original was renamed to .cpanm
	cpanmOrigPath := filepath.Join(installBinDir, "ack.cpanm")
	if _, err := os.Stat(cpanmOrigPath); os.IsNotExist(err) {
		t.Errorf("expected original at %s to exist", cpanmOrigPath)
	}

	// Verify wrapper content contains PERL5LIB
	wrapperContent, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("failed to read wrapper: %v", err)
	}

	if !strings.Contains(string(wrapperContent), "PERL5LIB") {
		t.Errorf("wrapper should contain PERL5LIB, got: %s", string(wrapperContent))
	}

	if !strings.Contains(string(wrapperContent), "ack.cpanm") {
		t.Errorf("wrapper should reference ack.cpanm, got: %s", string(wrapperContent))
	}
}

func TestCpanInstallAction_Execute_EmptyVersion(t *testing.T) {
	// Test installation with empty version (latest)
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	// Set HOME to temp directory
	os.Setenv("HOME", tmpDir)

	// Create a mock perl installation
	toolsDir := filepath.Join(tmpDir, ".tsuku", "tools")
	perlDir := filepath.Join(toolsDir, "perl-5.38.0")
	perlBinDir := filepath.Join(perlDir, "bin")
	if err := os.MkdirAll(perlBinDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create executable perl
	perlPath := filepath.Join(perlBinDir, "perl")
	if err := os.WriteFile(perlPath, []byte("#!/bin/sh\necho perl"), 0755); err != nil {
		t.Fatalf("failed to create perl file: %v", err)
	}

	// Create install directory
	installDir := filepath.Join(tmpDir, "install")
	installBinDir := filepath.Join(installDir, "bin")
	if err := os.MkdirAll(installBinDir, 0755); err != nil {
		t.Fatalf("failed to create install bin dir: %v", err)
	}

	// Create cpanm that creates the expected executable
	cpanmPath := filepath.Join(perlBinDir, "cpanm")
	cpanmScript := fmt.Sprintf(`#!/bin/sh
mkdir -p "%s/bin"
cat > "%s/bin/myapp" << 'SCRIPT'
#!/bin/sh
echo "myapp"
SCRIPT
chmod +x "%s/bin/myapp"
exit 0
`, installDir, installDir, installDir)
	if err := os.WriteFile(cpanmPath, []byte(cpanmScript), 0755); err != nil {
		t.Fatalf("failed to create cpanm file: %v", err)
	}

	action := &CpanInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		Version:    "", // Empty version means latest
		InstallDir: installDir,
	}

	params := map[string]interface{}{
		"distribution": "MyApp",
		"executables":  []interface{}{"myapp"},
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("expected successful installation with empty version, got error: %v", err)
	}
}

func TestIsValidModuleName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid module names
		{"simple name", "App", true},
		{"with colons", "App::Ack", true},
		{"with underscore", "App_Test", true},
		{"mixed case", "MyModule", true},
		{"with numbers", "App5", true},
		{"complex", "File::Slurp::Tiny", true},
		{"deeply nested", "A::B::C::D::E", true},
		{"with underscore and colons", "My_App::Test_Module", true},

		// Invalid module names
		{"empty", "", false},
		{"starts with number", "1App", false},
		{"starts with underscore", "_App", false},
		{"starts with colons", "::App", false},
		{"ends with colons", "App::", false},
		{"empty part", "App::::Ack", false},
		{"contains hyphen", "App-Ack", false},
		{"contains dot", "App.Ack", false},
		{"contains space", "App Ack", false},
		{"contains slash", "App/Ack", false},
		{"too long", strings.Repeat("A", 129), false},
		{"part starts with number", "App::1Test", false},

		// Security test cases
		{"injection semicolon", "App;echo", false},
		{"injection backtick", "App`pwd`", false},
		{"injection dollar", "App$()", false},
		{"path traversal", "../../etc/passwd", false},
		{"command substitution", "$(whoami)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidModuleName(tt.input)
			if result != tt.expected {
				t.Errorf("isValidModuleName(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCpanInstallAction_Execute_ModuleParameter(t *testing.T) {
	action := &CpanInstallAction{}

	// Test invalid module name validation
	tests := []struct {
		name        string
		params      map[string]interface{}
		expectError string
	}{
		{
			name: "invalid module name with semicolon",
			params: map[string]interface{}{
				"distribution": "ack",
				"module":       "App;Ack",
				"executables":  []interface{}{"ack"},
			},
			expectError: "invalid module name",
		},
		{
			name: "invalid module name with hyphen",
			params: map[string]interface{}{
				"distribution": "ack",
				"module":       "App-Ack",
				"executables":  []interface{}{"ack"},
			},
			expectError: "invalid module name",
		},
		{
			name: "invalid module name empty part",
			params: map[string]interface{}{
				"distribution": "ack",
				"module":       "App::::Ack",
				"executables":  []interface{}{"ack"},
			},
			expectError: "invalid module name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &ExecutionContext{
				Version:    "1.0.0",
				InstallDir: "/tmp/test",
			}

			err := action.Execute(ctx, tt.params)
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.expectError)
				return
			}

			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestCpanInstallAction_Execute_WithModuleParameter(t *testing.T) {
	// Test that module parameter is used when provided
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	// Set HOME to temp directory
	os.Setenv("HOME", tmpDir)

	// Create a mock perl installation
	toolsDir := filepath.Join(tmpDir, ".tsuku", "tools")
	perlDir := filepath.Join(toolsDir, "perl-5.38.0")
	perlBinDir := filepath.Join(perlDir, "bin")
	if err := os.MkdirAll(perlBinDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create executable perl
	perlPath := filepath.Join(perlBinDir, "perl")
	if err := os.WriteFile(perlPath, []byte("#!/bin/sh\necho perl"), 0755); err != nil {
		t.Fatalf("failed to create perl file: %v", err)
	}

	// Create install directory
	installDir := filepath.Join(tmpDir, "install")
	installBinDir := filepath.Join(installDir, "bin")
	if err := os.MkdirAll(installBinDir, 0755); err != nil {
		t.Fatalf("failed to create install bin dir: %v", err)
	}

	// Create cpanm that records the module name it receives
	cpanmPath := filepath.Join(perlBinDir, "cpanm")
	logFile := filepath.Join(tmpDir, "cpanm.log")
	cpanmScript := fmt.Sprintf(`#!/bin/sh
# Record the last argument (the module name)
echo "$@" > "%s"
mkdir -p "%s/bin"
cat > "%s/bin/ack" << 'SCRIPT'
#!/bin/sh
echo "ack"
SCRIPT
chmod +x "%s/bin/ack"
exit 0
`, logFile, installDir, installDir, installDir)
	if err := os.WriteFile(cpanmPath, []byte(cpanmScript), 0755); err != nil {
		t.Fatalf("failed to create cpanm file: %v", err)
	}

	action := &CpanInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		Version:    "v3.9.0",
		InstallDir: installDir,
	}

	// Use module parameter to override distribution name conversion
	params := map[string]interface{}{
		"distribution": "ack",
		"module":       "App::Ack",
		"executables":  []interface{}{"ack"},
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("expected successful installation, got error: %v", err)
		return
	}

	// Verify cpanm was called with App::Ack@v3.9.0, not ack@v3.9.0
	logContent, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read cpanm log: %v", err)
	}

	// The cpanm call should contain App::Ack@v3.9.0
	if !strings.Contains(string(logContent), "App::Ack@v3.9.0") {
		t.Errorf("expected cpanm to be called with App::Ack@v3.9.0, got: %s", string(logContent))
	}

	// It should NOT contain just "ack" (the distribution name converted would be "ack")
	// Actually the distribution "ack" converted is just "ack", so we need to ensure the module is used
	if strings.Contains(string(logContent), " ack@") {
		t.Errorf("expected cpanm to use module name not distribution, got: %s", string(logContent))
	}
}

func TestBuildDeterministicPerlEnv(t *testing.T) {
	// Test that SOURCE_DATE_EPOCH is set
	env := buildDeterministicPerlEnv("/usr/bin/perl", false)

	foundSourceDateEpoch := false
	foundPerlEnvVar := false
	for _, e := range env {
		if strings.HasPrefix(e, "SOURCE_DATE_EPOCH=") {
			foundSourceDateEpoch = true
			if e != "SOURCE_DATE_EPOCH=0" {
				t.Errorf("expected SOURCE_DATE_EPOCH=0, got %s", e)
			}
		}
		if strings.HasPrefix(e, "PERL5LIB=") || strings.HasPrefix(e, "PERL_LOCAL_LIB_ROOT=") {
			foundPerlEnvVar = true
		}
	}

	if !foundSourceDateEpoch {
		t.Error("SOURCE_DATE_EPOCH should be set in deterministic environment")
	}
	if foundPerlEnvVar {
		t.Error("PERL* environment variables should be filtered out")
	}
}

func TestBuildDeterministicPerlEnv_PathIncludesPerlDir(t *testing.T) {
	perlPath := "/home/test/.tsuku/tools/perl-5.38.0/bin/perl"
	env := buildDeterministicPerlEnv(perlPath, false)

	expectedPerlDir := "/home/test/.tsuku/tools/perl-5.38.0/bin"
	foundPath := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			foundPath = true
			if !strings.Contains(e, expectedPerlDir) {
				t.Errorf("PATH should include perl directory %s, got: %s", expectedPerlDir, e)
			}
			break
		}
	}

	if !foundPath {
		t.Error("PATH should be set in environment")
	}
}

func TestCpanInstallAction_IsPrimitive(t *testing.T) {
	// Verify that cpan_install is registered as a primitive
	if !IsPrimitive("cpan_install") {
		t.Error("cpan_install should be registered as a primitive")
	}
}

func TestCpanInstallAction_Execute_MirrorParameter(t *testing.T) {
	// Test that mirror parameter is handled correctly
	tmpDir := t.TempDir()

	// Set HOME to temp directory (same pattern as other tests)
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Setenv("HOME", tmpDir)

	// Create mock perl installation with cpanm that logs arguments
	toolsDir := filepath.Join(tmpDir, ".tsuku", "tools")
	perlBinDir := filepath.Join(toolsDir, "perl-5.38.0", "bin")
	if err := os.MkdirAll(perlBinDir, 0755); err != nil {
		t.Fatalf("failed to create perl bin dir: %v", err)
	}

	logFile := filepath.Join(tmpDir, "cpanm.log")
	installDir := filepath.Join(tmpDir, "install")

	// Create mock perl
	perlPath := filepath.Join(perlBinDir, "perl")
	if err := os.WriteFile(perlPath, []byte("#!/bin/sh\necho 'perl'"), 0755); err != nil {
		t.Fatalf("failed to create mock perl: %v", err)
	}

	// Create mock cpanm that logs arguments and creates the executable
	cpanmPath := filepath.Join(perlBinDir, "cpanm")
	cpanmScript := fmt.Sprintf(`#!/bin/sh
echo "$@" >> %s
# Create the expected executable in the install dir
mkdir -p %s/bin
cat > %s/bin/myapp << 'SCRIPT'
#!/bin/sh
echo "myapp - mock"
SCRIPT
chmod +x %s/bin/myapp
`, logFile, installDir, installDir, installDir)
	if err := os.WriteFile(cpanmPath, []byte(cpanmScript), 0755); err != nil {
		t.Fatalf("failed to create mock cpanm: %v", err)
	}

	if err := os.MkdirAll(installDir, 0755); err != nil {
		t.Fatalf("failed to create install dir: %v", err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: installDir,
		WorkDir:    tmpDir,
		Version:    "1.0.0",
	}

	params := map[string]interface{}{
		"distribution": "MyApp",
		"executables":  []interface{}{"myapp"},
		"mirror":       "https://cpan.example.com/",
		"mirror_only":  true,
	}

	action := &CpanInstallAction{}
	err := action.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify cpanm was called with mirror options
	logContent, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read cpanm log: %v", err)
	}

	logStr := string(logContent)
	if !strings.Contains(logStr, "--mirror") {
		t.Errorf("expected cpanm to be called with --mirror, got: %s", logStr)
	}
	if !strings.Contains(logStr, "https://cpan.example.com/") {
		t.Errorf("expected cpanm to use specified mirror URL, got: %s", logStr)
	}
	if !strings.Contains(logStr, "--mirror-only") {
		t.Errorf("expected cpanm to be called with --mirror-only, got: %s", logStr)
	}
}

func TestCpanInstallAction_Execute_CpanfileParameter(t *testing.T) {
	// Test that cpanfile parameter triggers --installdeps
	tmpDir := t.TempDir()

	// Set HOME to tmpDir so findPerlInstallation finds our mock perl
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Setenv("HOME", tmpDir)

	// Create mock perl installation at ~/.tsuku/tools/perl-5.38.0/bin/
	perlBinDir := filepath.Join(tmpDir, ".tsuku", "tools", "perl-5.38.0", "bin")
	if err := os.MkdirAll(perlBinDir, 0755); err != nil {
		t.Fatalf("failed to create perl bin dir: %v", err)
	}

	logFile := filepath.Join(tmpDir, "cpanm.log")

	// Create mock perl that outputs version
	perlPath := filepath.Join(perlBinDir, "perl")
	if err := os.WriteFile(perlPath, []byte("#!/bin/sh\necho 'This is perl 5, version 38, subversion 0 (v5.38.0)'"), 0755); err != nil {
		t.Fatalf("failed to create mock perl: %v", err)
	}

	installDir := filepath.Join(tmpDir, "install")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		t.Fatalf("failed to create install dir: %v", err)
	}

	// Create mock cpanm that logs args and creates expected executable
	cpanmPath := filepath.Join(perlBinDir, "cpanm")
	cpanmScript := fmt.Sprintf(`#!/bin/sh
echo "$@" >> %s
# Create the expected executable in the bin dir so wrapper creation succeeds
# cpanm uses --local-lib which installs to installDir/bin/
mkdir -p %s/bin
echo '#!/bin/sh' > %s/bin/myapp
chmod 755 %s/bin/myapp
exit 0
`, logFile, installDir, installDir, installDir)
	if err := os.WriteFile(cpanmPath, []byte(cpanmScript), 0755); err != nil {
		t.Fatalf("failed to create mock cpanm: %v", err)
	}

	// Create a cpanfile
	cpanfileDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(cpanfileDir, 0755); err != nil {
		t.Fatalf("failed to create cpanfile dir: %v", err)
	}
	cpanfilePath := filepath.Join(cpanfileDir, "cpanfile")
	if err := os.WriteFile(cpanfilePath, []byte("requires 'Plack';\n"), 0644); err != nil {
		t.Fatalf("failed to create cpanfile: %v", err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: installDir,
		WorkDir:    tmpDir,
		Version:    "1.0.0",
	}

	params := map[string]interface{}{
		"distribution": "MyApp",
		"executables":  []interface{}{"myapp"},
		"cpanfile":     cpanfilePath,
	}

	action := &CpanInstallAction{}
	err := action.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify cpanm was called with --installdeps
	logContent, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read cpanm log: %v", err)
	}

	logStr := string(logContent)
	if !strings.Contains(logStr, "--installdeps") {
		t.Errorf("expected cpanm to be called with --installdeps, got: %s", logStr)
	}
}
