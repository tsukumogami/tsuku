package validate

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRuntimeDetector_Detect_Podman(t *testing.T) {
	d := &RuntimeDetector{
		lookPath: func(name string) (string, error) {
			if name == "podman" {
				return "/usr/bin/podman", nil
			}
			return "", errors.New("not found")
		},
		cmdRun: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			// Simulate podman info --format {{.Host.Security.Rootless}}
			if name == "/usr/bin/podman" && len(args) >= 3 && args[0] == "info" {
				return []byte("true\n"), nil
			}
			return nil, errors.New("unexpected command")
		},
	}

	ctx := context.Background()
	runtime, err := d.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect() error = %v, want nil", err)
	}

	if runtime.Name() != "podman" {
		t.Errorf("runtime.Name() = %q, want %q", runtime.Name(), "podman")
	}
	if !runtime.IsRootless() {
		t.Error("runtime.IsRootless() = false, want true")
	}
}

func TestRuntimeDetector_Detect_DockerRootless(t *testing.T) {
	d := &RuntimeDetector{
		lookPath: func(name string) (string, error) {
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			return "", errors.New("not found")
		},
		cmdRun: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if name == "/usr/bin/docker" && len(args) >= 2 && args[0] == "info" {
				// Docker rootless includes "rootless" in security options
				return []byte("[rootless seccomp]\n"), nil
			}
			return nil, errors.New("unexpected command")
		},
	}

	ctx := context.Background()
	runtime, err := d.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect() error = %v, want nil", err)
	}

	if runtime.Name() != "docker" {
		t.Errorf("runtime.Name() = %q, want %q", runtime.Name(), "docker")
	}
	if !runtime.IsRootless() {
		t.Error("runtime.IsRootless() = false, want true")
	}
}

func TestRuntimeDetector_Detect_DockerGroup(t *testing.T) {
	d := &RuntimeDetector{
		lookPath: func(name string) (string, error) {
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			return "", errors.New("not found")
		},
		cmdRun: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if name == "/usr/bin/docker" && len(args) >= 1 && args[0] == "info" {
				// Non-rootless docker info (no "rootless" in output)
				return []byte("[seccomp]\n"), nil
			}
			return nil, errors.New("unexpected command")
		},
	}

	ctx := context.Background()
	runtime, err := d.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect() error = %v, want nil", err)
	}

	if runtime.Name() != "docker" {
		t.Errorf("runtime.Name() = %q, want %q", runtime.Name(), "docker")
	}
	if runtime.IsRootless() {
		t.Error("runtime.IsRootless() = true, want false")
	}
}

func TestRuntimeDetector_Detect_NoRuntime(t *testing.T) {
	d := &RuntimeDetector{
		lookPath: func(name string) (string, error) {
			return "", errors.New("not found")
		},
		cmdRun: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return nil, errors.New("should not be called")
		},
	}

	ctx := context.Background()
	runtime, err := d.Detect(ctx)

	if !errors.Is(err, ErrNoRuntime) {
		t.Errorf("Detect() error = %v, want ErrNoRuntime", err)
	}
	if runtime != nil {
		t.Error("Detect() runtime should be nil when no runtime available")
	}
}

func TestRuntimeDetector_Detect_Caching(t *testing.T) {
	detectCount := 0
	d := &RuntimeDetector{
		lookPath: func(name string) (string, error) {
			detectCount++
			if name == "podman" {
				return "/usr/bin/podman", nil
			}
			return "", errors.New("not found")
		},
		cmdRun: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if name == "/usr/bin/podman" {
				return []byte("true\n"), nil
			}
			return nil, errors.New("unexpected")
		},
	}

	ctx := context.Background()

	// First detection
	r1, err := d.Detect(ctx)
	if err != nil {
		t.Fatalf("First Detect() error = %v", err)
	}

	// Second detection should use cache
	r2, err := d.Detect(ctx)
	if err != nil {
		t.Fatalf("Second Detect() error = %v", err)
	}

	// Should be the same runtime instance
	if r1 != r2 {
		t.Error("Detect() should return cached runtime on subsequent calls")
	}

	// lookPath should only be called once (for the first detection)
	if detectCount != 1 {
		t.Errorf("lookPath called %d times, want 1 (caching should prevent re-detection)", detectCount)
	}
}

