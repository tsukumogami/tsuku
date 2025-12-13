package actions

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestApplyPatchAction_Name(t *testing.T) {
	action := &ApplyPatchAction{}
	if action.Name() != "apply_patch" {
		t.Errorf("Name() = %s, want apply_patch", action.Name())
	}
}

func TestApplyPatchAction_Execute_InlinePatch(t *testing.T) {
	// Skip if patch command not available
	if _, err := checkPatchCommand(); err != nil {
		t.Skip("patch command not available")
	}

	tmpDir := t.TempDir()

	// Create a test file to patch
	testFile := filepath.Join(tmpDir, "test.txt")
	originalContent := "Hello World\n"
	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a simple patch
	patchData := `--- a/test.txt
+++ b/test.txt
@@ -1 +1 @@
-Hello World
+Hello Patch
`

	action := &ApplyPatchAction{}
	ctx := &ExecutionContext{
		WorkDir: tmpDir,
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"data":  patchData,
		"strip": 1,
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify file was patched
	result, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	expected := "Hello Patch\n"
	if string(result) != expected {
		t.Errorf("File content = %q, want %q", string(result), expected)
	}
}

func TestApplyPatchAction_Execute_MultilinePatch(t *testing.T) {
	// Skip if patch command not available
	if _, err := checkPatchCommand(); err != nil {
		t.Skip("patch command not available")
	}

	tmpDir := t.TempDir()

	// Create a test file to patch
	testFile := filepath.Join(tmpDir, "config.h")
	originalContent := "#define DEBUG 1\n#define VERSION 1\n"
	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a patch that modifies multiple lines
	patchData := `--- a/config.h
+++ b/config.h
@@ -1,2 +1,2 @@
-#define DEBUG 1
-#define VERSION 1
+#define DEBUG 0
+#define VERSION 2
`

	action := &ApplyPatchAction{}
	ctx := &ExecutionContext{
		WorkDir: tmpDir,
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"data":  patchData,
		"strip": 1,
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	result, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	expected := "#define DEBUG 0\n#define VERSION 2\n"
	if string(result) != expected {
		t.Errorf("File content = %q, want %q", string(result), expected)
	}
}

func TestApplyPatchAction_Execute_StripLevel(t *testing.T) {
	// Skip if patch command not available
	if _, err := checkPatchCommand(); err != nil {
		t.Skip("patch command not available")
	}

	tmpDir := t.TempDir()

	// Create subdirectory structure matching the patch
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}

	testFile := filepath.Join(srcDir, "main.c")
	originalContent := "int main() { return 0; }\n"
	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Patch with deeper path (strip=2 needed to remove a/b/...)
	patchData := `--- a/b/src/main.c
+++ a/b/src/main.c
@@ -1 +1 @@
-int main() { return 0; }
+int main() { return 1; }
`

	action := &ApplyPatchAction{}
	ctx := &ExecutionContext{
		WorkDir: tmpDir,
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"data":  patchData,
		"strip": 2,
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	result, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	expected := "int main() { return 1; }\n"
	if string(result) != expected {
		t.Errorf("File content = %q, want %q", string(result), expected)
	}
}

func TestApplyPatchAction_Execute_Subdir(t *testing.T) {
	// Skip if patch command not available
	if _, err := checkPatchCommand(); err != nil {
		t.Skip("patch command not available")
	}

	tmpDir := t.TempDir()

	// Create subdirectory
	subDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	testFile := filepath.Join(subDir, "util.c")
	originalContent := "// Original\n"
	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Patch relative to subdir
	patchData := `--- a/util.c
+++ b/util.c
@@ -1 +1 @@
-// Original
+// Patched
`

	action := &ApplyPatchAction{}
	ctx := &ExecutionContext{
		WorkDir: tmpDir,
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"data":   patchData,
		"strip":  1,
		"subdir": "lib",
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	result, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	expected := "// Patched\n"
	if string(result) != expected {
		t.Errorf("File content = %q, want %q", string(result), expected)
	}
}

func TestApplyPatchAction_Execute_MissingParams(t *testing.T) {
	action := &ApplyPatchAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Neither url nor data
	params := map[string]interface{}{
		"strip": 1,
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() expected error for missing url/data")
	}
}

func TestApplyPatchAction_Execute_BothParams(t *testing.T) {
	action := &ApplyPatchAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Both url and data
	params := map[string]interface{}{
		"url":   "https://example.com/patch",
		"data":  "some patch",
		"strip": 1,
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() expected error for both url and data")
	}
}

func TestApplyPatchAction_Execute_InvalidURL(t *testing.T) {
	action := &ApplyPatchAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// HTTP URL (not HTTPS)
	params := map[string]interface{}{
		"url":   "http://example.com/patch",
		"strip": 1,
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() expected error for http URL")
	}
}

func TestApplyPatchAction_Execute_PathTraversalSubdir(t *testing.T) {
	action := &ApplyPatchAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	tests := []struct {
		name   string
		subdir string
	}{
		{"parent dir", "../etc"},
		{"absolute path", "/etc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := map[string]interface{}{
				"data":   "some patch",
				"subdir": tt.subdir,
			}

			err := action.Execute(ctx, params)
			if err == nil {
				t.Error("Execute() expected error for path traversal")
			}
		})
	}
}

func TestApplyPatchAction_Execute_NonexistentSubdir(t *testing.T) {
	action := &ApplyPatchAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"data":   "some patch",
		"subdir": "nonexistent",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() expected error for nonexistent subdir")
	}
}

// checkPatchCommand checks if patch command is available
func checkPatchCommand() (string, error) {
	return exec.LookPath("patch")
}
