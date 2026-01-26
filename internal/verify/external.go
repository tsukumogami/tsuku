package verify

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// ExternalLibraryInfo holds information about an externally-managed library.
type ExternalLibraryInfo struct {
	// Packages lists the system packages that provide this library.
	Packages []string
	// Family is the Linux family (alpine, debian, rhel, arch, suse).
	Family string
	// LibraryFiles are the discovered shared library files.
	LibraryFiles []string
}

// packageManagerActions maps action names to their Linux family.
var packageManagerActions = map[string]string{
	"apk_install":    "alpine",
	"apt_install":    "debian",
	"dnf_install":    "rhel",
	"pacman_install": "arch",
	"zypper_install": "suse",
}

// CheckExternalLibrary checks if a library recipe is satisfied by system packages.
// It examines the recipe's steps to find package manager actions that match the
// current platform, verifies the packages are installed, and discovers the library files.
//
// Returns nil if the library is not externally managed or packages are not installed.
func CheckExternalLibrary(r *recipe.Recipe, target platform.Target) (*ExternalLibraryInfo, error) {
	if !r.IsLibrary() {
		return nil, nil
	}

	// Find package manager steps that match the current platform
	for _, step := range r.Steps {
		family, isPackageManager := packageManagerActions[step.Action]
		if !isPackageManager {
			continue
		}

		// Check if step matches current platform
		if step.When != nil && !step.When.Matches(target) {
			continue
		}

		// Also verify the target's linux family matches the action's expected family
		if target.LinuxFamily() != family {
			continue
		}

		// Get packages from step params
		packages := getPackagesFromParams(step.Params)
		if len(packages) == 0 {
			continue
		}

		// Check if all packages are installed
		if !allPackagesInstalled(packages, family) {
			return nil, nil // Packages not installed
		}

		// Discover library files from installed packages
		libFiles, err := discoverLibraryFiles(packages, family)
		if err != nil {
			return nil, fmt.Errorf("failed to discover library files: %w", err)
		}

		return &ExternalLibraryInfo{
			Packages:     packages,
			Family:       family,
			LibraryFiles: libFiles,
		}, nil
	}

	return nil, nil // No matching package manager steps
}

// getPackagesFromParams extracts the packages list from step parameters.
func getPackagesFromParams(params map[string]interface{}) []string {
	pkgs, ok := params["packages"]
	if !ok {
		return nil
	}

	switch v := pkgs.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// allPackagesInstalled checks if all packages are installed on the system.
func allPackagesInstalled(packages []string, family string) bool {
	for _, pkg := range packages {
		if !isPackageInstalled(pkg, family) {
			return false
		}
	}
	return true
}

// isPackageInstalled checks if a package is installed using read-only queries.
func isPackageInstalled(pkg string, family string) bool {
	switch family {
	case "alpine":
		cmd := exec.Command("apk", "info", "-e", pkg)
		return cmd.Run() == nil
	case "debian":
		cmd := exec.Command("dpkg-query", "-W", "-f=${Status}", pkg)
		out, err := cmd.Output()
		return err == nil && strings.Contains(string(out), "install ok installed")
	case "rhel", "suse":
		cmd := exec.Command("rpm", "-q", pkg)
		return cmd.Run() == nil
	case "arch":
		cmd := exec.Command("pacman", "-Q", pkg)
		return cmd.Run() == nil
	}
	return false
}

// discoverLibraryFiles queries the package manager to find library files.
func discoverLibraryFiles(packages []string, family string) ([]string, error) {
	var allFiles []string

	for _, pkg := range packages {
		files, err := listPackageFiles(pkg, family)
		if err != nil {
			return nil, fmt.Errorf("failed to list files for package %s: %w", pkg, err)
		}

		// Filter to only shared library files
		for _, f := range files {
			if isSharedLibraryPath(f) {
				allFiles = append(allFiles, f)
			}
		}
	}

	// Deduplicate and resolve to real files (not symlinks)
	return deduplicateLibraries(allFiles)
}

// listPackageFiles queries the package manager to list files installed by a package.
func listPackageFiles(pkg string, family string) ([]string, error) {
	var cmd *exec.Cmd

	switch family {
	case "alpine":
		// apk info -L lists files owned by the package
		cmd = exec.Command("apk", "info", "-L", pkg)
	case "debian":
		// dpkg -L lists files owned by the package
		cmd = exec.Command("dpkg", "-L", pkg)
	case "rhel", "suse":
		// rpm -ql lists files owned by the package
		cmd = exec.Command("rpm", "-ql", pkg)
	case "arch":
		// pacman -Ql lists files with package prefix
		cmd = exec.Command("pacman", "-Ql", pkg)
	default:
		return nil, fmt.Errorf("unsupported package manager family: %s", family)
	}

	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// pacman -Ql format: "package /path/to/file"
		if family == "arch" {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 {
				line = parts[1]
			}
		}

		// apk info -L format: first line is "pkgname-version contains:", then paths
		// Paths may or may not have leading slash
		if family == "alpine" {
			// Skip the header line (contains "contains:")
			if strings.Contains(line, " contains:") {
				continue
			}
			// Prepend / if path doesn't start with it
			if !strings.HasPrefix(line, "/") {
				line = "/" + line
			}
		}

		files = append(files, line)
	}

	return files, nil
}

// isSharedLibraryPath checks if a path looks like a shared library.
func isSharedLibraryPath(path string) bool {
	name := filepath.Base(path)

	// macOS dynamic libraries
	if strings.HasSuffix(name, ".dylib") {
		return true
	}

	// Linux shared objects: libfoo.so, libfoo.so.1, libfoo.so.1.2.3
	if strings.Contains(name, ".so") {
		idx := strings.Index(name, ".so")
		suffix := name[idx+3:]
		if suffix == "" {
			return true
		}
		// Check if suffix is version-like: .1, .1.2, .1.2.3, etc.
		if len(suffix) > 0 && suffix[0] == '.' {
			for _, c := range suffix[1:] {
				if c != '.' && (c < '0' || c > '9') {
					return false
				}
			}
			return true
		}
	}

	return false
}

// deduplicateLibraries resolves symlinks and returns unique real library files.
func deduplicateLibraries(paths []string) ([]string, error) {
	seen := make(map[string]bool)
	var result []string

	for _, path := range paths {
		// Resolve symlinks to get the real file
		realPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			// Skip files that can't be resolved (broken symlinks, etc.)
			continue
		}

		if !seen[realPath] {
			seen[realPath] = true
			result = append(result, realPath)
		}
	}

	return result, nil
}
