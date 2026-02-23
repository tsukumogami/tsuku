// Package sandbox provides container-based sandbox testing for recipes.
// It derives container configuration from installation plans by analyzing
// the network requirements and complexity of each action.
package sandbox

import (
	"time"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/containerimages"
	"github.com/tsukumogami/tsuku/internal/executor"
)

// SourceBuildSandboxImage is the container image for source builds and
// ecosystem packages. Uses Ubuntu for better package availability.
const SourceBuildSandboxImage = "ubuntu:22.04"

// ResourceLimits defines container resource constraints for sandbox testing.
type ResourceLimits struct {
	Memory   string        // Memory limit (e.g., "2g")
	CPUs     string        // CPU limit (e.g., "2")
	PidsMax  int           // Maximum number of processes
	Timeout  time.Duration // Execution timeout
	ReadOnly bool          // Read-only root filesystem
}

// DefaultLimits returns resource limits suitable for simple binary installations.
func DefaultLimits() ResourceLimits {
	return ResourceLimits{
		Memory:   "2g",
		CPUs:     "2",
		PidsMax:  100,
		Timeout:  2 * time.Minute,
		ReadOnly: false, // Need to install packages
	}
}

// SourceBuildLimits returns resource limits suitable for source builds.
// Source builds need more time and resources than binary extraction.
func SourceBuildLimits() ResourceLimits {
	return ResourceLimits{
		Memory:   "4g",
		CPUs:     "4",
		PidsMax:  500,
		Timeout:  15 * time.Minute,
		ReadOnly: false,
	}
}

// SandboxRequirements describes what a sandbox container needs.
// These requirements are derived from analyzing the installation plan.
//
// Note: Build tools are NOT tracked here - tsuku's normal dependency resolution
// handles them via ActionDependencies.InstallTime (see DESIGN-dependency-provisioning.md).
type SandboxRequirements struct {
	// RequiresNetwork is true if any step needs network access.
	// Actions like cargo_build, go_build, npm_install need network for dependencies.
	RequiresNetwork bool

	// Image is the recommended container image based on requirements.
	// Uses the default image from container-images.json for binary-only installs,
	// or ubuntu:22.04 for source builds.
	Image string

	// Resources are the recommended resource limits.
	Resources ResourceLimits

	// ExtraEnv holds additional environment variables for the container in
	// KEY=VALUE format. These are appended to RunOptions.Env after the
	// hardcoded sandbox variables. Keys that collide with hardcoded vars
	// (TSUKU_SANDBOX, TSUKU_HOME, HOME, DEBIAN_FRONTEND, PATH) are
	// silently dropped to prevent subverting the sandbox environment.
	ExtraEnv []string
}

// ComputeSandboxRequirements derives container requirements from a plan.
// It iterates through plan steps, querying each action's RequiresNetwork() method
// via the NetworkValidator interface, and aggregates the results.
//
// The function selects appropriate container image and resource limits based on:
//   - Target family: selects family-specific base image when targetFamily is set
//   - Network requirements: any network-requiring action upgrades resources
//   - Build complexity: build actions (configure_make, cmake_build, etc.) get more resources
//
// When targetFamily is empty, defaults to Debian-based images for backward compatibility.
func ComputeSandboxRequirements(plan *executor.InstallationPlan, targetFamily string) *SandboxRequirements {
	defaultImage := containerimages.DefaultImage()
	buildImage := SourceBuildSandboxImage
	if img, ok := containerimages.ImageForFamily(targetFamily); ok {
		defaultImage = img
		buildImage = img
	}

	reqs := &SandboxRequirements{
		RequiresNetwork: false,
		Image:           defaultImage,
		Resources:       DefaultLimits(),
	}

	if plan == nil {
		return reqs
	}

	// Check if any step requires network by querying the action
	for _, step := range plan.Steps {
		action := actions.Get(step.Action)
		if action == nil {
			// Unknown action - fail closed (no network) for security
			continue
		}

		// Check if action implements NetworkValidator
		if nv, ok := action.(actions.NetworkValidator); ok {
			if nv.RequiresNetwork() {
				reqs.RequiresNetwork = true
				// Don't break - we still need to check for build actions
			}
		}
	}

	// Upgrade image and resources for network-requiring (ecosystem) builds
	// Network-requiring steps typically involve compilation which needs more resources
	if reqs.RequiresNetwork {
		reqs.Image = buildImage
		reqs.Resources = SourceBuildLimits()
	}

	// Also upgrade for plans with known build actions (even if offline)
	// Build actions like configure_make may not need network but still need
	// more resources and a fuller base image. Network is required because
	// the sandbox needs apt-get to install build-essential.
	if hasBuildActions(plan) {
		reqs.RequiresNetwork = true
		reqs.Image = buildImage
		reqs.Resources = SourceBuildLimits()
	}

	return reqs
}

// buildActions lists actions that involve compilation and need extra resources.
var buildActions = map[string]bool{
	"configure_make": true,
	"cmake_build":    true,
	"cargo_build":    true,
	"go_build":       true,
}

// hasBuildActions checks if plan contains compilation steps.
// These steps need more resources even if they don't require network
// (e.g., configure_make with vendored dependencies).
func hasBuildActions(plan *executor.InstallationPlan) bool {
	for _, step := range plan.Steps {
		if buildActions[step.Action] {
			return true
		}
	}
	return false
}
