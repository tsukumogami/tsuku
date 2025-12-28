// Package sandbox provides container-based sandbox testing for recipes.
// It derives container configuration from installation plans by analyzing
// the network requirements and complexity of each action.
package sandbox

import (
	"time"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/executor"
)

// DefaultSandboxImage is the container image for simple binary installations.
// Uses Debian because the tsuku binary is dynamically linked against glibc.
const DefaultSandboxImage = "debian:bookworm-slim"

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
	// Uses debian:bookworm-slim for binary-only, ubuntu:22.04 for source builds.
	Image string

	// Resources are the recommended resource limits.
	Resources ResourceLimits
}

// ComputeSandboxRequirements derives container requirements from a plan.
// It iterates through plan steps, querying each action's RequiresNetwork() method
// via the NetworkValidator interface, and aggregates the results.
//
// The function selects appropriate container image and resource limits based on:
//   - Network requirements: any network-requiring action upgrades to ubuntu image
//   - Build complexity: build actions (configure_make, cmake_build, etc.) get more resources
func ComputeSandboxRequirements(plan *executor.InstallationPlan) *SandboxRequirements {
	reqs := &SandboxRequirements{
		RequiresNetwork: false,
		Image:           DefaultSandboxImage,
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
		reqs.Image = SourceBuildSandboxImage
		reqs.Resources = SourceBuildLimits()
	}

	// Also upgrade for plans with known build actions (even if offline)
	// Build actions like configure_make may not need network but still need
	// more resources and a fuller base image. Network is required because
	// the sandbox needs apt-get to install build-essential.
	if hasBuildActions(plan) {
		reqs.RequiresNetwork = true
		reqs.Image = SourceBuildSandboxImage
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
