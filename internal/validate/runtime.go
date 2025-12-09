// Package validate provides container-based validation for recipes.
package validate

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ErrNoRuntime is returned when no container runtime is available.
var ErrNoRuntime = errors.New("no container runtime available")

// Runtime represents a container runtime (Podman or Docker).
type Runtime interface {
	// Name returns the runtime name ("podman" or "docker").
	Name() string

	// IsRootless returns true if the runtime is running in rootless mode.
	IsRootless() bool

	// Run executes a container with the given options.
	Run(ctx context.Context, opts RunOptions) (*RunResult, error)
}

// RunOptions configures a container run.
type RunOptions struct {
	Image   string            // Container image (e.g., "alpine:latest")
	Command []string          // Command to run inside container
	Mounts  []Mount           // Volume mounts
	Env     []string          // Environment variables (KEY=value format)
	WorkDir string            // Working directory inside container
	Network string            // Network mode ("none", "host", etc.)
	Limits  ResourceLimits    // Resource limits
	Labels  map[string]string // Container labels
}

// Mount represents a volume mount.
type Mount struct {
	Source   string // Host path
	Target   string // Container path
	ReadOnly bool   // Mount as read-only
}

// ResourceLimits defines container resource constraints.
type ResourceLimits struct {
	Memory   string        // Memory limit (e.g., "2g")
	CPUs     string        // CPU limit (e.g., "2")
	PidsMax  int           // Maximum number of processes
	Timeout  time.Duration // Execution timeout
	ReadOnly bool          // Read-only root filesystem
}

// RunResult contains the result of a container run.
type RunResult struct {
	ExitCode int    // Container exit code
	Stdout   string // Standard output
	Stderr   string // Standard error
}

// RuntimeDetector detects available container runtimes.
type RuntimeDetector struct {
	mu       sync.RWMutex
	detected Runtime
	checked  bool

	// For testing: allow overriding command execution
	lookPath func(string) (string, error)
	cmdRun   func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// NewRuntimeDetector creates a new runtime detector.
func NewRuntimeDetector() *RuntimeDetector {
	return &RuntimeDetector{
		lookPath: exec.LookPath,
		cmdRun:   defaultCmdRun,
	}
}

// defaultCmdRun executes a command and returns its combined output.
func defaultCmdRun(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// Detect returns the best available container runtime, or ErrNoRuntime if none available.
// Results are cached after the first detection.
func (d *RuntimeDetector) Detect(ctx context.Context) (Runtime, error) {
	d.mu.RLock()
	if d.checked {
		r := d.detected
		d.mu.RUnlock()
		if r == nil {
			return nil, ErrNoRuntime
		}
		return r, nil
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()

	// Double-check after acquiring write lock
	if d.checked {
		if d.detected == nil {
			return nil, ErrNoRuntime
		}
		return d.detected, nil
	}

	// Try runtimes in preference order
	// 1. Podman (preferred for native rootless support)
	if r := d.tryPodman(ctx); r != nil {
		d.detected = r
		d.checked = true
		return r, nil
	}

	// 2. Docker rootless
	if r := d.tryDockerRootless(ctx); r != nil {
		d.detected = r
		d.checked = true
		return r, nil
	}

	// 3. Docker with group membership
	if r := d.tryDockerGroup(ctx); r != nil {
		d.detected = r
		d.checked = true
		return r, nil
	}

	// No runtime available
	d.checked = true
	return nil, ErrNoRuntime
}

// Reset clears the cached detection result, forcing re-detection on next Detect call.
func (d *RuntimeDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.detected = nil
	d.checked = false
}

// tryPodman attempts to detect and verify Podman.
func (d *RuntimeDetector) tryPodman(ctx context.Context) Runtime {
	// Check if podman binary exists
	path, err := d.lookPath("podman")
	if err != nil {
		return nil
	}

	// Check if podman can run rootless by checking podman info
	output, err := d.cmdRun(ctx, path, "info", "--format", "{{.Host.Security.Rootless}}")
	if err != nil {
		return nil
	}

	isRootless := strings.TrimSpace(string(output)) == "true"

	return &podmanRuntime{
		path:     path,
		rootless: isRootless,
	}
}

// tryDockerRootless attempts to detect Docker in rootless mode.
func (d *RuntimeDetector) tryDockerRootless(ctx context.Context) Runtime {
	path, err := d.lookPath("docker")
	if err != nil {
		return nil
	}

	// Check if Docker is running in rootless mode
	// Docker rootless uses a user-owned socket
	output, err := d.cmdRun(ctx, path, "info", "--format", "{{.SecurityOptions}}")
	if err != nil {
		return nil
	}

	// Rootless Docker includes "rootless" in security options
	if strings.Contains(string(output), "rootless") {
		return &dockerRuntime{
			path:     path,
			rootless: true,
		}
	}

	return nil
}

// tryDockerGroup attempts to detect Docker accessible via group membership.
func (d *RuntimeDetector) tryDockerGroup(ctx context.Context) Runtime {
	path, err := d.lookPath("docker")
	if err != nil {
		return nil
	}

	// Check if current user is in docker group or can access docker
	if !d.canAccessDocker(ctx, path) {
		return nil
	}

	return &dockerRuntime{
		path:     path,
		rootless: false,
	}
}

// canAccessDocker checks if the current user can access Docker.
func (d *RuntimeDetector) canAccessDocker(ctx context.Context, dockerPath string) bool {
	// Try to run docker info - this will fail if user doesn't have access
	_, err := d.cmdRun(ctx, dockerPath, "info")
	return err == nil
}

// podmanRuntime implements Runtime for Podman.
type podmanRuntime struct {
	path     string
	rootless bool
}

func (r *podmanRuntime) Name() string {
	return "podman"
}

func (r *podmanRuntime) IsRootless() bool {
	return r.rootless
}

func (r *podmanRuntime) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	args := r.buildArgs(opts)

	// Apply timeout if specified
	if opts.Limits.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Limits.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, r.path, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := &RunResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			return result, fmt.Errorf("container execution timed out: %w", err)
		} else {
			return result, fmt.Errorf("container execution failed: %w", err)
		}
	}

	return result, nil
}

