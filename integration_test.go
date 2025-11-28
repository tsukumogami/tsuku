//go:build integration

package main_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testMatrix represents the structure of test-matrix.json
type testMatrix struct {
	Tiers map[string][]string `json:"tiers"`
	Tests map[string]testCase `json:"tests"`
	CI    struct {
		Linux []string `json:"linux"`
		MacOS []string `json:"macos"`
	} `json:"ci"`
}

type testCase struct {
	Tool     string   `json:"tool"`
	Tier     int      `json:"tier"`
	Desc     string   `json:"desc"`
	Features []string `json:"features"`
}

const (
	dockerImage      = "tsuku-integration-test"
	dockerfilePath   = "Dockerfile.integration"
	testMatrixPath   = "test-matrix.json"
	tsukuBinaryName  = "tsuku"
	buildContextPath = "."
)

var (
	// Command-line flags for filtering tests
	toolFilter = flag.String("tool", "", "Run only tests for specific tool (e.g., -tool=actionlint)")
	tierFilter = flag.Int("tier", 0, "Run only tests for specific tier (e.g., -tier=1)")
	skipBuild  = flag.Bool("skip-build", false, "Skip Docker image build (use existing image)")
)

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

// TestIntegration runs integration tests inside Docker containers
func TestIntegration(t *testing.T) {
	// Check if Docker is available
	if err := checkDocker(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}

	// Get project root (where test-matrix.json is)
	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("Failed to find project root: %v", err)
	}

	// Build tsuku binary for Linux (container target)
	if err := buildTsukuBinary(t, projectRoot); err != nil {
		t.Fatalf("Failed to build tsuku binary: %v", err)
	}
	defer os.Remove(filepath.Join(projectRoot, tsukuBinaryName))

	// Build Docker image (unless skipped)
	if !*skipBuild {
		if err := buildDockerImage(t, projectRoot); err != nil {
			t.Fatalf("Failed to build Docker image: %v", err)
		}
	}

	// Load test matrix
	matrix, err := loadTestMatrix(projectRoot)
	if err != nil {
		t.Fatalf("Failed to load test matrix: %v", err)
	}

	// Get test IDs for current platform
	testIDs := getTestIDsForPlatform(matrix)

	// Run tests
	for _, testID := range testIDs {
		tc, ok := matrix.Tests[testID]
		if !ok {
			t.Errorf("Test case %s not found in matrix", testID)
			continue
		}

		// Apply filters
		if *toolFilter != "" && tc.Tool != *toolFilter {
			continue
		}
		if *tierFilter != 0 && tc.Tier != *tierFilter {
			continue
		}

		// Run as subtest
		t.Run(fmt.Sprintf("%s_%s", testID, tc.Tool), func(t *testing.T) {
			t.Parallel()
			runToolInstallTest(t, tc)
		})
	}
}

// checkDocker verifies Docker is installed and running
func checkDocker() error {
	cmd := exec.Command("docker", "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker not available: %w", err)
	}
	return nil
}

// findProjectRoot finds the project root directory (where go.mod is)
func findProjectRoot() (string, error) {
	// Start from current working directory
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up until we find go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod in any parent directory")
		}
		dir = parent
	}
}

// buildTsukuBinary builds the tsuku binary for Linux
func buildTsukuBinary(t *testing.T, projectRoot string) error {
	t.Log("Building tsuku binary for Linux...")

	cmd := exec.Command("go", "build", "-o", tsukuBinaryName, "./cmd/tsuku")
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build failed: %w\nStderr: %s", err, stderr.String())
	}

	t.Log("Built tsuku binary successfully")
	return nil
}

// buildDockerImage builds the integration test Docker image
func buildDockerImage(t *testing.T, projectRoot string) error {
	t.Log("Building Docker image...")

	cmd := exec.Command("docker", "build",
		"-f", dockerfilePath,
		"-t", dockerImage,
		buildContextPath,
	)
	cmd.Dir = projectRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build failed: %w\nStderr: %s", err, stderr.String())
	}

	t.Log("Built Docker image successfully")
	return nil
}

