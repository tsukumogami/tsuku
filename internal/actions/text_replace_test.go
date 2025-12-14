package actions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTextReplaceAction_Name(t *testing.T) {
	t.Parallel()
	action := &TextReplaceAction{}
	if action.Name() != "text_replace" {
		t.Errorf("Name() = %s, want text_replace", action.Name())
	}
}

func TestTextReplaceAction_Execute_LiteralReplace(t *testing.T) {
	t.Parallel()
	// Create temp directory
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "Hello STATIC World"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	action := &TextReplaceAction{}
	ctx := &ExecutionContext{
		WorkDir: tmpDir,
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"file":        "test.txt",
		"pattern":     "STATIC",
		"replacement": "SHARED",
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Read file and verify
	result, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	expected := "Hello SHARED World"
	if string(result) != expected {
		t.Errorf("File content = %q, want %q", string(result), expected)
	}
}

func TestTextReplaceAction_Execute_RegexReplace(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "version.h")
	content := "#define VERSION \"1.0.0\"\n#define BUILD 123"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	action := &TextReplaceAction{}
	ctx := &ExecutionContext{
		WorkDir: tmpDir,
		Version: "2.0.0",
	}

	params := map[string]interface{}{
		"file":        "version.h",
		"pattern":     `#define VERSION ".*"`,
		"replacement": `#define VERSION "2.0.0"`,
		"regex":       true,
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	result, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	expected := "#define VERSION \"2.0.0\"\n#define BUILD 123"
	if string(result) != expected {
		t.Errorf("File content = %q, want %q", string(result), expected)
	}
}

func TestTextReplaceAction_Execute_Delete(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "Makefile")
	content := "CFLAGS=-O2 -DDEBUG\nall: build"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	action := &TextReplaceAction{}
	ctx := &ExecutionContext{
		WorkDir: tmpDir,
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"file":        "Makefile",
		"pattern":     " -DDEBUG",
		"replacement": "", // Empty = delete
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	result, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	expected := "CFLAGS=-O2\nall: build"
	if string(result) != expected {
		t.Errorf("File content = %q, want %q", string(result), expected)
	}
}

func TestTextReplaceAction_Execute_MultipleOccurrences(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.txt")
	content := "foo bar foo baz foo"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	action := &TextReplaceAction{}
	ctx := &ExecutionContext{
		WorkDir: tmpDir,
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"file":        "test.txt",
		"pattern":     "foo",
		"replacement": "qux",
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	result, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	expected := "qux bar qux baz qux"
	if string(result) != expected {
		t.Errorf("File content = %q, want %q", string(result), expected)
	}
}

func TestTextReplaceAction_Execute_PatternNotFound(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.txt")
	content := "Hello World"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	action := &TextReplaceAction{}
	ctx := &ExecutionContext{
		WorkDir: tmpDir,
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"file":        "test.txt",
		"pattern":     "NOTFOUND",
		"replacement": "REPLACED",
	}

	// Should not error when pattern not found
	if err := action.Execute(ctx, params); err != nil {
		t.Errorf("Execute() error = %v, want no error", err)
	}

	// File should be unchanged
	result, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	if string(result) != content {
		t.Errorf("File content = %q, want %q", string(result), content)
	}
}

func TestTextReplaceAction_Execute_PreservesPermissions(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "script.sh")
	content := "#!/bin/bash\necho PLACEHOLDER"
	if err := os.WriteFile(testFile, []byte(content), 0755); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	action := &TextReplaceAction{}
	ctx := &ExecutionContext{
		WorkDir: tmpDir,
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"file":        "script.sh",
		"pattern":     "PLACEHOLDER",
		"replacement": "Hello",
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Failed to stat result file: %v", err)
	}

	// Check that executable bit is preserved (0755)
	mode := info.Mode()
	if mode&0100 == 0 {
		t.Errorf("Executable bit not preserved: mode = %o", mode)
	}
}

func TestTextReplaceAction_Execute_MissingFile(t *testing.T) {
	t.Parallel()
	action := &TextReplaceAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"file":        "nonexistent.txt",
		"pattern":     "old",
		"replacement": "new",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() expected error for missing file")
	}
}

func TestTextReplaceAction_Execute_MissingFileParam(t *testing.T) {
	t.Parallel()
	action := &TextReplaceAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"pattern":     "old",
		"replacement": "new",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() expected error for missing file param")
	}
}

func TestTextReplaceAction_Execute_MissingPatternParam(t *testing.T) {
	t.Parallel()
	action := &TextReplaceAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"file":        "test.txt",
		"replacement": "new",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() expected error for missing pattern param")
	}
}

func TestTextReplaceAction_Execute_PathTraversal(t *testing.T) {
	t.Parallel()
	action := &TextReplaceAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	tests := []struct {
		name string
		file string
	}{
		{"parent dir", "../etc/passwd"},
		{"absolute path", "/etc/passwd"},
		{"nested traversal", "subdir/../../etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := map[string]interface{}{
				"file":        tt.file,
				"pattern":     "old",
				"replacement": "new",
			}

			err := action.Execute(ctx, params)
			if err == nil {
				t.Error("Execute() expected error for path traversal")
			}
		})
	}
}

func TestTextReplaceAction_Execute_InvalidRegex(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	action := &TextReplaceAction{}
	ctx := &ExecutionContext{
		WorkDir: tmpDir,
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"file":        "test.txt",
		"pattern":     "[invalid",
		"replacement": "new",
		"regex":       true,
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() expected error for invalid regex")
	}
}

func TestTextReplaceAction_Execute_VariableSubstitution(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "config.txt")
	content := "version=PLACEHOLDER"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	action := &TextReplaceAction{}
	ctx := &ExecutionContext{
		WorkDir: tmpDir,
		Version: "3.2.1",
	}

	params := map[string]interface{}{
		"file":        "config.txt",
		"pattern":     "PLACEHOLDER",
		"replacement": "{version}",
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	result, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	expected := "version=3.2.1"
	if string(result) != expected {
		t.Errorf("File content = %q, want %q", string(result), expected)
	}
}

func TestTextReplaceAction_Execute_SubdirectoryFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create subdirectory
	subDir := filepath.Join(tmpDir, "src", "lib")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	testFile := filepath.Join(subDir, "config.h")
	content := "#define DEBUG 1"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	action := &TextReplaceAction{}
	ctx := &ExecutionContext{
		WorkDir: tmpDir,
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"file":        "src/lib/config.h",
		"pattern":     "DEBUG 1",
		"replacement": "DEBUG 0",
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	result, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	expected := "#define DEBUG 0"
	if string(result) != expected {
		t.Errorf("File content = %q, want %q", string(result), expected)
	}
}