func (r *podmanRuntime) buildArgs(opts RunOptions) []string {
	args := []string{"run", "--rm"}

	// Network isolation
	if opts.Network != "" {
		args = append(args, "--network="+opts.Network)
	}

	// IPC isolation
	args = append(args, "--ipc=none")

	// Resource limits
	if opts.Limits.Memory != "" {
		args = append(args, "--memory="+opts.Limits.Memory)
	}
	if opts.Limits.CPUs != "" {
		args = append(args, "--cpus="+opts.Limits.CPUs)
	}
	if opts.Limits.PidsMax > 0 {
		args = append(args, fmt.Sprintf("--pids-limit=%d", opts.Limits.PidsMax))
	}
	if opts.Limits.ReadOnly {
		args = append(args, "--read-only")
		// Add writable /tmp for most use cases
		args = append(args, "--tmpfs", "/tmp:rw,size=1g")
	}

	// Mounts
	for _, m := range opts.Mounts {
		mountStr := m.Source + ":" + m.Target
		if m.ReadOnly {
			mountStr += ":ro"
		}
		args = append(args, "-v", mountStr)
	}

	// Environment variables
	for _, env := range opts.Env {
		args = append(args, "-e", env)
	}

	// Working directory
	if opts.WorkDir != "" {
		args = append(args, "-w", opts.WorkDir)
	}

	// Labels
	for k, v := range opts.Labels {
		args = append(args, "--label", k+"="+v)
	}

	// Image
	args = append(args, opts.Image)

	// Command
	args = append(args, opts.Command...)

	return args
}

// dockerRuntime implements Runtime for Docker.
type dockerRuntime struct {
	path     string
	rootless bool
}

func (r *dockerRuntime) Name() string {
	return "docker"
}

func (r *dockerRuntime) IsRootless() bool {
	return r.rootless
}

func (r *dockerRuntime) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	args := r.buildArgs(opts)

	// Apply timeout if specified
	if opts.Limits.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Limits.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, r.path, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := &RunResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			return result, fmt.Errorf("container execution timed out: %w", err)
		} else {
			return result, fmt.Errorf("container execution failed: %w", err)
		}
	}

	return result, nil
}

func (r *dockerRuntime) buildArgs(opts RunOptions) []string {
	args := []string{"run", "--rm"}

	// Network isolation
	if opts.Network != "" {
		args = append(args, "--network="+opts.Network)
	}

	// IPC isolation
	args = append(args, "--ipc=none")

	// Resource limits
	if opts.Limits.Memory != "" {
		args = append(args, "--memory="+opts.Limits.Memory)
	}
	if opts.Limits.CPUs != "" {
		args = append(args, "--cpus="+opts.Limits.CPUs)
	}
	if opts.Limits.PidsMax > 0 {
		args = append(args, fmt.Sprintf("--pids-limit=%d", opts.Limits.PidsMax))
	}
	if opts.Limits.ReadOnly {
		args = append(args, "--read-only")
		// Add writable /tmp for most use cases
		args = append(args, "--tmpfs", "/tmp:rw,size=1g")
	}

	// Mounts
	for _, m := range opts.Mounts {
		mountStr := m.Source + ":" + m.Target
		if m.ReadOnly {
			mountStr += ":ro"
		}
		args = append(args, "-v", mountStr)
	}

	// Environment variables
	for _, env := range opts.Env {
		args = append(args, "-e", env)
	}

	// Working directory
	if opts.WorkDir != "" {
		args = append(args, "-w", opts.WorkDir)
	}

	// Labels
	for k, v := range opts.Labels {
		args = append(args, "--label", k+"="+v)
	}

	// Image
	args = append(args, opts.Image)

	// Command
	args = append(args, opts.Command...)

	return args
}
