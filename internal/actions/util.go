package actions

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ExpandVars replaces variables in a string with their values
// Supported variables: {version}, {os}, {arch}, {install_dir}, {work_dir}, {libs_dir}
func ExpandVars(s string, vars map[string]string) string {
	result := s
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}
	return result
}

// GetStandardVars returns standard variable mappings
func GetStandardVars(version, installDir, workDir, libsDir string) map[string]string {
	return map[string]string{
		"version":     version,
		"os":          MapOS(runtime.GOOS),
		"arch":        MapArch(runtime.GOARCH),
		"install_dir": installDir,
		"work_dir":    workDir,
		"libs_dir":    libsDir,
	}
}

// MapOS maps Go GOOS to common naming conventions
func MapOS(goos string) string {
	switch goos {
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	case "windows":
		return "windows"
	default:
		return goos
	}
}

// MapArch maps Go GOARCH to common naming conventions
func MapArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "386":
		return "386"
	default:
		return goarch
	}
}

// ApplyMapping applies custom OS/arch mapping from recipe
func ApplyMapping(value string, mapping map[string]string) string {
	if mapped, ok := mapping[value]; ok {
		return mapped
	}
	return value
}

// VerifyChecksum verifies file checksum
func VerifyChecksum(filePath, expectedChecksum, algo string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var hash []byte
	switch strings.ToLower(algo) {
	case "sha256":
		h := sha256.New()
		if _, err := io.Copy(h, file); err != nil {
			return fmt.Errorf("failed to hash file: %w", err)
		}
		hash = h.Sum(nil)
	case "sha512":
		h := sha512.New()
		if _, err := io.Copy(h, file); err != nil {
			return fmt.Errorf("failed to hash file: %w", err)
		}
		hash = h.Sum(nil)
	default:
		return fmt.Errorf("unsupported hash algorithm: %s", algo)
	}

	actualChecksum := hex.EncodeToString(hash)
	expectedChecksum = strings.TrimSpace(strings.ToLower(expectedChecksum))

	// Strip algorithm prefix if present (e.g., "sha256:abc123" -> "abc123")
	if idx := strings.Index(expectedChecksum, ":"); idx != -1 {
		expectedChecksum = expectedChecksum[idx+1:]
	}

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch:\n  expected: %s\n  actual:   %s", expectedChecksum, actualChecksum)
	}

	return nil
}

// ReadChecksumFile reads a checksum from a file
// Supports formats:
//   - Just the checksum: "abc123..."
//   - Checksum + filename: "abc123... filename.tar.gz"
//   - Multi-line with filenames (like SHA256SUMS):
//     "abc123...  file1.tar.gz"
//     "def456...  file2.tar.gz"
//
// If targetFilename is provided and the file has multiple lines,
// it searches for the line matching the target filename.
func ReadChecksumFile(path string, targetFilename ...string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read checksum file: %w", err)
	}

	content := strings.TrimSpace(string(data))
	lines := strings.Split(content, "\n")

	// If we have a target filename and multiple lines, search for matching line
	if len(targetFilename) > 0 && targetFilename[0] != "" && len(lines) > 1 {
		target := targetFilename[0]
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Check if this line contains the target filename
			// Format: "checksum  filename" or "checksum filename"
			if strings.HasSuffix(line, target) || strings.Contains(line, " "+target) || strings.Contains(line, "\t"+target) {
				// Extract checksum (first field)
				fields := strings.Fields(line)
				if len(fields) >= 1 {
					return fields[0], nil
				}
			}
		}
		return "", fmt.Errorf("checksum not found for file %q in checksum file", target)
	}

	// Single line or no target: use first line, take first field
	firstLine := lines[0]
	if idx := strings.Index(firstLine, " "); idx != -1 {
		firstLine = firstLine[:idx]
	}

	return strings.TrimSpace(firstLine), nil
}

