package actions

import (
	"fmt"
	"strings"
)

// SetupBuildEnvAction configures the build environment from resolved dependencies.
// This action is a no-op wrapper that prepares environment variables for subsequent
// build actions. The actual environment configuration is handled by buildAutotoolsEnv().
type SetupBuildEnvAction struct{ BaseAction }

// IsDeterministic returns true because environment setup produces identical results.
func (SetupBuildEnvAction) IsDeterministic() bool { return true }

// Name returns the action name
func (a *SetupBuildEnvAction) Name() string {
	return "setup_build_env"
}

// Execute configures the build environment from dependencies.
// This action populates ctx.Env with environment variables that subsequent
// build actions (configure_make, cmake_build) can use. The environment includes
// PATH, PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS, CC, CXX, and other variables needed
// for building with tsuku-provided dependencies.
//
// No parameters required - uses ctx.Dependencies automatically.
func (a *SetupBuildEnvAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	fmt.Printf("   Configuring build environment from %d dependencies\n", len(ctx.Dependencies.InstallTime))

	// Build environment from dependencies and set it on the context
	ctx.Env = buildAutotoolsEnv(ctx)

	// Extract and display the configured environment variables
	var path, pkgConfigPath, cppFlags, ldFlags, cc, cxx string
	for _, e := range ctx.Env {
		if strings.HasPrefix(e, "PATH=") {
			path = strings.TrimPrefix(e, "PATH=")
		} else if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			pkgConfigPath = strings.TrimPrefix(e, "PKG_CONFIG_PATH=")
		} else if strings.HasPrefix(e, "CPPFLAGS=") {
			cppFlags = strings.TrimPrefix(e, "CPPFLAGS=")
		} else if strings.HasPrefix(e, "LDFLAGS=") {
			ldFlags = strings.TrimPrefix(e, "LDFLAGS=")
		} else if strings.HasPrefix(e, "CC=") {
			cc = strings.TrimPrefix(e, "CC=")
		} else if strings.HasPrefix(e, "CXX=") {
			cxx = strings.TrimPrefix(e, "CXX=")
		}
	}

	// Show what was configured
	if path != "" {
		// Count only tsuku-managed paths (before first non-tsuku path)
		pathParts := strings.Split(path, ":")
		tsukuPathCount := 0
		for _, p := range pathParts {
			if strings.Contains(p, "/.tsuku/") {
				tsukuPathCount++
			} else {
				break // Stop at first non-tsuku path
			}
		}
		if tsukuPathCount > 0 {
			fmt.Printf("   PATH: %d dependency bin path(s) prepended\n", tsukuPathCount)
		}
	}
	if pkgConfigPath != "" {
		pathCount := len(strings.Split(pkgConfigPath, ":"))
		fmt.Printf("   PKG_CONFIG_PATH: %d path(s)\n", pathCount)
	}
	if cppFlags != "" {
		flagCount := len(strings.Fields(cppFlags))
		fmt.Printf("   CPPFLAGS: %d flag(s)\n", flagCount)
	}
	if ldFlags != "" {
		flagCount := len(strings.Fields(ldFlags))
		fmt.Printf("   LDFLAGS: %d flag(s)\n", flagCount)
	}
	if cc != "" {
		fmt.Printf("   CC: %s\n", cc)
	}
	if cxx != "" {
		fmt.Printf("   CXX: %s\n", cxx)
	}

	if len(ctx.Dependencies.InstallTime) == 0 {
		fmt.Printf("   (No dependencies to configure)\n")
	}

	return nil
}