// loadTestMatrix loads and parses test-matrix.json
func loadTestMatrix(projectRoot string) (*testMatrix, error) {
	data, err := os.ReadFile(filepath.Join(projectRoot, testMatrixPath))
	if err != nil {
		return nil, fmt.Errorf("failed to read test matrix: %w", err)
	}

	var matrix testMatrix
	if err := json.Unmarshal(data, &matrix); err != nil {
		return nil, fmt.Errorf("failed to parse test matrix: %w", err)
	}

	return &matrix, nil
}

// getTestIDsForPlatform returns test IDs for the current platform
func getTestIDsForPlatform(matrix *testMatrix) []string {
	// Integration tests run in Linux containers, so use Linux test IDs
	// regardless of host platform
	return matrix.CI.Linux
}

// runToolInstallTest runs a single tool installation test in Docker
func runToolInstallTest(t *testing.T, tc testCase) {
	t.Logf("Installing %s (%s)", tc.Tool, tc.Desc)

	// Check for Linux-only tests when running on non-Linux host
	for _, feature := range tc.Features {
		if feature == "platform:linux_only" && runtime.GOOS != "linux" {
			// This is fine - we run in Linux container anyway
			t.Logf("Note: %s is Linux-only, running in container", tc.Tool)
		}
	}

	// Run docker container with tsuku install
	cmd := exec.Command("docker", "run",
		"--rm",
		"-e", fmt.Sprintf("GITHUB_TOKEN=%s", os.Getenv("GITHUB_TOKEN")),
		dockerImage,
		"install", tc.Tool,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Log output for debugging
	if stdout.Len() > 0 {
		t.Logf("stdout:\n%s", stdout.String())
	}
	if stderr.Len() > 0 {
		t.Logf("stderr:\n%s", stderr.String())
	}

	if err != nil {
		t.Errorf("Failed to install %s: %v", tc.Tool, err)
	}
}

// TestIntegrationSingle allows running a single tool test for debugging
func TestIntegrationSingle(t *testing.T) {
	if *toolFilter == "" {
		t.Skip("Use -tool flag to run single tool test (e.g., -tool=actionlint)")
	}

	// Check if Docker is available
	if err := checkDocker(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}

	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("Failed to find project root: %v", err)
	}

	// Build if not skipping
	if !*skipBuild {
		if err := buildTsukuBinary(t, projectRoot); err != nil {
			t.Fatalf("Failed to build tsuku binary: %v", err)
		}
		defer os.Remove(filepath.Join(projectRoot, tsukuBinaryName))

		if err := buildDockerImage(t, projectRoot); err != nil {
			t.Fatalf("Failed to build Docker image: %v", err)
		}
	}

	// Load matrix to get tool info
	matrix, err := loadTestMatrix(projectRoot)
	if err != nil {
		t.Fatalf("Failed to load test matrix: %v", err)
	}

	// Find the test case for the specified tool
	var foundTC *testCase
	for _, tc := range matrix.Tests {
		if tc.Tool == *toolFilter {
			foundTC = &tc
			break
		}
	}

	if foundTC == nil {
		// Tool not in matrix, run anyway with minimal info
		foundTC = &testCase{
			Tool: *toolFilter,
			Desc: "custom tool test",
		}
	}

	runToolInstallTest(t, *foundTC)
}

// TestListTools prints available tools from the test matrix
func TestListTools(t *testing.T) {
	if os.Getenv("LIST_TOOLS") != "1" {
		t.Skip("Set LIST_TOOLS=1 to list available tools")
	}

	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("Failed to find project root: %v", err)
	}

	matrix, err := loadTestMatrix(projectRoot)
	if err != nil {
		t.Fatalf("Failed to load test matrix: %v", err)
	}

	var sb strings.Builder
	sb.WriteString("\nAvailable tools for integration testing:\n")
	sb.WriteString("=========================================\n")

	for id, tc := range matrix.Tests {
		sb.WriteString(fmt.Sprintf("  %s: %s (tier %d) - %s\n", id, tc.Tool, tc.Tier, tc.Desc))
	}

	sb.WriteString("\nCI Linux tests: ")
	sb.WriteString(strings.Join(matrix.CI.Linux, ", "))
	sb.WriteString("\nCI macOS tests: ")
	sb.WriteString(strings.Join(matrix.CI.MacOS, ", "))
	sb.WriteString("\n")

	t.Log(sb.String())
}