// GetString safely gets a string value from params map
func GetString(params map[string]interface{}, key string) (string, bool) {
	val, ok := params[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

// GetInt safely gets an int value from params map
func GetInt(params map[string]interface{}, key string) (int, bool) {
	val, ok := params[key]
	if !ok {
		return 0, false
	}

	// Handle int, int64, and float64 (JSON unmarshals numbers as float64)
	switch v := val.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

// GetBool safely gets a bool value from params map
func GetBool(params map[string]interface{}, key string) (bool, bool) {
	val, ok := params[key]
	if !ok {
		return false, false
	}
	b, ok := val.(bool)
	return b, ok
}

// GetStringSlice safely gets a []string from params map
func GetStringSlice(params map[string]interface{}, key string) ([]string, bool) {
	val, ok := params[key]
	if !ok {
		return nil, false
	}

	// Handle []interface{} from TOML
	switch v := val.(type) {
	case []string:
		return v, true
	case []interface{}:
		result := make([]string, len(v))
		for i, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			result[i] = str
		}
		return result, true
	default:
		return nil, false
	}
}

// GetMapStringString safely gets a map[string]string from params map
func GetMapStringString(params map[string]interface{}, key string) (map[string]string, bool) {
	val, ok := params[key]
	if !ok {
		return nil, false
	}

	// Handle map[string]interface{} from TOML
	switch v := val.(type) {
	case map[string]string:
		return v, true
	case map[string]interface{}:
		result := make(map[string]string)
		for k, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			result[k] = str
		}
		return result, true
	default:
		return nil, false
	}
}

// ResolvePythonStandalone finds the path to tsuku's python-standalone installation
// This ensures pipx uses tsuku's managed Python, not the system Python
// Returns empty string if not found
func ResolvePythonStandalone() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Look for python-standalone in ~/.tsuku/tools/
	toolsDir := filepath.Join(homeDir, ".tsuku", "tools")
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return ""
	}

	// Find all python-standalone-* directories
	var pythonDirs []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "python-standalone-") {
			pythonDirs = append(pythonDirs, entry.Name())
		}
	}

	if len(pythonDirs) == 0 {
		return ""
	}

	// Sort to get the latest version (lexicographically)
	sort.Strings(pythonDirs)
	latestDir := pythonDirs[len(pythonDirs)-1]

	// Check if python3 exists and is executable
	pythonPath := filepath.Join(toolsDir, latestDir, "bin", "python3")
	if info, err := os.Stat(pythonPath); err == nil && info.Mode()&0111 != 0 {
		return pythonPath
	}

	return ""
}

// ResolvePipx finds the path to tsuku's pipx installation
// Returns empty string if not found
func ResolvePipx() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Look for pipx in ~/.tsuku/tools/
	toolsDir := filepath.Join(homeDir, ".tsuku", "tools")
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return ""
	}

	// Find all pipx-* directories
	var pipxDirs []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "pipx-") {
			pipxDirs = append(pipxDirs, entry.Name())
		}
	}

	if len(pipxDirs) == 0 {
		return ""
	}

	// Sort to get the latest version (lexicographically)
	sort.Strings(pipxDirs)
	latestDir := pipxDirs[len(pipxDirs)-1]

	// Check if pipx exists and is executable
	pipxPath := filepath.Join(toolsDir, latestDir, "bin", "pipx")
	if info, err := os.Stat(pipxPath); err == nil && info.Mode()&0111 != 0 {
		return pipxPath
	}

	return ""
}

