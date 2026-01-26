package actions

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// DependencyMissingError indicates required system packages are not installed.
// The CLI uses this to provide specialized output with aggregated install commands.
type DependencyMissingError struct {
	Packages []string // Missing package names
	Command  string   // Install command (from Describe())
	Family   string   // Linux family (alpine, debian, rhel, arch, suse)
}

// Error implements the error interface.
func (e *DependencyMissingError) Error() string {
	return fmt.Sprintf("missing system packages: %v (install with: %s)", e.Packages, e.Command)
}

// IsDependencyMissing checks if an error is a DependencyMissingError.
func IsDependencyMissing(err error) bool {
	var depErr *DependencyMissingError
	return errors.As(err, &depErr)
}

// AsDependencyMissing extracts the DependencyMissingError from an error.
// Returns nil if the error is not a DependencyMissingError.
func AsDependencyMissing(err error) *DependencyMissingError {
	var depErr *DependencyMissingError
	if errors.As(err, &depErr) {
		return depErr
	}
	return nil
}

// isPackageInstalled checks if a package is installed using read-only queries.
// These commands do not require elevated privileges.
func isPackageInstalled(pkg string, family string) bool {
	switch family {
	case "alpine":
		// apk info -e returns 0 if package is installed
		cmd := exec.Command("apk", "info", "-e", pkg)
		return cmd.Run() == nil
	case "debian":
		// dpkg-query returns status; check for "install ok installed"
		cmd := exec.Command("dpkg-query", "-W", "-f=${Status}", pkg)
		out, err := cmd.Output()
		return err == nil && strings.Contains(string(out), "install ok installed")
	case "rhel", "suse":
		// rpm -q returns 0 if package is installed
		cmd := exec.Command("rpm", "-q", pkg)
		return cmd.Run() == nil
	case "arch":
		// pacman -Q returns 0 if package is installed
		cmd := exec.Command("pacman", "-Q", pkg)
		return cmd.Run() == nil
	}
	return false
}

// checkMissingPackages returns a list of packages that are not installed.
func checkMissingPackages(packages []string, family string) []string {
	var missing []string
	for _, pkg := range packages {
		if !isPackageInstalled(pkg, family) {
			missing = append(missing, pkg)
		}
	}
	return missing
}

// getRootPrefix returns "sudo " or "doas " if not running as root,
// or empty string if already root.
func getRootPrefix() string {
	if os.Getuid() == 0 {
		return ""
	}

	// Prefer doas if available (common on Alpine/BSD)
	if _, err := exec.LookPath("doas"); err == nil {
		return "doas "
	}

	return "sudo "
}

// buildInstallCommand creates the install command with appropriate root prefix.
func buildInstallCommand(baseCmd string, packages []string) string {
	prefix := getRootPrefix()
	return prefix + baseCmd + " " + strings.Join(packages, " ")
}
