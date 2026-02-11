package actions

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyDirectory recursively copies a directory from src to dst, preserving symlinks
// This is the canonical implementation used by all actions
func CopyDirectory(src, dst string) error {
	return CopyDirectoryExcluding(src, dst, "")
}

// CopyDirectoryExcluding recursively copies a directory from src to dst,
// preserving symlinks and skipping any directory matching the exclude name.
// If exclude is empty, no directories are excluded.
func CopyDirectoryExcluding(src, dst, exclude string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Skip the source directory itself (walk includes it)
		if relPath == "." {
			return nil
		}

		// Skip excluded directory and its contents
		if exclude != "" && info.IsDir() && info.Name() == exclude {
			return filepath.SkipDir
		}

		// Target path
		targetPath := filepath.Join(dst, relPath)

		// Check if it's a symlink (use Lstat to not follow the link)
		linkInfo, err := os.Lstat(path)
		if err != nil {
			return err
		}

		if linkInfo.Mode()&os.ModeSymlink != 0 {
			// It's a symlink - preserve it
			return CopySymlink(path, targetPath)
		}

		if info.IsDir() {
			// Create directory
			return os.MkdirAll(targetPath, info.Mode())
		}

		// Copy file
		return CopyFile(path, targetPath, info.Mode())
	})
}

// CopySymlink copies a symlink from src to dst, preserving the link target
func CopySymlink(src, dst string) error {
	// Read the symlink target
	target, err := os.Readlink(src)
	if err != nil {
		return fmt.Errorf("failed to read symlink: %w", err)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Remove destination if it already exists
	os.Remove(dst)

	// Create the symlink
	if err := os.Symlink(target, dst); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	return nil
}

// CopyFile copies a file from src to dst with the given permissions
func CopyFile(src, dst string, mode os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer srcFile.Close()

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy: %w", err)
	}

	if err := os.Chmod(dst, mode); err != nil {
		return fmt.Errorf("failed to chmod: %w", err)
	}

	return nil
}
