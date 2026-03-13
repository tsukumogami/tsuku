package actions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyDirectory_PreservesSymlinks(t *testing.T) {
	t.Parallel()
	// Create temporary directories
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a test structure with symlinks
	// src/
	// ├── bin/
	// │   ├── java -> ../jdk/bin/java  (symlink)
	// │   └── javac -> ../jdk/bin/javac (symlink)
	// └── jdk/
	//     └── bin/
	//         ├── java (regular file)
	//         └── javac (regular file)

	jdkBinDir := filepath.Join(srcDir, "jdk", "bin")
	if err := os.MkdirAll(jdkBinDir, 0755); err != nil {
		t.Fatal(err)
	}

	binDir := filepath.Join(srcDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create actual binary files
	javaFile := filepath.Join(jdkBinDir, "java")
	javacFile := filepath.Join(jdkBinDir, "javac")
	if err := os.WriteFile(javaFile, []byte("java binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(javacFile, []byte("javac binary"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create symlinks in bin/
	javaLink := filepath.Join(binDir, "java")
	javacLink := filepath.Join(binDir, "javac")
	if err := os.Symlink("../jdk/bin/java", javaLink); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../jdk/bin/javac", javacLink); err != nil {
		t.Fatal(err)
	}

	// Copy directory using the shared function
	if err := CopyDirectory(srcDir, dstDir); err != nil {
		t.Fatalf("CopyDirectory failed: %v", err)
	}

	// Verify symlinks are preserved
	dstJavaLink := filepath.Join(dstDir, "bin", "java")
	dstJavacLink := filepath.Join(dstDir, "bin", "javac")

	// Check that they are symlinks
	javaInfo, err := os.Lstat(dstJavaLink)
	if err != nil {
		t.Fatalf("Failed to lstat java link: %v", err)
	}
	if javaInfo.Mode()&os.ModeSymlink == 0 {
		t.Error("java should be a symlink but is a regular file")
	}

	javacInfo, err := os.Lstat(dstJavacLink)
	if err != nil {
		t.Fatalf("Failed to lstat javac link: %v", err)
	}
	if javacInfo.Mode()&os.ModeSymlink == 0 {
		t.Error("javac should be a symlink but is a regular file")
	}

	// Verify symlink targets are correct
	javaTarget, err := os.Readlink(dstJavaLink)
	if err != nil {
		t.Fatalf("Failed to read java symlink: %v", err)
	}
	if javaTarget != "../jdk/bin/java" {
		t.Errorf("java symlink target = %q, want %q", javaTarget, "../jdk/bin/java")
	}

	javacTarget, err := os.Readlink(dstJavacLink)
	if err != nil {
		t.Fatalf("Failed to read javac symlink: %v", err)
	}
	if javacTarget != "../jdk/bin/javac" {
		t.Errorf("javac symlink target = %q, want %q", javacTarget, "../jdk/bin/javac")
	}

	// Verify the actual files were also copied
	dstJavaFile := filepath.Join(dstDir, "jdk", "bin", "java")
	dstJavacFile := filepath.Join(dstDir, "jdk", "bin", "javac")

	javaContent, err := os.ReadFile(dstJavaFile)
	if err != nil {
		t.Fatalf("Failed to read copied java file: %v", err)
	}
	if string(javaContent) != "java binary" {
		t.Errorf("java file content = %q, want %q", string(javaContent), "java binary")
	}

	javacContent, err := os.ReadFile(dstJavacFile)
	if err != nil {
		t.Fatalf("Failed to read copied javac file: %v", err)
	}
	if string(javacContent) != "javac binary" {
		t.Errorf("javac file content = %q, want %q", string(javacContent), "javac binary")
	}
}

func TestCopySymlink(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a target file
	targetFile := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a symlink to the target
	srcLink := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink("target.txt", srcLink); err != nil {
		t.Fatal(err)
	}

	// Copy the symlink
	dstLink := filepath.Join(tmpDir, "copied_link.txt")
	if err := CopySymlink(srcLink, dstLink); err != nil {
		t.Fatalf("CopySymlink failed: %v", err)
	}

	// Verify it's a symlink
	info, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatalf("Failed to lstat copied link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("copied link should be a symlink")
	}

	// Verify target is correct
	target, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatalf("Failed to read copied symlink: %v", err)
	}
	if target != "target.txt" {
		t.Errorf("symlink target = %q, want %q", target, "target.txt")
	}
}

func TestCopyFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	srcFile := filepath.Join(tmpDir, "source.txt")
	dstFile := filepath.Join(tmpDir, "dest.txt")

	// Create source file with specific mode
	content := []byte("test file content")
	if err := os.WriteFile(srcFile, content, 0755); err != nil {
		t.Fatal(err)
	}

	// Copy the file
	if err := CopyFile(srcFile, dstFile, 0755); err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	// Verify content
	copiedContent, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("Failed to read copied file: %v", err)
	}
	if string(copiedContent) != string(content) {
		t.Errorf("copied content = %q, want %q", string(copiedContent), string(content))
	}

	// Verify permissions
	info, err := os.Stat(dstFile)
	if err != nil {
		t.Fatalf("Failed to stat copied file: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("file mode = %o, want %o", info.Mode().Perm(), 0755)
	}
}

