package actions

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestApplyPatchFileAction_Name(t *testing.T) {
	action := &ApplyPatchFileAction{}
	if action.Name() != "apply_patch_file" {
		t.Errorf("Name() = %s, want apply_patch_file", action.Name())
	}
}

func TestApplyPatchFileAction_Execute_InlineData(t *testing.T) {
	if _, err := exec.LookPath("patch"); err != nil {
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

	action := &ApplyPatchFileAction{}
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

func TestApplyPatchFileAction_Execute_FromFile(t *testing.T) {
	if _, err := exec.LookPath("patch"); err != nil {
		t.Skip("patch command not available")
	}

	tmpDir := t.TempDir()

	// Create a test file to patch
	testFile := filepath.Join(tmpDir, "test.txt")
	originalContent := "Hello World\n"
	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a patch file
	patchData := `--- a/test.txt
+++ b/test.txt
@@ -1 +1 @@
-Hello World
+Hello File
`
	patchFile := filepath.Join(tmpDir, "fix.patch")
	if err := os.WriteFile(patchFile, []byte(patchData), 0644); err != nil {
		t.Fatalf("Failed to create patch file: %v", err)
	}

	action := &ApplyPatchFileAction{}
	ctx := &ExecutionContext{
		WorkDir: tmpDir,
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"file":  patchFile,
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

	expected := "Hello File\n"
	if string(result) != expected {
		t.Errorf("File content = %q, want %q", string(result), expected)
	}
}

func TestApplyPatchFileAction_Execute_MissingParams(t *testing.T) {
	action := &ApplyPatchFileAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Neither file nor data
	params := map[string]interface{}{
		"strip": 1,
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() expected error for missing file/data")
	}
}

func TestApplyPatchFileAction_Execute_BothParams(t *testing.T) {
	action := &ApplyPatchFileAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Both file and data
	params := map[string]interface{}{
		"file":  "/some/file.patch",
		"data":  "some patch",
		"strip": 1,
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() expected error for both file and data")
	}
}

func TestApplyPatchFileAction_Execute_NonexistentFile(t *testing.T) {
	action := &ApplyPatchFileAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"file":  "/nonexistent/patch.diff",
		"strip": 1,
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() expected error for nonexistent patch file")
	}
}

func TestApplyPatchFileAction_Execute_PathTraversalSubdir(t *testing.T) {
	action := &ApplyPatchFileAction{}
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

func TestApplyPatchFileAction_IsPrimitive(t *testing.T) {
	if !IsPrimitive("apply_patch_file") {
		t.Error("apply_patch_file should be registered as a primitive")
	}
}

func TestApplyPatchFileAction_IsDeterministic(t *testing.T) {
	if !IsDeterministic("apply_patch_file") {
		t.Error("apply_patch_file should be deterministic")
	}
}