// ResolveCargo finds the path to tsuku's cargo installation
// Returns empty string if not found
func ResolveCargo() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Look for rust-* directories in ~/.tsuku/tools/
	toolsDir := filepath.Join(homeDir, ".tsuku", "tools")
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return ""
	}

	// Find all rust-* directories (tsuku's rust installation)
	var rustDirs []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "rust-") {
			rustDirs = append(rustDirs, entry.Name())
		}
	}

	if len(rustDirs) == 0 {
		return ""
	}

	// Sort to get the latest version (lexicographically)
	sort.Strings(rustDirs)
	latestDir := rustDirs[len(rustDirs)-1]

	// Check if cargo exists and is executable
	// Standard location: bin/cargo (used by install.sh)
	cargoPath := filepath.Join(toolsDir, latestDir, "bin", "cargo")
	if info, err := os.Stat(cargoPath); err == nil && info.Mode()&0111 != 0 {
		return cargoPath
	}

	// Legacy location: cargo/bin/cargo (raw archive extraction)
	cargoPath = filepath.Join(toolsDir, latestDir, "cargo", "bin", "cargo")
	if info, err := os.Stat(cargoPath); err == nil && info.Mode()&0111 != 0 {
		return cargoPath
	}

	return ""
}

// ResolveGem finds the path to tsuku's gem executable (from ruby installation)
// Returns empty string if not found
func ResolveGem() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Look for ruby-* directories in ~/.tsuku/tools/
	toolsDir := filepath.Join(homeDir, ".tsuku", "tools")
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return ""
	}

	// Find all ruby-* directories (tsuku's ruby installation)
	var rubyDirs []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "ruby-") {
			rubyDirs = append(rubyDirs, entry.Name())
		}
	}

	if len(rubyDirs) == 0 {
		return ""
	}

	// Sort to get the latest version (lexicographically)
	sort.Strings(rubyDirs)
	latestDir := rubyDirs[len(rubyDirs)-1]

	// Check if gem exists and is executable
	gemPath := filepath.Join(toolsDir, latestDir, "bin", "gem")
	if info, err := os.Stat(gemPath); err == nil && info.Mode()&0111 != 0 {
		return gemPath
	}

	return ""
}

// ResolveZig finds the path to tsuku's zig executable
// Returns empty string if not found
func ResolveZig() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Look for zig-* directories in ~/.tsuku/tools/
	toolsDir := filepath.Join(homeDir, ".tsuku", "tools")
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return ""
	}

	// Find all zig-* directories (tsuku's zig installation)
	var zigDirs []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "zig-") {
			zigDirs = append(zigDirs, entry.Name())
		}
	}

	if len(zigDirs) == 0 {
		return ""
	}

	// Sort to get the latest version (lexicographically)
	sort.Strings(zigDirs)
	latestDir := zigDirs[len(zigDirs)-1]

	// Check if zig exists and is executable
	zigPath := filepath.Join(toolsDir, latestDir, "zig")
	if info, err := os.Stat(zigPath); err == nil && info.Mode()&0111 != 0 {
		return zigPath
	}

	return ""
}

