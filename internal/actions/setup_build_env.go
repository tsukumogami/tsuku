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
// This action doesn't modify files or state - it validates that the environment
// can be configured and provides informative output about what will be available.
//
// No parameters required - uses ctx.Dependencies automatically.
func (a *SetupBuildEnvAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	fmt.Printf("   Configuring build environment from %d dependencies\n", len(ctx.Dependencies.InstallTime))

	// Get configured environment (validates it can be built)
	env := buildAutotoolsEnv(ctx)

	// Extract and display the configured environment variables
	var pkgConfigPath, cppFlags, ldFlags, cc, cxx string
	for _, e := range env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
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
