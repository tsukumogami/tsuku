package actions

import (
	"os"
	"path/filepath"
	"testing"
)

// -- gem_common.go: createGemWrapper --

func TestCreateGemWrapper_SameDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a fake gem script
	srcScript := filepath.Join(tmpDir, "mygem")
	if err := os.WriteFile(srcScript, []byte("#!/usr/bin/env ruby\nputs 'hello'"), 0755); err != nil {
		t.Fatal(err)
	}

	err := createGemWrapper(srcScript, tmpDir, "mygem", "/usr/bin", ".")
	if err != nil {
		t.Fatalf("createGemWrapper() error = %v", err)
	}

	// .gem file should exist
	gemPath := filepath.Join(tmpDir, "mygem.gem")
	if _, err := os.Stat(gemPath); os.IsNotExist(err) {
		t.Error("Expected .gem file to exist")
	}

	// wrapper should exist
	wrapperPath := filepath.Join(tmpDir, "mygem")
	wrapperContent, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(wrapperContent) == 0 {
		t.Error("Wrapper script is empty")
	}
}

func TestCreateGemWrapper_CrossDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatal(err)
	}

	srcScript := filepath.Join(srcDir, "mygem")
	if err := os.WriteFile(srcScript, []byte("#!/usr/bin/env ruby\nputs 'hello'"), 0755); err != nil {
		t.Fatal(err)
	}

	err := createGemWrapper(srcScript, dstDir, "mygem", "/usr/bin", "ruby/3.2")
	if err != nil {
		t.Fatalf("createGemWrapper() error = %v", err)
	}

	// .gem file should exist in dst
	gemPath := filepath.Join(dstDir, "mygem.gem")
	if _, err := os.Stat(gemPath); os.IsNotExist(err) {
		t.Error("Expected .gem file in dst dir")
	}

	// wrapper should exist
	wrapperPath := filepath.Join(dstDir, "mygem")
	if _, err := os.Stat(wrapperPath); os.IsNotExist(err) {
		t.Error("Expected wrapper script in dst dir")
	}
}