// -- utils.go: CopyDirectoryExcluding, CopySymlink, CopyFile --

func TestCopyDirectoryExcluding_WithExclude(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dst")

	// Create source structure with an excluded dir
	if err := os.MkdirAll(filepath.Join(srcDir, "keep", "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "exclude_me"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "keep", "file.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "exclude_me", "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CopyDirectoryExcluding(srcDir, dstDir, "exclude_me"); err != nil {
		t.Fatalf("CopyDirectoryExcluding() error = %v", err)
	}

	// Kept files should exist
	if _, err := os.Stat(filepath.Join(dstDir, "keep", "file.txt")); err != nil {
		t.Error("Expected keep/file.txt to be copied")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "root.txt")); err != nil {
		t.Error("Expected root.txt to be copied")
	}

	// Excluded dir should not exist
	if _, err := os.Stat(filepath.Join(dstDir, "exclude_me")); !os.IsNotExist(err) {
		t.Error("Expected exclude_me directory to be excluded")
	}
}

func TestCopyDirectoryExcluding_WithSymlinks(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dst")

	// Create a file and a symlink to it
	if err := os.WriteFile(filepath.Join(srcDir, "real.txt"), []byte("real"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(srcDir, "link.txt")); err != nil {
		t.Fatal(err)
	}

	if err := CopyDirectoryExcluding(srcDir, dstDir, ""); err != nil {
		t.Fatalf("CopyDirectoryExcluding() error = %v", err)
	}

	// Symlink should be preserved
	target, err := os.Readlink(filepath.Join(dstDir, "link.txt"))
	if err != nil {
		t.Fatalf("Readlink error: %v", err)
	}
	if target != "real.txt" {
		t.Errorf("symlink target = %q, want %q", target, "real.txt")
	}
}

func TestCopyFile_Success(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "test.txt")
	dstPath := filepath.Join(dstDir, "sub", "test.txt")
	if err := os.WriteFile(srcPath, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CopyFile(srcPath, dstPath, 0755); err != nil {
		t.Fatalf("CopyFile() error = %v", err)
	}

	data, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "content" {
		t.Errorf("file content = %q, want %q", string(data), "content")
	}

	info, _ := os.Stat(dstPath)
	if info.Mode().Perm() != 0755 {
		t.Errorf("file mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	t.Parallel()
	err := CopyFile("/nonexistent/file", filepath.Join(t.TempDir(), "dst"), 0644)
	if err == nil {
		t.Error("Expected error for nonexistent source")
	}
}

func TestCopySymlink_Success(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create source symlink
	if err := os.WriteFile(filepath.Join(srcDir, "target.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	srcLink := filepath.Join(srcDir, "link")
	if err := os.Symlink("target.txt", srcLink); err != nil {
		t.Fatal(err)
	}

	dstLink := filepath.Join(dstDir, "sub", "link")
	if err := CopySymlink(srcLink, dstLink); err != nil {
		t.Fatalf("CopySymlink() error = %v", err)
	}

	target, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatal(err)
	}
	if target != "target.txt" {
		t.Errorf("symlink target = %q, want %q", target, "target.txt")
	}
}

func TestCopyDirectory_NoExclude(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dst")

	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CopyDirectory(srcDir, dstDir); err != nil {
		t.Fatalf("CopyDirectory() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(dstDir, "a.txt")); err != nil {
		t.Error("Expected a.txt to be copied")
	}
}

// -- composites.go: CopyDirectory --

func TestCopyDirectory(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create some files in src
	if err := os.MkdirAll(filepath.Join(srcDir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "bin", "tool"), []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "README"), []byte("readme"), 0644); err != nil {
		t.Fatal(err)
	}

	err := CopyDirectory(srcDir, dstDir)
	if err != nil {
		t.Fatalf("CopyDirectory() error: %v", err)
	}

	// Verify files were copied
	if _, err := os.Stat(filepath.Join(dstDir, "bin", "tool")); os.IsNotExist(err) {
		t.Error("Expected bin/tool to be copied")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "README")); os.IsNotExist(err) {
		t.Error("Expected README to be copied")
	}
}