// setupZigWrappers creates wrapper scripts for using zig as a C/C++ compiler
// Creates cc, c++, gcc, and g++ wrapper scripts in the specified directory
// Note: zig cc has compatibility issues with some code (like __cpu_model in GCC)
// so this is a best-effort approach for environments without a system compiler
func setupZigWrappers(zigPath, wrapperDir string) error {
	if err := os.MkdirAll(wrapperDir, 0755); err != nil {
		return err
	}

	// Create cc wrapper
	// Add -fPIC by default because zig's lld is stricter about this than GNU ld
	// This is required for building shared libraries (like Ruby native extensions)
	// Add -Wno-date-time because zig/clang treats __DATE__/__TIME__ macros as errors
	// by default (for reproducibility), but many legacy projects use them
	ccWrapper := filepath.Join(wrapperDir, "cc")
	ccContent := fmt.Sprintf("#!/bin/sh\nexec \"%s\" cc -fPIC -Wno-date-time \"$@\"\n", zigPath)
	if err := os.WriteFile(ccWrapper, []byte(ccContent), 0755); err != nil {
		return err
	}

	// Create c++ wrapper
	cxxWrapper := filepath.Join(wrapperDir, "c++")
	cxxContent := fmt.Sprintf("#!/bin/sh\nexec \"%s\" c++ -fPIC -Wno-date-time \"$@\"\n", zigPath)
	if err := os.WriteFile(cxxWrapper, []byte(cxxContent), 0755); err != nil {
		return err
	}

	// Also create gcc/g++ symlinks for compatibility (Ruby mkmf uses gcc)
	gccWrapper := filepath.Join(wrapperDir, "gcc")
	_ = os.Remove(gccWrapper)
	_ = os.Symlink(ccWrapper, gccWrapper) // Ignore error - symlink is best-effort

	gxxWrapper := filepath.Join(wrapperDir, "g++")
	_ = os.Remove(gxxWrapper)
	_ = os.Symlink(cxxWrapper, gxxWrapper) // Ignore error - symlink is best-effort

	// Create ar wrapper (zig ar)
	arWrapper := filepath.Join(wrapperDir, "ar")
	arContent := fmt.Sprintf("#!/bin/sh\nexec \"%s\" ar \"$@\"\n", zigPath)
	if err := os.WriteFile(arWrapper, []byte(arContent), 0755); err != nil {
		return err
	}

	// Create ranlib wrapper (zig ranlib)
	ranlibWrapper := filepath.Join(wrapperDir, "ranlib")
	ranlibContent := fmt.Sprintf("#!/bin/sh\nexec \"%s\" ranlib \"$@\"\n", zigPath)
	if err := os.WriteFile(ranlibWrapper, []byte(ranlibContent), 0755); err != nil {
		return err
	}

	// Create ld wrapper using zig's bundled lld
	// zig ld.lld is a drop-in replacement for GNU ld
	ldWrapper := filepath.Join(wrapperDir, "ld")
	ldContent := fmt.Sprintf("#!/bin/sh\nexec \"%s\" ld.lld \"$@\"\n", zigPath)
	if err := os.WriteFile(ldWrapper, []byte(ldContent), 0755); err != nil {
		return err
	}

	return nil
}

// hasSystemCompiler checks if gcc or cc is available in the system PATH
func hasSystemCompiler() bool {
	// Check for gcc first
	if _, err := exec.LookPath("gcc"); err == nil {
		return true
	}
	// Check for cc
	if _, err := exec.LookPath("cc"); err == nil {
		return true
	}
	return false
}

// SetupCCompilerEnv adds CC and CXX environment variables using zig as the compiler
// Creates wrapper scripts because many build systems expect CC to be a simple path
// Returns updated env slice and a boolean indicating if zig was found
func SetupCCompilerEnv(env []string) ([]string, bool) {
	zigPath := ResolveZig()
	if zigPath == "" {
		return env, false
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return env, false
	}

	wrapperDir := filepath.Join(homeDir, ".tsuku", "tools", "zig-cc-wrapper")
	if err := setupZigWrappers(zigPath, wrapperDir); err != nil {
		return env, false
	}

	ccWrapper := filepath.Join(wrapperDir, "cc")
	cxxWrapper := filepath.Join(wrapperDir, "c++")

	// Set CC, CXX and add wrapper directory to PATH
	env = append(env, fmt.Sprintf("CC=%s", ccWrapper))
	env = append(env, fmt.Sprintf("CXX=%s", cxxWrapper))

	// Prepend wrapper directory to PATH so cc/gcc are found first
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = fmt.Sprintf("PATH=%s:%s", wrapperDir, e[5:])
			return env, true
		}
	}
	// If PATH not found in env, add it
	env = append(env, fmt.Sprintf("PATH=%s:%s", wrapperDir, os.Getenv("PATH")))

	return env, true
}