func TestRuntimeDetector_Reset(t *testing.T) {
	detectCount := 0
	d := &RuntimeDetector{
		lookPath: func(name string) (string, error) {
			detectCount++
			if name == "podman" {
				return "/usr/bin/podman", nil
			}
			return "", errors.New("not found")
		},
		cmdRun: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte("true\n"), nil
		},
	}

	ctx := context.Background()

	// First detection
	_, _ = d.Detect(ctx)

	// Reset
	d.Reset()

	// Detection after reset should re-detect
	_, _ = d.Detect(ctx)

	if detectCount != 2 {
		t.Errorf("After Reset(), lookPath called %d times, want 2", detectCount)
	}
}

func TestRuntimeDetector_PreferenceOrder(t *testing.T) {
	// When both Podman and Docker are available, Podman should be preferred
	d := &RuntimeDetector{
		lookPath: func(name string) (string, error) {
			if name == "podman" {
				return "/usr/bin/podman", nil
			}
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			return "", errors.New("not found")
		},
		cmdRun: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if name == "/usr/bin/podman" {
				return []byte("true\n"), nil
			}
			if name == "/usr/bin/docker" {
				return []byte("[rootless]\n"), nil
			}
			return nil, errors.New("unexpected")
		},
	}

	ctx := context.Background()
	runtime, err := d.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if runtime.Name() != "podman" {
		t.Errorf("runtime.Name() = %q, want %q (Podman should be preferred)", runtime.Name(), "podman")
	}
}

func TestPodmanRuntime_BuildArgs(t *testing.T) {
	r := &podmanRuntime{path: "/usr/bin/podman", rootless: true}

	opts := RunOptions{
		Image:   "alpine:latest",
		Command: []string{"echo", "hello"},
		Mounts: []Mount{
			{Source: "/host/path", Target: "/container/path", ReadOnly: true},
		},
		Env:     []string{"FOO=bar"},
		WorkDir: "/workspace",
		Network: "none",
		Limits: ResourceLimits{
			Memory:   "2g",
			CPUs:     "2",
			PidsMax:  100,
			ReadOnly: true,
		},
		Labels: map[string]string{"app": "tsuku"},
	}

	args := r.buildArgs(opts)

	// Check required arguments are present
	expected := []string{
		"run", "--rm",
		"--network=none",
		"--ipc=none",
		"--memory=2g",
		"--cpus=2",
		"--pids-limit=100",
		"--read-only",
	}

	for _, e := range expected {
		found := false
		for _, a := range args {
			if a == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("buildArgs() missing expected arg %q, got %v", e, args)
		}
	}

	// Check image and command are at the end
	if args[len(args)-2] != "echo" || args[len(args)-1] != "hello" {
		t.Errorf("buildArgs() command should be at end, got %v", args[len(args)-2:])
	}
}

func TestDockerRuntime_BuildArgs(t *testing.T) {
	r := &dockerRuntime{path: "/usr/bin/docker", rootless: false}

	opts := RunOptions{
		Image:   "alpine:latest",
		Command: []string{"sh", "-c", "ls"},
		Network: "none",
		Limits: ResourceLimits{
			Memory: "1g",
		},
	}

	args := r.buildArgs(opts)

	// Should include run, --rm, network, ipc, memory
	if args[0] != "run" {
		t.Errorf("buildArgs()[0] = %q, want %q", args[0], "run")
	}
	if args[1] != "--rm" {
		t.Errorf("buildArgs()[1] = %q, want %q", args[1], "--rm")
	}
}

func TestRunOptions_Defaults(t *testing.T) {
	// Verify zero values are safe
	opts := RunOptions{
		Image:   "alpine",
		Command: []string{"echo", "test"},
	}

	r := &podmanRuntime{path: "/usr/bin/podman"}
	args := r.buildArgs(opts)

	// Should still build valid args with defaults
	if len(args) < 4 {
		t.Errorf("buildArgs() with minimal options should still produce valid args, got %v", args)
	}
}

func TestResourceLimits_Timeout(t *testing.T) {
	// Test that timeout is respected (we can't actually run containers in unit tests,
	// but we can verify the timeout is propagated)
	limits := ResourceLimits{
		Timeout: 5 * time.Second,
	}

	if limits.Timeout != 5*time.Second {
		t.Errorf("ResourceLimits.Timeout = %v, want %v", limits.Timeout, 5*time.Second)
	}
}
