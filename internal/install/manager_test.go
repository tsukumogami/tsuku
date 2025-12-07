package install

import (
	"strings"
	"testing"
)

func TestGenerateWrapperScript_NoPathAdditions(t *testing.T) {
	content := generateWrapperScript("/home/user/.tsuku/tools/mytool-1.0.0/bin/mytool", nil)

	// Should have shebang
	if !strings.HasPrefix(content, "#!/bin/sh\n") {
		t.Errorf("wrapper should start with shebang, got: %s", content)
	}

	// Should NOT have PATH= line when no additions
	if strings.Contains(content, "PATH=") {
		t.Errorf("wrapper should not have PATH= line when no path additions, got: %s", content)
	}

	// Should have exec with target
	if !strings.Contains(content, `exec "/home/user/.tsuku/tools/mytool-1.0.0/bin/mytool" "$@"`) {
		t.Errorf("wrapper should have exec line, got: %s", content)
	}
}

func TestGenerateWrapperScript_SinglePathAddition(t *testing.T) {
	pathAdditions := []string{"/home/user/.tsuku/tools/nodejs-20.10.0/bin"}
	content := generateWrapperScript("/home/user/.tsuku/tools/turbo-1.10.0/bin/turbo", pathAdditions)

	// Should have shebang
	if !strings.HasPrefix(content, "#!/bin/sh\n") {
		t.Errorf("wrapper should start with shebang, got: %s", content)
	}

	// Should have PATH= line with the addition
	expectedPath := `PATH="/home/user/.tsuku/tools/nodejs-20.10.0/bin:$PATH"`
	if !strings.Contains(content, expectedPath) {
		t.Errorf("wrapper should have PATH addition, want %s, got: %s", expectedPath, content)
	}

	// Should have exec with target
	if !strings.Contains(content, `exec "/home/user/.tsuku/tools/turbo-1.10.0/bin/turbo" "$@"`) {
		t.Errorf("wrapper should have exec line, got: %s", content)
	}
}

func TestGenerateWrapperScript_MultiplePathAdditions(t *testing.T) {
	pathAdditions := []string{
		"/home/user/.tsuku/tools/nodejs-20.10.0/bin",
		"/home/user/.tsuku/tools/python-3.11.0/bin",
	}
	content := generateWrapperScript("/home/user/.tsuku/tools/sometool-1.0.0/bin/sometool", pathAdditions)

	// Should have shebang
	if !strings.HasPrefix(content, "#!/bin/sh\n") {
		t.Errorf("wrapper should start with shebang, got: %s", content)
	}

	// Should have PATH= line with both additions (colon separated)
	// Order may vary due to map iteration, so check both are present
	if !strings.Contains(content, "PATH=\"") {
		t.Errorf("wrapper should have PATH= line, got: %s", content)
	}
	if !strings.Contains(content, "/home/user/.tsuku/tools/nodejs-20.10.0/bin") {
		t.Errorf("wrapper should contain nodejs path, got: %s", content)
	}
	if !strings.Contains(content, "/home/user/.tsuku/tools/python-3.11.0/bin") {
		t.Errorf("wrapper should contain python path, got: %s", content)
	}
	if !strings.Contains(content, ":$PATH\"") {
		t.Errorf("wrapper should end PATH with :$PATH, got: %s", content)
	}

	// Should have exec with target
	if !strings.Contains(content, `exec "/home/user/.tsuku/tools/sometool-1.0.0/bin/sometool" "$@"`) {
		t.Errorf("wrapper should have exec line, got: %s", content)
	}
}

func TestGenerateWrapperScript_CorrectLineOrder(t *testing.T) {
	pathAdditions := []string{"/home/user/.tsuku/tools/nodejs-20.10.0/bin"}
	content := generateWrapperScript("/home/user/.tsuku/tools/turbo-1.10.0/bin/turbo", pathAdditions)

	lines := strings.Split(content, "\n")

	// Line 0: shebang
	if lines[0] != "#!/bin/sh" {
		t.Errorf("line 0 should be shebang, got: %s", lines[0])
	}

	// Line 1: PATH=
	if !strings.HasPrefix(lines[1], "PATH=") {
		t.Errorf("line 1 should be PATH=, got: %s", lines[1])
	}

	// Line 2: exec
	if !strings.HasPrefix(lines[2], "exec ") {
		t.Errorf("line 2 should be exec, got: %s", lines[2])
	}
}

func TestGenerateWrapperScript_AbsolutePaths(t *testing.T) {
	pathAdditions := []string{"/absolute/path/to/dep/bin"}
	content := generateWrapperScript("/absolute/path/to/tool/bin/tool", pathAdditions)

	// Verify absolute paths are used (no $HOME or relative paths)
	if strings.Contains(content, "$HOME") {
		t.Errorf("wrapper should use absolute paths, not $HOME, got: %s", content)
	}
	if strings.Contains(content, "$TSUKU_HOME") {
		t.Errorf("wrapper should use absolute paths, not $TSUKU_HOME, got: %s", content)
	}

	// The paths should be absolute
	if !strings.Contains(content, "/absolute/path/to/dep/bin") {
		t.Errorf("wrapper should contain absolute dep path, got: %s", content)
	}
	if !strings.Contains(content, "/absolute/path/to/tool/bin/tool") {
		t.Errorf("wrapper should contain absolute tool path, got: %s", content)
	}
}

func TestGenerateWrapperScript_NoEmptyPathEntries(t *testing.T) {
	// Empty path additions should not create empty PATH entries like "::$PATH"
	content := generateWrapperScript("/path/to/tool", []string{})

	if strings.Contains(content, "::") {
		t.Errorf("wrapper should not have empty path entries (::), got: %s", content)
	}
	if strings.Contains(content, "PATH=\":") {
		t.Errorf("wrapper should not start PATH with colon, got: %s", content)
	}
}

func TestValidateShellSafePath_ValidPaths(t *testing.T) {
	validPaths := []string{
		"/home/user/.tsuku/tools/nodejs-20.10.0/bin",
		"/usr/local/bin/tool",
		"/path/with-dashes/and_underscores/v1.0.0",
		"/path/with spaces/is fine",
		"/tmp/tool@version",
	}

	for _, path := range validPaths {
		if err := validateShellSafePath(path); err != nil {
			t.Errorf("expected path %q to be valid, got error: %v", path, err)
		}
	}
}

func TestValidateShellSafePath_DangerousChars(t *testing.T) {
	tests := []struct {
		name string
		path string
		char rune
	}{
		{"newline", "/path/with\nnewline", '\n'},
		{"carriage return", "/path/with\rreturn", '\r'},
		{"double quote", "/path/with\"quote", '"'},
		{"single quote", "/path/with'quote", '\''},
		{"backtick", "/path/with`command`", '`'},
		{"dollar sign", "/path/with$var", '$'},
		{"backslash", "/path/with\\escape", '\\'},
		{"semicolon", "/path/with;command", ';'},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateShellSafePath(tc.path)
			if err == nil {
				t.Errorf("expected error for path with %s, got nil", tc.name)
			}
		})
	}
}