// ResolvePerl finds the path to tsuku's perl executable
// Returns empty string if not found
func ResolvePerl() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Look for perl-* directories in ~/.tsuku/tools/
	toolsDir := filepath.Join(homeDir, ".tsuku", "tools")
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return ""
	}

	// Find all perl-* directories (tsuku's perl installation)
	var perlDirs []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "perl-") {
			perlDirs = append(perlDirs, entry.Name())
		}
	}

	if len(perlDirs) == 0 {
		return ""
	}

	// Sort to get the latest version (lexicographically)
	sort.Strings(perlDirs)
	latestDir := perlDirs[len(perlDirs)-1]

	// Check if perl exists and is executable
	perlPath := filepath.Join(toolsDir, latestDir, "bin", "perl")
	if info, err := os.Stat(perlPath); err == nil && info.Mode()&0111 != 0 {
		return perlPath
	}

	return ""
}

// ResolveCpanm finds the path to tsuku's cpanm executable (from perl installation)
// Returns empty string if not found
func ResolveCpanm() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Look for perl-* directories in ~/.tsuku/tools/
	toolsDir := filepath.Join(homeDir, ".tsuku", "tools")
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return ""
	}

	// Find all perl-* directories (tsuku's perl installation)
	var perlDirs []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "perl-") {
			perlDirs = append(perlDirs, entry.Name())
		}
	}

	if len(perlDirs) == 0 {
		return ""
	}

	// Sort to get the latest version (lexicographically)
	sort.Strings(perlDirs)
	latestDir := perlDirs[len(perlDirs)-1]

	// Check if cpanm exists and is executable
	cpanmPath := filepath.Join(toolsDir, latestDir, "bin", "cpanm")
	if info, err := os.Stat(cpanmPath); err == nil && info.Mode()&0111 != 0 {
		return cpanmPath
	}

	return ""
}

// ResolveGo finds the path to tsuku's go executable
// Returns empty string if not found
func ResolveGo() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Look for go-* directories in $TSUKU_HOME/tools/
	toolsDir := filepath.Join(homeDir, ".tsuku", "tools")
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return ""
	}

	// Find all go-<version> directories (tsuku's go installation)
	// Must match go-<digit...> to avoid matching tools like go-migrate, go-task
	var goDirs []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "go-") {
			// Skip tool directories like go-migrate, go-task, etc.
			// Go toolchain versions start with a digit (e.g., go-1.23.4)
			rest := strings.TrimPrefix(entry.Name(), "go-")
			if len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
				goDirs = append(goDirs, entry.Name())
			}
		}
	}

	if len(goDirs) == 0 {
		return ""
	}

	// Sort to get the latest version (lexicographically)
	sort.Strings(goDirs)
	latestDir := goDirs[len(goDirs)-1]

	// Check if go exists and is executable
	goPath := filepath.Join(toolsDir, latestDir, "bin", "go")
	if info, err := os.Stat(goPath); err == nil && info.Mode()&0111 != 0 {
		return goPath
	}

	return ""
}

// ResolveGoVersion finds the path to a specific Go version installed by tsuku.
// Returns empty string if the specific version is not found.
// The version should be in format "1.23.4" (without "go" prefix).
func ResolveGoVersion(version string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Look for go-<version> directory in $TSUKU_HOME/tools/
	toolsDir := filepath.Join(homeDir, ".tsuku", "tools")
	goPath := filepath.Join(toolsDir, "go-"+version, "bin", "go")

	if info, err := os.Stat(goPath); err == nil && info.Mode()&0111 != 0 {
		return goPath
	}

	return ""
}

// GetGoVersion extracts the Go version string from a Go binary path.
// Returns the version (e.g., "1.23.4") by running `go version`.
// Returns empty string on error.
func GetGoVersion(goPath string) string {
	cmd := exec.Command(goPath, "version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Output format: "go version go1.23.4 linux/amd64"
	fields := strings.Fields(string(output))
	if len(fields) < 3 {
		return ""
	}

	// Extract version from "go1.23.4"
	goVersion := fields[2]
	if strings.HasPrefix(goVersion, "go") {
		return strings.TrimPrefix(goVersion, "go")
	}

	return ""
}
